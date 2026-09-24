package analysis

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"hash/crc32"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// These tests use generated images, temporary directories and in-memory
// HTTP/database doubles only. They never read DATABASE_URL or call a real model.
func visionTestPNG(t *testing.T) []byte {
	t.Helper()
	var out bytes.Buffer
	if err := png.Encode(&out, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func visionTestFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func visionTestRoots(t *testing.T) (string, string) {
	t.Helper()
	objects, napcat := t.TempDir(), t.TempDir()
	t.Setenv("OBJECT_ROOT", objects)
	t.Setenv("NAPCAT_MEDIA_ROOT", napcat)
	return objects, napcat
}

func TestVisionLocalRootsAndDetectedMIME(t *testing.T) {
	objects, napcat := visionTestRoots(t)
	data := visionTestPNG(t)
	objectPath := filepath.Join(objects, "stored.img")
	napcatPath := filepath.Join(napcat, "qq.jpg") // Deliberately misleading extension.
	visionTestFile(t, objectPath, data)
	visionTestFile(t, napcatPath, data)
	for _, name := range []string{objectPath, napcatPath, "stored.img", "qq.jpg"} {
		path, err := EnsureLocalImageFile(context.Background(), name, "")
		if err != nil {
			t.Fatalf("allowed image %q rejected: %v", name, err)
		}
		encoded, err := EncodeImageToBase64(path, "text/plain")
		if err != nil || !strings.HasPrefix(encoded, "data:image/png;base64,") {
			t.Fatalf("image bytes, not caller MIME/extension, must determine MIME: %q, %v", encoded, err)
		}
	}
	if got := NewVisionAnalyzer(nil, "").ObjectRoot; got != objects {
		t.Fatalf("MCP constructor did not inherit OBJECT_ROOT: %q", got)
	}
	custom := t.TempDir()
	visionTestFile(t, filepath.Join(custom, "asset"), data)
	if _, err := readVisionImage("asset", custom); err != nil {
		t.Fatalf("explicit API object root was ignored: %v", err)
	}
}

func TestVisionRejectsOutsidePathsAndSymlinkEscapes(t *testing.T) {
	objects, _ := visionTestRoots(t)
	outside := t.TempDir()
	externalImage := filepath.Join(outside, "image.png")
	visionTestFile(t, externalImage, visionTestPNG(t))
	if err := os.Symlink(externalImage, filepath.Join(objects, "escape.png")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(objects, "escape-dir")); err != nil {
		t.Fatal(err)
	}
	traversal, err := filepath.Rel(objects, externalImage)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{externalImage, traversal, "escape.png", filepath.Join("escape-dir", "image.png"), "file://" + externalImage, objects} {
		if _, err := EnsureLocalImageFile(context.Background(), path, objects); err == nil {
			t.Errorf("unsafe path accepted: %q", path)
		}
		if _, err := EncodeImageToBase64(path, "image/png"); err == nil {
			t.Errorf("encoder accepted unsafe path: %q", path)
		}
	}
	visionTestFile(t, filepath.Join(objects, "good.png"), visionTestPNG(t))
	if err := os.Symlink("good.png", filepath.Join(objects, "inside.png")); err != nil {
		t.Fatal(err)
	}
	if _, err := readVisionImage("inside.png", objects); err != nil {
		t.Fatalf("a symlink remaining inside the configured root should work: %v", err)
	}
}

func TestVisionUsesValidatedSnapshotAfterPathReplacement(t *testing.T) {
	objects, _ := visionTestRoots(t)
	path := filepath.Join(objects, "image.png")
	data := visionTestPNG(t)
	visionTestFile(t, path, data)
	img, err := readVisionImage(path, objects)
	if err != nil {
		t.Fatal(err)
	}
	external := filepath.Join(t.TempDir(), "secret")
	visionTestFile(t, external, []byte("not an image"))
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, path); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(img.data, data) || !strings.HasPrefix(visionImageDataURL(img.data, img.mimeType), "data:image/png;base64,") {
		t.Fatal("validated snapshot changed after file replacement")
	}
	if _, err := EncodeImageToBase64(path, "image/png"); err == nil {
		t.Fatal("a new encoder call must reject the replacement symlink")
	}
}

func TestVisionRejectsFakeTruncatedOversizedAndHugeImages(t *testing.T) {
	objects, _ := visionTestRoots(t)
	data := visionTestPNG(t)
	truncated := data[:33] // Complete PNG signature and IHDR, but no pixel data.
	if _, _, err := image.DecodeConfig(bytes.NewReader(truncated)); err != nil {
		t.Fatalf("fixture must have a valid header: %v", err)
	}
	for name, body := range map[string][]byte{"fake.jpg": []byte("credentials"), "truncated.png": truncated, "empty.png": nil} {
		path := filepath.Join(objects, name)
		visionTestFile(t, path, body)
		if _, err := EncodeImageToBase64(path, "image/png"); err == nil {
			t.Errorf("invalid image accepted: %s", name)
		}
	}
	large := filepath.Join(objects, "large.png")
	f, err := os.Create(large)
	if err != nil {
		t.Fatal(err)
	}
	err = f.Truncate(maxVisionImageBytes + 1)
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		t.Fatalf("create oversized fixture: %v %v", err, closeErr)
	}
	if _, err := EncodeImageToBase64(large, "image/png"); err == nil {
		t.Fatal("oversized file was accepted")
	}
	for _, dims := range [][2]uint32{{maxVisionImageSide + 1, 1}, {6000, 4000}} {
		bomb := bytes.Clone(data)
		binary.BigEndian.PutUint32(bomb[16:20], dims[0])
		binary.BigEndian.PutUint32(bomb[20:24], dims[1])
		binary.BigEndian.PutUint32(bomb[29:33], crc32.ChecksumIEEE(bomb[12:29]))
		if _, _, err := image.DecodeConfig(bytes.NewReader(bomb)); err != nil {
			t.Fatalf("dimension fixture has invalid header: %v", err)
		}
		if _, err := validateVisionImage(bomb); err == nil || !strings.Contains(err.Error(), "dimensions") {
			t.Fatalf("oversized dimensions must fail before pixel decoding: %v", err)
		}
	}
}

