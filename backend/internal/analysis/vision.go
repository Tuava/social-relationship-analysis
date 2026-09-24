package analysis

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/seagull/social-relationship-analysis/backend/internal/media"
	"github.com/seagull/social-relationship-analysis/backend/internal/secrets"
)

type VisionAnalysisResult struct {
	AssetID           string            `json:"asset_id"`
	PHash             string            `json:"phash"`
	OCRText           string            `json:"ocr_text"`
	Category          string            `json:"category"`
	VisualTags        []string          `json:"visual_tags"`
	SceneDescription  string            `json:"scene_description"`
	IntentAnalysis    string            `json:"intent_analysis"`
	ExtractedEntities []ExtractedEntity `json:"extracted_entities"`
	ProcessedAt       time.Time         `json:"processed_at"`
}

type ExtractedEntity struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

type VisionAnalyzer struct {
	DB         *pgxpool.Pool
	ObjectRoot string
}

func NewVisionAnalyzer(db *pgxpool.Pool, objectRoot string) *VisionAnalyzer {
	return &VisionAnalyzer{
		DB:         db,
		ObjectRoot: visionObjectRoot(objectRoot),
	}
}

const (
	maxVisionImageBytes  = 10 << 20
	maxVisionImagePixels = 20_000_000
	maxVisionImageSide   = 16_384
)

type validatedVisionImage struct {
	data     []byte
	mimeType string
	path     string
}

func visionObjectRoot(configured string) string {
	if root := strings.TrimSpace(configured); root != "" {
		return root
	}
	if root := strings.TrimSpace(os.Getenv("OBJECT_ROOT")); root != "" {
		return root
	}
	// Match config.Load; never implicitly grant access to cwd or the temp directory.
	return "../data/objects"
}

// readVisionImage opens through os.Root, so symlink traversal is checked during
// the open itself. Validation, hashing and upload subsequently use the same bytes.
func readVisionImage(name, objectRoot string) (*validatedVisionImage, error) {
	if name == "" || (!filepath.IsAbs(name) && !filepath.IsLocal(name)) {
		return nil, fmt.Errorf("image path is outside configured media roots")
	}
	roots := []string{visionObjectRoot(objectRoot)}
	if dir := strings.TrimSpace(os.Getenv("NAPCAT_MEDIA_ROOT")); dir != "" {
		roots = append(roots, dir)
	}
	var failures []error
	for _, dir := range roots {
		abs, err := filepath.Abs(dir)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		rel := name
		if filepath.IsAbs(name) {
			rel, err = filepath.Rel(abs, name)
			if err != nil || !filepath.IsLocal(rel) {
				continue
			}
		}
		img, err := readVisionImageInRoot(abs, rel)
		if err == nil {
			return img, nil
		}
		failures = append(failures, err)
	}
	if len(failures) == 0 {
		return nil, fmt.Errorf("image path is outside OBJECT_ROOT and NAPCAT_MEDIA_ROOT")
	}
	return nil, fmt.Errorf("read image within configured media roots: %w", errors.Join(failures...))
}

