package media

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/seagull/social-relationship-analysis/backend/internal/collectors/napcat"
	"github.com/seagull/social-relationship-analysis/backend/internal/collectors/qzone"
	"github.com/seagull/social-relationship-analysis/backend/internal/secrets"
)

const defaultMaxObjectSize int64 = 512 << 20

type Worker struct {
	DB               *pgxpool.Pool
	Store            Store
	Logger           *slog.Logger
	MaxObjectSize    int64
	AllowedMediaRoot string
	Client           *http.Client
	concurrency      chan struct{}
}

func (w *Worker) Run(ctx context.Context) {
	if w.Logger == nil {
		w.Logger = slog.Default()
	}
	if w.MaxObjectSize <= 0 {
		w.MaxObjectSize = defaultMaxObjectSize
	}
	if w.Client == nil {
		w.Client = &http.Client{Timeout: 45 * time.Second}
	}
	if w.concurrency == nil {
		w.concurrency = make(chan struct{}, 6)
	}
	if _, err := w.DB.Exec(ctx, `UPDATE media_references SET status='pending',next_attempt_at=now()
        WHERE status='downloading'`); err != nil {
		w.Logger.Warn("recover stale media downloads", "error", err)
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.processBatch(ctx)
		}
	}
}

func (w *Worker) processBatch(ctx context.Context) {
	var paused bool
	var concurrency int
	if err := w.DB.QueryRow(ctx, `SELECT paused,concurrency FROM media_download_settings WHERE singleton=true`).Scan(&paused, &concurrency); err != nil || paused {
		return
	}
	available := concurrency - len(w.concurrency)
	if available <= 0 {
		return
	}
	for range available {
		select {
		case w.concurrency <- struct{}{}:
		case <-ctx.Done():
			return
		default:
			return
		}
		var id, accountID, kind, sourceRef, sourceURL, resolver, filename string
		err := w.DB.QueryRow(ctx, `WITH next AS (
            SELECT r.id FROM media_references r
            LEFT JOIN media_download_policies p ON p.media_kind=r.media_kind
            WHERE r.status IN ('pending','failed') AND r.attempt_count < 6 AND r.next_attempt_at <= now()
              AND (COALESCE(p.enabled,false) OR r.download_selected)
            ORDER BY r.download_selected DESC,
              COALESCE(p.priority,0) DESC,
              r.relation_depth ASC,
              r.priority_boost DESC,
              r.created_at ASC
            FOR UPDATE OF r SKIP LOCKED LIMIT 1
        ) UPDATE media_references r SET status='downloading',attempt_count=r.attempt_count+1,last_attempt_at=now()
        FROM next WHERE r.id=next.id
        RETURNING r.id::text,r.source_account_id::text,r.media_kind,r.source_ref,r.source_url,r.resolver_endpoint,r.original_filename`).
			Scan(&id, &accountID, &kind, &sourceRef, &sourceURL, &resolver, &filename)
		if err != nil {
			<-w.concurrency
			return
		}
		go func() {
			defer func() { <-w.concurrency }()
			if err := w.processOne(ctx, id, accountID, kind, sourceRef, sourceURL, resolver, filename); err != nil {
				_, _ = w.DB.Exec(ctx, `UPDATE media_references SET status='failed',last_error=$2,
                next_attempt_at=now()+(LEAST(attempt_count,5)||' minutes')::interval WHERE id=$1`, id, err.Error())
				w.Logger.Warn("media archive failed", "reference_id", id, "error", err)
			}
		}()
	}
}