func TestVisionSupportsPNGJPEGAndGIF(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	for _, format := range []string{"png", "jpeg", "gif"} {
		var out bytes.Buffer
		var err error
		switch format {
		case "png":
			err = png.Encode(&out, img)
		case "jpeg":
			err = jpeg.Encode(&out, img, nil)
		case "gif":
			err = gif.Encode(&out, img, nil)
		}
		if err != nil {
			t.Fatal(err)
		}
		mimeType, err := validateVisionImage(out.Bytes())
		if err != nil || mimeType != "image/"+format {
			t.Fatalf("valid %s rejected: mime=%q err=%v", format, mimeType, err)
		}
	}
}

func TestVisionURLAndIPPolicy(t *testing.T) {
	for _, raw := range []string{"https://multimedia.nt.qq.com/download?fileid=123", "http://gchat.qpic.cn/a", "https://q1.qlogo.cn/g", "https://img.gtimg.cn/a", "https://Q1.QLOGO.CN./a"} {
		u, err := url.Parse(raw)
		if err != nil || !trustedVisionURL(u) {
			t.Errorf("controlled QQ URL rejected: %q", raw)
		}
	}
	for _, raw := range []string{"https://qq.com.evil.example/a", "https://evilqq.com/a", "https://user:pass@qq.com/a", "https://qq.com:8080/a", "file:///tmp/a", "data:image/png;base64,AA==", "http://127.0.0.1/a", "http://[::1]/a", "//qpic.cn/a"} {
		u, err := url.Parse(raw)
		if err == nil && trustedVisionURL(u) {
			t.Errorf("unsafe URL accepted: %q", raw)
		}
	}
	for _, ip := range []string{"127.0.0.1", "10.1.1.1", "192.168.1.1", "169.254.169.254", "100.64.0.1", "0.0.0.1", "192.0.2.1", "198.18.0.1", "240.0.0.1", "::1", "::", "fc00::1", "fe80::1", "fec0::1", "3fff::1", "::7f00:1", "::ffff:127.0.0.1", "64:ff9b::7f00:1", "2002:7f00:1::"} {
		if publicVisionIP(net.ParseIP(ip)) {
			t.Errorf("non-public destination accepted: %s", ip)
		}
	}
	for _, ip := range []string{"1.1.1.1", "2606:4700:4700::1111"} {
		if !publicVisionIP(net.ParseIP(ip)) {
			t.Errorf("public address rejected: %s", ip)
		}
	}
}

type visionRoundTripFunc func(*http.Request) (*http.Response, error)

