package api

import (
	"net/http"
	"os"
	"path"
	"strings"
)

// serveFrontend serves a prebuilt SPA. Unknown API and asset URLs stay 404.
func (s *Server) serveFrontend(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if name == "api" || strings.HasPrefix(name, "api/") {
		http.NotFound(w, r)
		return
	}
	for _, part := range strings.Split(name, "/") {
		if strings.HasPrefix(part, ".") {
			http.NotFound(w, r)
			return
		}
	}
	root, err := os.OpenRoot(s.FrontendDist)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer root.Close()
	f, err := root.Open(name)
	if err == nil {
		info, statErr := f.Stat()
		if statErr == nil && info.Mode().IsRegular() {
			defer f.Close()
			w.Header().Set("X-Content-Type-Options", "nosniff")
			http.ServeContent(w, r, name, info.ModTime(), f)
			return
		}
		f.Close()
	}
	if path.Ext(name) != "" || strings.HasPrefix(name, "assets/") {
		http.NotFound(w, r)
		return
	}
	index, err := root.Open("index.html")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer index.Close()
	info, err := index.Stat()
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	http.ServeContent(w, r, "index.html", info.ModTime(), index)
}