func (w *Worker) processOne(ctx context.Context, referenceID, accountID, kind, sourceRef, sourceURL, resolver, filename string) error {
	account, err := w.account(ctx, accountID)
	if err != nil {
		return err
	}
	source := sourceURL
	var inline []byte
	var localPath string
	if filepath.IsAbs(source) {
		localPath, source = source, ""
	}
	parsedSource, _ := url.Parse(source)
	if source != "" && (parsedSource == nil || (parsedSource.Scheme != "http" && parsedSource.Scheme != "https")) {
		source = ""
	}
	if resolver != "" && sourceRef != "" && !w.usableLocalPath(localPath) {
		params := map[string]any{"file": sourceRef, "file_id": sourceRef}
		clientCall := func() ([]byte, error) {
			if strings.HasPrefix(resolver, "qzone:") {
				params = map[string]any{"url": sourceRef}
				return qzone.NewHTTPClient(account.qzoneHTTPURL, account.qzoneToken).Call(ctx, strings.TrimPrefix(resolver, "qzone:"), params)
			}
			return napcat.NewHTTPClient(account.httpURL, account.httpToken).Call(ctx, resolver, params)
		}
		if resolver == "/get_record" {
			params["out_format"] = "mp3"
		}
		value, callErr := clientCall()
		if callErr != nil && source == "" {
			return callErr
		}
		if callErr == nil {
			resolvedSource, resolvedPath, resolvedInline := responseSource(value)
			if filepath.IsAbs(resolvedSource) && resolvedPath == "" {
				resolvedPath, resolvedSource = resolvedSource, ""
			}
			if resolvedSource != "" || resolvedPath != "" || len(resolvedInline) > 0 {
				source, inline = resolvedSource, resolvedInline
				if resolvedPath != "" {
					localPath = resolvedPath
				}
			}
		}
	}
	if source == "" && localPath == "" && len(inline) == 0 {
		return fmt.Errorf("media source is empty")
	}

	var reader io.Reader
	closeReader := func() error { return nil }
	mimeType := mime.TypeByExtension(filepath.Ext(filename))
	switch {
	case len(inline) > 0:
		reader = bytes.NewReader(inline)
	case w.usableLocalPath(localPath):
		file, openErr := os.Open(localPath)
		if openErr != nil {
			return openErr
		}
		reader, closeReader = file, file.Close
		if filename == "" {
			filename = filepath.Base(localPath)
		}
	case source != "":
		req, requestErr := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
		if requestErr != nil {
			return requestErr
		}
		allowedPrivateHost := httpHost(account.httpURL)
		if requestErr = validateRemoteURL(req.URL, allowedPrivateHost); requestErr != nil {
			return requestErr
		}
		client := *w.Client
		client.CheckRedirect = func(redirect *http.Request, via []*http.Request) error {
			if len(via) >= 4 {
				return fmt.Errorf("too many media redirects")
			}
			return validateRemoteURL(redirect.URL, allowedPrivateHost)
		}
		res, requestErr := client.Do(req)
		if requestErr != nil {
			return requestErr
		}
		if res.StatusCode < 200 || res.StatusCode >= 300 {
			res.Body.Close()
			return fmt.Errorf("media download: %s", res.Status)
		}
		reader, closeReader = res.Body, res.Body.Close
		if mimeType == "" {
			mimeType = strings.TrimSpace(strings.Split(res.Header.Get("Content-Type"), ";")[0])
		}
		if filename == "" {
			filename = filepath.Base(res.Request.URL.Path)
		}
	case localPath != "":
		return fmt.Errorf("local media path is outside NAPCAT_MEDIA_ROOT or unavailable")
	}
	defer closeReader()
	if mimeType == "" {
		mimeType = mime.TypeByExtension(filepath.Ext(filename))
	}
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	asset, err := w.Store.PutLimited(reader, mimeType, w.MaxObjectSize)
	if err != nil {
		return err
	}
	var assetID string
	err = w.DB.QueryRow(ctx, `INSERT INTO media_assets(
        sha256,mime_type,size,object_path,source_url,source_message_id,original_filename,source_account_id,source_raw_record_id,downloaded_at
    ) SELECT $1,$2,$3,$4,NULLIF($5,''),NULLIF($6,''),$7,$8,raw_record_id,now() FROM media_references WHERE id=$9
    ON CONFLICT(sha256) DO UPDATE SET downloaded_at=now(),
        mime_type=CASE WHEN media_assets.mime_type='application/octet-stream' THEN EXCLUDED.mime_type ELSE media_assets.mime_type END,
        original_filename=COALESCE(NULLIF(media_assets.original_filename,''),EXCLUDED.original_filename)
    RETURNING id::text`, asset.SHA256, asset.MIMEType, asset.Size, asset.ObjectPath, source, sourceRef, filename, accountID, referenceID).Scan(&assetID)
	if err != nil {
		return err
	}
	if _, err = w.DB.Exec(ctx, `UPDATE media_references SET status='completed',asset_id=$2,last_error=NULL,completed_at=now(),download_selected=false WHERE id=$1`, referenceID, assetID); err != nil {
		return err
	}
	_, _ = w.DB.Exec(ctx, `UPDATE message_media SET media_asset_id=$2 WHERE media_reference_id=$1`, referenceID, assetID)
	_, _ = w.DB.Exec(ctx, `UPDATE content_media SET media_asset_id=$2 WHERE media_reference_id=$1`, referenceID, assetID)
	if kind == "avatar" {
		_, _ = w.DB.Exec(ctx, `UPDATE person_profiles SET avatar_uri='/api/v1/media/assets/'||$2 WHERE person_id=(SELECT person_id FROM media_references WHERE id=$1)`, referenceID, assetID)
	}
	return nil
}