func (f visionRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestVisionRemoteDownloadAndRedirectGuards(t *testing.T) {
	objects, _ := visionTestRoots(t)
	data := visionTestPNG(t)
	client := visionHTTPClient()
	transport := client.Transport.(*http.Transport)
	if transport.Proxy != nil || transport.DialContext == nil {
		t.Fatal("image transport must pin checked public IPs without an environment proxy")
	}
	calls := 0
	client.Transport = visionRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/plain"}}, Body: io.NopCloser(bytes.NewReader(data)), Request: req}, nil
	})
	source := "https://gchat.qpic.cn/image.png"
	for i := 0; i < 2; i++ {
		path, err := downloadVisionImage(context.Background(), source, objects, client)
		if err != nil {
			t.Fatal(err)
		}
		img, err := readVisionImage(path, objects)
		if err != nil || !bytes.Equal(img.data, data) {
			t.Fatalf("cached download was not validated: %v", err)
		}
	}
	if calls != 1 {
		t.Fatalf("valid cache should avoid the second request: calls=%d", calls)
	}
	for _, destination := range []string{"http://127.0.0.1/a", "https://evil.example/a", "https://qpic.cn:8080/a", "http://qpic.cn/a", "https://qq.com@evil.example/a"} {
		calls = 0
		client.Transport = visionRoundTripFunc(func(req *http.Request) (*http.Response, error) {
			calls++
			return &http.Response{StatusCode: 302, Header: http.Header{"Location": {destination}}, Body: io.NopCloser(strings.NewReader("")), Request: req}, nil
		})
		if _, err := downloadVisionImage(context.Background(), "https://gchat.qpic.cn/redirect", objects, client); err == nil || calls != 1 {
			t.Fatalf("redirect must be rejected before destination is requested: %s calls=%d err=%v", destination, calls, err)
		}
	}
	calls = 0
	client.Transport = visionRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return &http.Response{StatusCode: 302, Header: http.Header{"Location": {"https://qpic.cn/final"}}, Body: io.NopCloser(strings.NewReader("")), Request: req}, nil
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(data)), Request: req}, nil
	})
	if _, err := downloadVisionImage(context.Background(), "https://gchat.qpic.cn/allowed-redirect", objects, client); err != nil || calls != 2 {
		t.Fatalf("controlled QQ redirect should remain usable: calls=%d err=%v", calls, err)
	}
}

type visionTrackedBody struct {
	io.Reader
	closed bool
}

func (b *visionTrackedBody) Close() error { b.closed = true; return nil }

type visionBrokenReader struct{}

func (visionBrokenReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func TestVisionDownloadRejectsInvalidBodiesAndClosesResponses(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		length int64
		body   io.Reader
	}{
		{"not-image", 200, -1, strings.NewReader("credentials")},
		{"read-failure", 200, -1, visionBrokenReader{}},
		{"server-failure", 500, -1, strings.NewReader("error")},
		{"advertised-limit", 200, maxVisionImageBytes + 1, strings.NewReader("")},
		{"stream-limit", 200, -1, io.LimitReader(visionZeroReader{}, maxVisionImageBytes+1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			objects, _ := visionTestRoots(t)
			body := &visionTrackedBody{Reader: tc.body}
			client := visionHTTPClient()
			client.Transport = visionRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: tc.status, ContentLength: tc.length, Header: make(http.Header), Body: body, Request: req}, nil
			})
			if _, err := downloadVisionImage(context.Background(), "https://qpic.cn/bad", objects, client); err == nil {
				t.Fatal("invalid download accepted")
			}
			if !body.closed {
				t.Fatal("response body leaked")
			}
			entries, err := os.ReadDir(objects)
			if err != nil || len(entries) != 0 {
				t.Fatalf("invalid download must not leave a partial cache: entries=%v err=%v", entries, err)
			}
		})
	}
}

type visionZeroReader struct{}

func (visionZeroReader) Read(p []byte) (int, error) { clear(p); return len(p), nil }

func TestVisionCacheConfinesWrites(t *testing.T) {
	objects, _ := visionTestRoots(t)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(objects, "cloud_cache")); err != nil {
		t.Fatal(err)
	}
	if _, err := cacheVisionImage(visionTestPNG(t), objects, "entry"); err == nil {
		t.Fatal("symlink cache directory escaped OBJECT_ROOT")
	}
	entries, err := os.ReadDir(outside)
	if err != nil || len(entries) != 0 {
		t.Fatalf("outside directory was modified: %v %v", entries, err)
	}
	objects = t.TempDir()
	if err := os.Mkdir(filepath.Join(objects, "cloud_cache"), 0700); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(outside, "secret")
	visionTestFile(t, secret, []byte("unchanged"))
	if err := os.Symlink(secret, filepath.Join(objects, "cloud_cache", "entry.img")); err != nil {
		t.Fatal(err)
	}
	if _, err := cacheVisionImage(visionTestPNG(t), objects, "entry"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(secret)
	if err != nil || string(data) != "unchanged" {
		t.Fatalf("atomic cache write followed destination symlink: %q %v", data, err)
	}
}

