package api

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

func (s *Server) remoteAvatar(w http.ResponseWriter, r *http.Request) {
	kind := chi.URLParam(r, "kind")
	id := chi.URLParam(r, "id")
	if id == "" || strings.Trim(id, "0123456789") != "" {
		http.Error(w, "invalid avatar id", http.StatusBadRequest)
		return
	}

	// 1. 优先查库：如果本地已入库下载完成该头像，则直接从本地存储输出
	var localAssetID string
	if kind == "person" {
		_ = s.Repo.DB.QueryRow(r.Context(), `
			SELECT mr.asset_id::text 
			FROM media_references mr 
			JOIN persons p ON p.id = mr.person_id 
			JOIN person_identifiers pi ON pi.person_id = p.id AND pi.platform = 'qq'
			WHERE pi.platform_user_id = $1 AND mr.media_kind = 'avatar' AND mr.status = 'completed' AND mr.asset_id IS NOT NULL
			ORDER BY mr.completed_at DESC LIMIT 1`, id).Scan(&localAssetID)
	} else if kind == "group" {
		_ = s.Repo.DB.QueryRow(r.Context(), `
			SELECT mr.asset_id::text 
			FROM media_references mr 
			JOIN "groups" g ON g.id = mr.group_id 
			WHERE g.platform_group_id = $1 AND mr.media_kind = 'avatar' AND mr.status = 'completed' AND mr.asset_id IS NOT NULL
			ORDER BY mr.completed_at DESC LIMIT 1`, id).Scan(&localAssetID)
	}

	if localAssetID != "" {
		var objectPath, mimeType, filename string
		err := s.Repo.DB.QueryRow(r.Context(), `SELECT object_path,mime_type,original_filename FROM media_assets WHERE id=$1`, localAssetID).Scan(&objectPath, &mimeType, &filename)
		if err == nil {
			root, rootErr := filepath.Abs(s.ObjectRoot)
			path, pathErr := filepath.Abs(filepath.Join(s.ObjectRoot, objectPath))
			if rootErr == nil && pathErr == nil && (path == root || strings.HasPrefix(path, root+string(os.PathSeparator))) {
				file, err := os.Open(path)
				if err == nil {
					defer file.Close()
					stat, err := file.Stat()
					if err == nil {
						if mimeType != "" {
							w.Header().Set("Content-Type", mimeType)
						}
						if filename != "" {
							w.Header().Set("Content-Disposition", "inline")
						}
						w.Header().Set("Cache-Control", "private, max-age=86400")
						http.ServeContent(w, r, filename, stat.ModTime(), file)
						return
					}
				}
			}
		}
	}

	// 2. 本地暂无资产时，向官方无防盗链限制的高清 CDN 代理拉取 (使用 100x100 极速轻量图，大幅降低网络与显存开销)
	var source string
	switch kind {
	case "person":
		source = "https://q1.qlogo.cn/g?b=qq&nk=" + id + "&s=100"
	case "group":
		source = "https://p.qlogo.cn/gh/" + id + "/" + id + "/100/"
	default:
		http.Error(w, "invalid avatar kind", http.StatusBadRequest)
		return
	}
	request, err := http.NewRequestWithContext(r.Context(), http.MethodGet, source, nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	request.Header.Set("User-Agent", "Mozilla/5.0")
	response, err := (&http.Client{Timeout: 12 * time.Second}).Do(request)
	if err != nil {
		http.Error(w, "avatar unavailable", http.StatusBadGateway)
		return
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		http.Error(w, "avatar unavailable", http.StatusBadGateway)
		return
	}
	contentType := response.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "image/") {
		http.Error(w, "invalid avatar response", http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "private, max-age=86400")
	_, _ = io.Copy(w, io.LimitReader(response.Body, 5<<20))
}