type accountRow struct{ httpURL, httpToken, qzoneHTTPURL, qzoneToken string }

func (w *Worker) account(ctx context.Context, id string) (accountRow, error) {
	var account accountRow
	err := w.DB.QueryRow(ctx, `SELECT a.http_url,a.http_token,COALESCE(q.http_url,''),COALESCE(q.access_token,'')
		FROM napcat_accounts a LEFT JOIN qzone_connections q ON q.account_id=a.id WHERE a.id=$1`, id).
		Scan(&account.httpURL, &account.httpToken, &account.qzoneHTTPURL, &account.qzoneToken)
	if err != nil {
		return account, err
	}
	account.httpToken, err = secrets.Decrypt(account.httpToken, os.Getenv("SRA_CONFIG_ENCRYPTION_KEY"))
	if err != nil {
		return account, fmt.Errorf("decrypt NapCat HTTP credential: %w", err)
	}
	account.qzoneToken, err = secrets.Decrypt(account.qzoneToken, os.Getenv("SRA_CONFIG_ENCRYPTION_KEY"))
	if err != nil {
		return account, fmt.Errorf("decrypt QZone credential: %w", err)
	}
	return account, err
}
func (w *Worker) allowedLocalPath(path string) bool {
	if w.AllowedMediaRoot == "" {
		return false
	}
	root, rootErr := filepath.Abs(w.AllowedMediaRoot)
	value, valueErr := filepath.Abs(path)
	return rootErr == nil && valueErr == nil && (value == root || strings.HasPrefix(value, root+string(os.PathSeparator)))
}
func (w *Worker) usableLocalPath(path string) bool {
	if !w.allowedLocalPath(path) {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}
func responseSource(raw []byte) (source, local string, data []byte) {
	var value struct {
		URL    string `json:"url"`
		File   string `json:"file"`
		Base64 string `json:"base64"`
	}
	if json.Unmarshal(raw, &value) != nil {
		return
	}
	source = value.URL
	if filepath.IsAbs(value.File) {
		local = value.File
	}
	if value.Base64 != "" {
		data, _ = base64.StdEncoding.DecodeString(value.Base64)
	}
	return
}
func validateRemoteURL(value *url.URL, allowedPrivateHost string) error {
	if value == nil || (value.Scheme != "http" && value.Scheme != "https") {
		return fmt.Errorf("media URL must use http or https")
	}
	host := value.Hostname()
	if host == "" {
		return fmt.Errorf("media URL host is empty")
	}
	if ip := net.ParseIP(host); ip != nil && (ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()) && !sameHost(host, allowedPrivateHost) {
		return fmt.Errorf("private media URL is not allowed")
	}
	if net.ParseIP(host) == nil && !sameHost(host, allowedPrivateHost) && !trustedQQMediaHost(host) {
		addresses, err := net.LookupIP(host)
		if err != nil {
			return fmt.Errorf("resolve media URL host: %w", err)
		}
		for _, address := range addresses {
			if address.IsLoopback() || address.IsPrivate() || address.IsLinkLocalUnicast() {
				return fmt.Errorf("media URL resolves to a private address")
			}
		}
	}
	return nil
}

func trustedQQMediaHost(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	for _, suffix := range []string{"qq.com", "qpic.cn", "qlogo.cn", "gtimg.cn"} {
		if host == suffix || strings.HasSuffix(host, "."+suffix) {
			return true
		}
	}
	return false
}

func httpHost(raw string) string {
	value, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return value.Hostname()
}

func sameHost(left, right string) bool {
	leftIP, rightIP := net.ParseIP(left), net.ParseIP(right)
	if leftIP != nil && rightIP != nil {
		return leftIP.Equal(rightIP)
	}
	return strings.EqualFold(left, right)
}