func readVisionImageInRoot(dir, relative string) (*validatedVisionImage, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	// Reject directories/devices/FIFOs before opening, and recheck the opened file.
	info, err := root.Stat(relative)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxVisionImageBytes {
		return nil, fmt.Errorf("image must be a regular file no larger than %d bytes", maxVisionImageBytes)
	}
	file, err := root.Open(relative)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err = file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxVisionImageBytes {
		return nil, fmt.Errorf("image must be a regular file no larger than %d bytes", maxVisionImageBytes)
	}
	data, err := io.ReadAll(io.LimitReader(file, maxVisionImageBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read image: %w", err)
	}
	mimeType, err := validateVisionImage(data)
	if err != nil {
		return nil, err
	}
	return &validatedVisionImage{data: data, mimeType: mimeType, path: filepath.Join(dir, relative)}, nil
}

func validateVisionImage(data []byte) (string, error) {
	if len(data) == 0 || len(data) > maxVisionImageBytes {
		return "", fmt.Errorf("image size must be between 1 and %d bytes", maxVisionImageBytes)
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("invalid image header: %w", err)
	}
	var mimeType string
	switch format {
	case "jpeg":
		mimeType = "image/jpeg"
	case "png":
		mimeType = "image/png"
	case "gif":
		mimeType = "image/gif"
	default:
		return "", fmt.Errorf("unsupported image format %q", format)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > maxVisionImageSide || cfg.Height > maxVisionImageSide || int64(cfg.Width)*int64(cfg.Height) > maxVisionImagePixels {
		return "", fmt.Errorf("image dimensions exceed vision limits")
	}
	// A plausible header or extension is not sufficient: decode the actual pixels.
	decoded, decodedFormat, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("invalid image data: %w", err)
	}
	if decodedFormat != format || decoded.Bounds().Dx() != cfg.Width || decoded.Bounds().Dy() != cfg.Height {
		return "", fmt.Errorf("inconsistent image dimensions or format")
	}
	return mimeType, nil
}

func trustedVisionURL(u *url.URL) bool {
	if u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.Opaque != "" || u.Host == "" {
		return false
	}
	if port := u.Port(); port != "" && port != "80" && port != "443" {
		return false
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	for _, suffix := range []string{"qq.com", "qpic.cn", "qlogo.cn", "gtimg.cn"} {
		if host == suffix || strings.HasSuffix(host, "."+suffix) {
			return true
		}
	}
	return false
}

var visionReservedNetworks = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("::/96"),
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("2001::/23"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"),
	netip.MustParsePrefix("3fff::/20"),
	netip.MustParsePrefix("fec0::/10"),
}

func publicVisionIP(ip net.IP) bool {
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	addr = addr.Unmap()
	if !addr.IsGlobalUnicast() || addr.IsPrivate() || addr.IsLoopback() || addr.IsLinkLocalUnicast() {
		return false
	}
	for _, prefix := range visionReservedNetworks {
		if prefix.Contains(addr) {
			return false
		}
	}
	return true
}

func dialVisionHost(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	if !trustedVisionURL(&url.URL{Scheme: "https", Host: net.JoinHostPort(host, port)}) {
		return nil, fmt.Errorf("image host is not an allowed QQ media host")
	}
	addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	for _, addr := range addresses {
		if addr.Zone != "" || !publicVisionIP(addr.IP) {
			return nil, fmt.Errorf("image host resolves to a non-public address")
		}
	}
	var lastErr error
	for _, addr := range addresses {
		// Dial the checked IP, not the hostname: no second DNS resolution/rebinding.
		conn, err := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, network, net.JoinHostPort(addr.IP.String(), port))
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	return nil, fmt.Errorf("no reachable public image address: %v", lastErr)
}

func visionHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 30 * time.Second,
		// No environment proxy: it would bypass the IP validation in DialContext.
		Transport: &http.Transport{
			DialContext:            dialVisionHost,
			TLSHandshakeTimeout:    10 * time.Second,
			ResponseHeaderTimeout:  10 * time.Second,
			MaxResponseHeaderBytes: 1 << 20,
			DisableCompression:     true,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 || !trustedVisionURL(req.URL) {
				return fmt.Errorf("image redirect is not allowed")
			}
			if len(via) > 0 && via[len(via)-1].URL.Scheme == "https" && req.URL.Scheme != "https" {
				return fmt.Errorf("image redirect cannot downgrade HTTPS")
			}
			return nil
		},
	}
}

// cacheVisionImage uses root-relative atomic writes; neither a symlink cache
// directory nor an existing symlink destination can redirect writes outside it.
func cacheVisionImage(data []byte, objectRoot, key string) (string, error) {
	if _, err := validateVisionImage(data); err != nil {
		return "", err
	}
	if !filepath.IsLocal(key) || filepath.Base(key) != key {
		return "", fmt.Errorf("invalid image cache key")
	}
	dir, err := filepath.Abs(visionObjectRoot(objectRoot))
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0750); err != nil {
		return "", err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return "", err
	}
	defer root.Close()
	if err := root.MkdirAll("cloud_cache", 0750); err != nil {
		return "", err
	}
	tmp := filepath.Join("cloud_cache", ".vision-"+rand.Text())
	file, err := root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return "", err
	}
	defer root.Remove(tmp)
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	if writeErr != nil {
		return "", writeErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	relative := filepath.Join("cloud_cache", key+".img")
	if err := root.Rename(tmp, relative); err != nil {
		return "", err
	}
	return filepath.Join(dir, relative), nil
}