type visionRowFunc func(...any) error

func (f visionRowFunc) Scan(dest ...any) error { return f(dest...) }

type visionDBStub struct {
	query func(string, ...any) pgx.Row
	exec  func(string, ...any) (pgconn.CommandTag, error)
}

func (db visionDBStub) QueryRow(_ context.Context, query string, args ...any) pgx.Row {
	return db.query(query, args...)
}

func (db visionDBStub) Exec(_ context.Context, query string, args ...any) (pgconn.CommandTag, error) {
	return db.exec(query, args...)
}

func TestVisionURLGetsUUIDBeforeSaving(t *testing.T) {
	objects, _ := visionTestRoots(t)
	data := visionTestPNG(t)
	source := "https://gchat.qpic.cn/registered.png"
	digest := sha256.Sum256([]byte(source))
	// Seed the validated URL cache: no external requests or real database needed.
	if _, err := cacheVisionImage(data, objects, "url-"+hex.EncodeToString(digest[:])); err != nil {
		t.Fatal(err)
	}
	const id = "00000000-0000-4000-8000-000000000123"
	inserted, updated := false, false
	db := visionDBStub{
		query: func(query string, args ...any) pgx.Row {
			if !strings.Contains(query, "INSERT INTO media_assets") || !strings.Contains(query, "ON CONFLICT(sha256)") || len(args) != 5 {
				t.Fatalf("unexpected registration: %s %#v", query, args)
			}
			if args[1] != "image/png" || args[2] != int64(len(data)) || args[4] != source || !filepath.IsLocal(args[3].(string)) {
				t.Fatalf("invalid registered image metadata: %#v", args)
			}
			inserted = true
			return visionRowFunc(func(dest ...any) error { *dest[0].(*string) = id; return nil })
		},
		exec: func(query string, args ...any) (pgconn.CommandTag, error) {
			if !inserted || !strings.Contains(query, "UPDATE media_assets") || args[0] != id {
				t.Fatalf("OCR must update the returned UUID, never the URL: %s %#v", query, args)
			}
			updated = true
			return pgconn.NewCommandTag("UPDATE 1"), nil
		},
	}
	asset, err := prepareVisionAsset(context.Background(), db, source, objects)
	if err != nil || asset.id != id {
		t.Fatalf("URL registration failed: asset=%v err=%v", asset, err)
	}
	if err := saveVisionAnalysis(context.Background(), db, &VisionAnalysisResult{AssetID: asset.id, OCRText: "recognized", ProcessedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if !updated {
		t.Fatal("URL result was not persisted")
	}
	if err := saveVisionAnalysis(context.Background(), db, &VisionAnalysisResult{AssetID: source}); err == nil {
		t.Fatal("raw URL must never enter a UUID UPDATE")
	}
}

func TestVisionRegisteredUUIDAndLookupFailure(t *testing.T) {
	objects, _ := visionTestRoots(t)
	data := visionTestPNG(t)
	visionTestFile(t, filepath.Join(objects, "asset.img"), data)
	const id = "00000000-0000-4000-8000-000000000456"
	db := visionDBStub{query: func(query string, args ...any) pgx.Row {
		if !strings.Contains(query, "SELECT object_path") || args[0] != id {
			t.Fatalf("UUID must resolve its existing asset: %s %#v", query, args)
		}
		return visionRowFunc(func(dest ...any) error { *dest[0].(*string) = "asset.img"; return nil })
	}}
	asset, err := prepareVisionAsset(context.Background(), db, id, objects)
	if err != nil || asset.id != id || !bytes.Equal(asset.image.data, data) {
		t.Fatalf("existing UUID path broken: %v %v", asset, err)
	}
	// Even a file with the same name must not turn a missing UUID into a path.
	visionTestFile(t, filepath.Join(objects, id), data)
	db.query = func(string, ...any) pgx.Row {
		return visionRowFunc(func(...any) error { return pgx.ErrNoRows })
	}
	if _, err := prepareVisionAsset(context.Background(), db, id, objects); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("missing UUID unexpectedly fell back to local file: %v", err)
	}
}

func TestVisionRejectsUnsafeAssetPathBeforeWriting(t *testing.T) {
	objects, _ := visionTestRoots(t)
	external := filepath.Join(t.TempDir(), "asset.png")
	visionTestFile(t, external, visionTestPNG(t))
	db := visionDBStub{query: func(string, ...any) pgx.Row {
		return visionRowFunc(func(dest ...any) error { *dest[0].(*string) = external; return nil })
	}}
	if _, err := prepareVisionAsset(context.Background(), db, "00000000-0000-4000-8000-000000000789", objects); err == nil {
		t.Fatal("database asset paths must also respect configured media roots")
	}
	db.exec = func(string, ...any) (pgconn.CommandTag, error) { return pgconn.NewCommandTag("UPDATE 0"), nil }
	if err := saveVisionAnalysis(context.Background(), db, &VisionAnalysisResult{AssetID: "00000000-0000-4000-8000-000000000789"}); err == nil {
		t.Fatal("missing asset update was reported as successful")
	}
}

func TestVisionRejectsUntrustedURLAndCorruptedCacheBeforeHTTP(t *testing.T) {
	objects, _ := visionTestRoots(t)
	calls := 0
	client := visionHTTPClient()
	client.Transport = visionRoundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return nil, errors.New("HTTP must not be reached")
	})
	if _, err := downloadVisionImage(context.Background(), "http://127.0.0.1/image", objects, client); err == nil {
		t.Fatal("untrusted URL accepted")
	}
	source := "https://qpic.cn/cache-check"
	digest := sha256.Sum256([]byte(source))
	path, err := cacheVisionImage(visionTestPNG(t), objects, "url-"+hex.EncodeToString(digest[:]))
	if err != nil {
		t.Fatal(err)
	}
	visionTestFile(t, path, []byte("cached credentials, not image pixels"))
	if _, err := downloadVisionImage(context.Background(), source, objects, client); err == nil {
		t.Fatal("corrupted cache accepted")
	}
	if calls != 0 {
		t.Fatalf("rejected input unexpectedly initiated HTTP: %d calls", calls)
	}
}

