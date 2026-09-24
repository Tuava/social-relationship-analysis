package api

import (
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

func TestPrivateMediaRequiresAuthentication(t *testing.T) {
	s := &Server{}
	for _, target := range []string{"/api/v1/media/chat-image?file=/etc/passwd", "/api/v1/media/avatars/person/10000001"} {
		w := httptest.NewRecorder()
		s.Router().ServeHTTP(w, httptest.NewRequest("GET", target, nil))
		if w.Code != 401 {
			t.Fatalf("%s returned %d", target, w.Code)
		}
	}
}
func TestLocalChatImageConfinement(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	png := append([]byte{137, 80, 78, 71, 13, 10, 26, 10}, make([]byte, 512)...)
	for _, name := range []string{filepath.Join(root, "image.png"), filepath.Join(outside, "private.png")} {
		if err := os.WriteFile(name, png, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "secret.env"), []byte("PASSWORD=private"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "private.png"), filepath.Join(root, "escape.png")); err != nil {
		t.Fatal(err)
	}
	s := &Server{ObjectRoot: root}
	for _, tc := range []struct {
		name string
		want bool
	}{{"image.png", true}, {"secret.env", false}, {"escape.png", false}, {"../private.png", false}} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/", nil)
		if got := s.serveLocalChatImage(w, r, filepath.Join(root, tc.name)); got != tc.want {
			t.Errorf("%s: %v", tc.name, got)
		}
	}
}
func TestChatImageURLAllowlist(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want bool
	}{{"https://multimedia.nt.qq.com/a", true}, {"https://gchat.qpic.cn/a", true}, {"http://127.0.0.1/a", false}, {"https://qq.com.example.org/a", false}, {"file:///etc/passwd", false}, {"https://qq.com:8080/a", false}} {
		u, _ := url.Parse(tc.raw)
		if trustedChatImageURL(u) != tc.want {
			t.Error(tc.raw)
		}
	}
}
func TestFrontendRouting(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html>app</html>"), 0600); err != nil {
		t.Fatal(err)
	}
	s := &Server{FrontendDist: dir}
	for _, tc := range []struct {
		path   string
		status int
	}{{"/", 200}, {"/login", 200}, {"/api/unknown", 404}, {"/missing.js", 404}, {"/.env", 404}} {
		w := httptest.NewRecorder()
		s.Router().ServeHTTP(w, httptest.NewRequest("GET", tc.path, nil))
		if w.Code != tc.status {
			t.Errorf("%s: %d", tc.path, w.Code)
		}
	}
}