func downloadVisionImage(ctx context.Context, source, objectRoot string, client *http.Client) (string, error) {
	parsed, err := url.Parse(source)
	if err != nil || !trustedVisionURL(parsed) {
		return "", fmt.Errorf("only controlled QQ HTTP(S) image URLs are allowed")
	}
	hash := sha256.Sum256([]byte(source))
	key := "url-" + hex.EncodeToString(hash[:])
	cached := filepath.Join("cloud_cache", key+".img")
	// Inspect the OBJECT_ROOT cache only, not an identically named NapCat file.
	dir, err := filepath.Abs(visionObjectRoot(objectRoot))
	if err != nil {
		return "", err
	}
	if img, err := readVisionImageInRoot(dir, cached); err == nil {
		return img.path, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("invalid cached image: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("download image: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("image server returned HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > maxVisionImageBytes {
		return "", fmt.Errorf("remote image exceeds %d bytes", maxVisionImageBytes)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxVisionImageBytes+1))
	if err != nil {
		return "", fmt.Errorf("read remote image: %w", err)
	}
	return cacheVisionImage(data, objectRoot, key)
}

func visionFileReference(value string) bool {
	if value == "" || len(value) > 128 || strings.ContainsAny(value, `/\\`) || value == "." || value == ".." {
		return false
	}
	for _, c := range value {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '.' || c == '_' || c == '-') {
			return false
		}
	}
	ext := strings.ToLower(filepath.Ext(value))
	if ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".gif" {
		return true
	}
	_, err := hex.DecodeString(value)
	return len(value) == 32 && err == nil
}

// EnsureLocalImageFile resolves only configured media roots or controlled QQ URLs.
func EnsureLocalImageFile(ctx context.Context, rawPath, objectRoot string) (string, error) {
	return resolveVisionImage(ctx, rawPath, objectRoot, nil)
}

func EnsureLocalImageFileWithDB(ctx context.Context, rawPath, objectRoot string, db *pgxpool.Pool) (string, error) {
	if db == nil {
		return EnsureLocalImageFile(ctx, rawPath, objectRoot)
	}
	return resolveVisionImage(ctx, rawPath, objectRoot, db)
}

func resolveVisionImage(ctx context.Context, rawPath, objectRoot string, db visionAssetDB) (string, error) {
	rawPath = strings.ReplaceAll(strings.TrimSpace(rawPath), "&amp;", "&")
	if rawPath == "" {
		return "", fmt.Errorf("empty image path or URL")
	}
	parsed, parseErr := url.Parse(rawPath)
	if parseErr == nil && parsed.Scheme != "" {
		if !trustedVisionURL(parsed) {
			return "", fmt.Errorf("only controlled QQ HTTP(S) image URLs are allowed")
		}
		client := visionHTTPClient()
		defer client.CloseIdleConnections()
		path, err := downloadVisionImage(ctx, rawPath, objectRoot, client)
		if err == nil {
			return path, nil
		}
		if db != nil {
			file := parsed.Query().Get("file")
			if file == "" {
				file = parsed.Query().Get("fileid")
			}
			if visionFileReference(file) {
				if path, fallbackErr := fetchImageFromNapCat(ctx, db, file, objectRoot); fallbackErr == nil {
					return path, nil
				}
			}
		}
		return "", err
	}
	img, err := readVisionImage(rawPath, objectRoot)
	if err == nil {
		return img.path, nil
	}
	if db != nil && visionFileReference(rawPath) && errors.Is(err, os.ErrNotExist) {
		return fetchImageFromNapCat(ctx, db, rawPath, objectRoot)
	}
	return "", err
}