func TestVisionRedirectLimit(t *testing.T) {
	objects, _ := visionTestRoots(t)
	calls := 0
	client := visionHTTPClient()
	client.Transport = visionRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 302, Header: http.Header{"Location": {"https://qpic.cn/loop"}}, Body: io.NopCloser(strings.NewReader("")), Request: req}, nil
	})
	if _, err := downloadVisionImage(context.Background(), "https://qpic.cn/loop", objects, client); err == nil || calls != 5 {
		t.Fatalf("redirect chain was not bounded: calls=%d err=%v", calls, err)
	}
}

func TestVisionImportsNapCatImageAndStopsOnRegistrationFailure(t *testing.T) {
	objects, napcat := visionTestRoots(t)
	path := filepath.Join(napcat, "image.png")
	data := visionTestPNG(t)
	visionTestFile(t, path, data)
	const id = "00000000-0000-4000-8000-000000000abc"
	registered := 0
	db := visionDBStub{query: func(query string, args ...any) pgx.Row {
		if !strings.Contains(query, "INSERT INTO media_assets") || args[4] != "" {
			t.Fatalf("local image registration leaked a local path as a URL: %s %#v", query, args)
		}
		copyPath := filepath.Join(objects, args[3].(string))
		copied, err := readVisionImage(copyPath, objects)
		if err != nil || !bytes.Equal(copied.data, data) {
			t.Fatalf("NapCat image was not copied into OBJECT_ROOT: %v", err)
		}
		registered++
		return visionRowFunc(func(dest ...any) error { *dest[0].(*string) = id; return nil })
	}}
	asset, err := prepareVisionAsset(context.Background(), db, path, objects)
	if err != nil || asset.id != id || registered != 1 {
		t.Fatalf("NapCat image import failed: asset=%v count=%d err=%v", asset, registered, err)
	}
	wantErr := errors.New("database registration unavailable")
	db.query = func(string, ...any) pgx.Row {
		return visionRowFunc(func(...any) error { return wantErr })
	}
	if _, err := prepareVisionAsset(context.Background(), db, path, objects); !errors.Is(err, wantErr) {
		t.Fatalf("registration errors must stop preparation before the LLM: %v", err)
	}
}