func fetchImageFromNapCat(ctx context.Context, db visionAssetDB, file, objectRoot string) (string, error) {
	if db == nil || !visionFileReference(file) {
		return "", fmt.Errorf("no database or invalid NapCat image reference")
	}
	var httpURL, token string
	err := db.QueryRow(ctx, `SELECT http_url, http_token FROM napcat_accounts WHERE enabled=true AND http_url != '' LIMIT 1`).Scan(&httpURL, &token)
	if err != nil {
		return "", err
	}
	token, err = secrets.Decrypt(token, os.Getenv("SRA_CONFIG_ENCRYPTION_KEY"))
	if err != nil {
		return "", fmt.Errorf("decrypt NapCat HTTP credential: %w", err)
	}
	reqBody, _ := json.Marshal(map[string]any{"file": file})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(httpURL, "/")+"/get_image", bytes.NewReader(reqBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	client := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error {
		return fmt.Errorf("NapCat image resolver redirects are not allowed")
	}}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("NapCat image resolver returned HTTP %d", resp.StatusCode)
	}
	var imgResp struct {
		Status string `json:"status"`
		Data   struct {
			File string `json:"file"`
			URL  string `json:"url"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&imgResp); err != nil {
		return "", err
	}
	if imgResp.Data.File != "" {
		if img, err := readVisionImage(imgResp.Data.File, objectRoot); err == nil {
			return img.path, nil
		}
	}
	if imgResp.Data.URL != "" {
		imageClient := visionHTTPClient()
		defer imageClient.CloseIdleConnections()
		return downloadVisionImage(ctx, imgResp.Data.URL, objectRoot, imageClient)
	}
	return "", fmt.Errorf("NapCat did not return an allowed, valid image")
}

type visionAssetDB interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

type preparedVisionAsset struct {
	id    string
	image *validatedVisionImage
}

func visionUUID(value string) bool {
	var id pgtype.UUID
	return len(value) == 36 && id.Scan(value) == nil && id.Valid
}

func prepareVisionAsset(ctx context.Context, db visionAssetDB, target, objectRoot string) (*preparedVisionAsset, error) {
	target = strings.TrimSpace(target)
	path := target
	registered := visionUUID(target)
	if registered {
		if err := db.QueryRow(ctx, `SELECT object_path FROM media_assets WHERE id=$1::uuid`, target).Scan(&path); err != nil {
			return nil, fmt.Errorf("load registered media asset: %w", err)
		}
	}
	// A UUID lookup failure must never fall back to interpreting it as a path.
	fullPath, err := resolveVisionImage(ctx, path, objectRoot, db)
	if err != nil {
		return nil, err
	}
	img, err := readVisionImage(fullPath, objectRoot)
	if err != nil {
		return nil, err
	}
	if registered {
		return &preparedVisionAsset{id: target, image: img}, nil
	}
	return registerVisionImage(ctx, db, img, target, objectRoot)
}

// URL/NapCat images need a real media UUID before any paid LLM call. Store a
// content-addressed copy in OBJECT_ROOT, including when the source is in NapCat.
func registerVisionImage(ctx context.Context, db visionAssetDB, img *validatedVisionImage, source, objectRoot string) (*preparedVisionAsset, error) {
	digest := sha256.Sum256(img.data)
	hash := hex.EncodeToString(digest[:])
	stored, err := cacheVisionImage(img.data, objectRoot, "sha256-"+hash)
	if err != nil {
		return nil, err
	}
	root, err := filepath.Abs(visionObjectRoot(objectRoot))
	if err != nil {
		return nil, err
	}
	relative, err := filepath.Rel(root, stored)
	if err != nil || !filepath.IsLocal(relative) {
		return nil, fmt.Errorf("cached image is outside OBJECT_ROOT")
	}
	sourceURL := ""
	if parsed, err := url.Parse(source); err == nil && trustedVisionURL(parsed) {
		sourceURL = source
	}
	var id string
	err = db.QueryRow(ctx, `INSERT INTO media_assets(sha256,mime_type,size,object_path,source_url,downloaded_at)
		VALUES($1,$2,$3,$4,NULLIF($5,''),now())
		ON CONFLICT(sha256) DO UPDATE SET mime_type=EXCLUDED.mime_type,size=EXCLUDED.size,
			object_path=EXCLUDED.object_path,downloaded_at=now()
		RETURNING id::text`, hash, img.mimeType, int64(len(img.data)), relative, sourceURL).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("register vision image: %w", err)
	}
	if !visionUUID(id) {
		return nil, fmt.Errorf("registered media asset has no valid UUID")
	}
	return &preparedVisionAsset{id: id, image: img}, nil
}

func saveVisionAnalysis(ctx context.Context, db visionAssetDB, result *VisionAnalysisResult) error {
	if !visionUUID(result.AssetID) {
		return fmt.Errorf("vision result requires a registered media UUID")
	}
	tags, err := json.Marshal(result.VisualTags)
	if err != nil {
		return err
	}
	if len(result.VisualTags) == 0 {
		tags = []byte("[]")
	}
	tag, err := db.Exec(ctx, `UPDATE media_assets
		SET phash=COALESCE(NULLIF($2,''),phash),ocr_text=$3,transcript_text=$4,
			visual_tags=$5::jsonb,ocr_status='completed',processed_at=$6
		WHERE id=$1::uuid`, result.AssetID, result.PHash, result.OCRText,
		result.SceneDescription+"\n\n【意图研判】"+result.IntentAnalysis, string(tags), result.ProcessedAt)
	if err != nil {
		return fmt.Errorf("save vision analysis to db: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("media asset disappeared before saving vision analysis")
	}
	return nil
}

// AnalyzeMediaAsset executes deep multimodal vision analysis on a media asset UUID, direct URL, or file path.
func (va *VisionAnalyzer) AnalyzeMediaAsset(ctx context.Context, assetID string) (*VisionAnalysisResult, error) {
	if va.DB == nil {
		return nil, fmt.Errorf("vision analysis requires a media database")
	}
	asset, err := prepareVisionAsset(ctx, va.DB, assetID, va.ObjectRoot)
	if err != nil {
		return nil, fmt.Errorf("prepare vision image: %w", err)
	}
	assetID = asset.id

	// Hash and upload the same validated snapshot, never reopen an unchecked path.
	phash, err := media.ComputePHashFromReader(bytes.NewReader(asset.image.data))
	if err != nil {
		return nil, fmt.Errorf("compute image perceptual hash: %w", err)
	}
	dataURL := visionImageDataURL(asset.image.data, asset.image.mimeType)

	cfg := GetActiveLLMConfig(ctx, va.DB)

	// Resolve vision-specific configuration from system_configs if present
	if va.DB != nil {
		var vModel, vBase, vKey string
		_ = va.DB.QueryRow(ctx, `
			SELECT 
				COALESCE((SELECT value #>> '{}' FROM system_configs WHERE key='vision.model'), ''),
				COALESCE((SELECT value #>> '{}' FROM system_configs WHERE key='vision.api_base'), ''),
				COALESCE((SELECT value #>> '{}' FROM system_configs WHERE key='vision.api_key'), '')
		`).Scan(&vModel, &vBase, &vKey)

		if vModel != "" {
			cfg.Model = vModel
		} else {
			cfg.Model = "glm-4.6v-flash"
		}
		if vBase != "" {
			cfg.APIBase = vBase
		}
		if vKey != "" {
			if decrypted, decryptErr := secrets.Decrypt(vKey, os.Getenv("SRA_CONFIG_ENCRYPTION_KEY")); decryptErr == nil {
				cfg.APIKey = decrypted
			}
		}
	} else if strings.Contains(cfg.Model, "glm-4") {
		cfg.Model = "glm-4.6v-flash"
	}

	prompt := `你是一名专业的社交网络与开源情报（OSINT）视觉分析专家。请对传入的图片进行高精度多模态研判，并严格输出 JSON 格式（不要输出额外解释）：
{
  "ocr_text": "完整识别出的所有可见文字内容（包括聊天文字、水印、时间、数字、金额、表情包配字、发球/结算数据）",
  "category": "主要分类（聊天截图 / 支付转账 / 社交自拍 / 游戏结算 / 影视动漫 / 系统通知 / 表情包梗图 / 证件票据 / 风景静物）",
  "visual_tags": ["标签1", "标签2", "标签3"],
  "scene_description": "画面客观详细描述（色彩、主体、UI界面、场景）",
  "intent_analysis": "社交传播意图与潜在争议分析（例如：日常分享 / 游戏炫耀 / 讽刺吐槽 / 证据保留）",
  "extracted_entities": [
    {"type": "qq", "value": "123456"},
    {"type": "name", "value": "某昵称"},
    {"type": "amount", "value": "￥50.00"}
  ]
}`

	messages := []ChatMessage{
		{
			Role: "user",
			Content: []VisionContentPart{
				{
					Type: "text",
					Text: prompt,
				},
				{
					Type: "image_url",
					ImageURL: &ImageURLParam{
						URL: dataURL,
					},
				},
			},
		},
	}

	rawResp, err := CallLLM(ctx, cfg, messages, 0.1)
	if err != nil {
		return nil, fmt.Errorf("vision llm analysis: %w", err)
	}

	cleanJSON := CleanJSONResponse(rawResp)
	var parsed struct {
		OCRText           string            `json:"ocr_text"`
		Category          string            `json:"category"`
		VisualTags        []string          `json:"visual_tags"`
		SceneDescription  string            `json:"scene_description"`
		IntentAnalysis    string            `json:"intent_analysis"`
		ExtractedEntities []ExtractedEntity `json:"extracted_entities"`
	}
	if err := json.Unmarshal([]byte(cleanJSON), &parsed); err != nil {
		// Fallback: put raw text as OCR text if json parse failed
		parsed.OCRText = cleanJSON
		parsed.Category = "other"
	}

	result := &VisionAnalysisResult{
		AssetID:           assetID,
		PHash:             phash,
		OCRText:           parsed.OCRText,
		Category:          parsed.Category,
		VisualTags:        parsed.VisualTags,
		SceneDescription:  parsed.SceneDescription,
		IntentAnalysis:    parsed.IntentAnalysis,
		ExtractedEntities: parsed.ExtractedEntities,
		ProcessedAt:       time.Now(),
	}
	if err := saveVisionAnalysis(ctx, va.DB, result); err != nil {
		return nil, err
	}
	return result, nil
}
