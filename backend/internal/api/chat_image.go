package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const maxChatImageSize = 32 << 20

func (s *Server) chatImage(w http.ResponseWriter, r *http.Request) {
	file := strings.TrimSpace(r.URL.Query().Get("file"))
	source := strings.TrimSpace(r.URL.Query().Get("url"))
	if filepath.IsAbs(file) {
		if s.serveLocalChatImage(w, r, file) {
			return
		}
		http.NotFound(w, r)
		return
	}
	if file != "" {
		client, _, ok := s.enabledAccount(w, r, r.URL.Query().Get("account_id"))
		if !ok {
			return
		}
		raw, err := client.Call(r.Context(), "/get_image", map[string]any{"file": file})
		if err == nil {
			var result struct {
				File string `json:"file"`
				URL  string `json:"url"`
			}
			if json.Unmarshal(raw, &result) == nil {
				if filepath.IsAbs(result.File) && s.serveLocalChatImage(w, r, result.File) {
					return
				}
				if result.URL != "" {
					source = result.URL
				}
			}
		}
	}
	parsed, err := url.Parse(source)
	if err != nil || !trustedChatImageURL(parsed) {
		http.NotFound(w, r)
		return
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, parsed.String(), nil)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	transport := &http.Transport{DialContext: dialPublicImageHost}
	defer transport.CloseIdleConnections()
	client := &http.Client{Timeout: 15 * time.Second, Transport: transport, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 || !trustedChatImageURL(req.URL) {
			return fmt.Errorf("image redirect not allowed")
		}
		return nil
	}}
	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, "image unavailable", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		http.Error(w, "image unavailable", http.StatusBadGateway)
		return
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxChatImageSize+1))
	if err != nil || len(body) > maxChatImageSize || !chatImageMIME(http.DetectContentType(body)) {
		http.Error(w, "invalid image", http.StatusBadGateway)
		return
	}
	setChatImageHeaders(w, http.DetectContentType(body))
	http.ServeContent(w, r, "image", time.Time{}, bytes.NewReader(body))
}

func (s *Server) serveLocalChatImage(w http.ResponseWriter, r *http.Request, name string) bool {
	for _, dir := range []string{s.NapCatMediaRoot, s.ObjectRoot} {
		if dir == "" {
			continue
		}
		abs, err := filepath.Abs(dir)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(abs, name)
		if err != nil || !filepath.IsLocal(rel) {
			continue
		}
		// os.Root confines symlinks as well as lexical traversal, without a check/open race.
		root, err := os.OpenRoot(abs)
		if err != nil {
			continue
		}
		f, err := root.Open(rel)
		root.Close()
		if err != nil {
			continue
		}
		info, err := f.Stat()
		if err != nil || !info.Mode().IsRegular() || info.Size() > maxChatImageSize {
			f.Close()
			continue
		}
		var head [512]byte
		n, _ := f.Read(head[:])
		contentType := http.DetectContentType(head[:n])
		if !chatImageMIME(contentType) {
			f.Close()
			continue
		}
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			f.Close()
			continue
		}
		setChatImageHeaders(w, contentType)
		http.ServeContent(w, r, filepath.Base(name), info.ModTime(), f)
		f.Close()
		return true
	}
	return false
}

func chatImageMIME(value string) bool {
	switch value {
	case "image/png", "image/jpeg", "image/gif", "image/webp", "image/bmp", "image/x-icon":
		return true
	}
	return false
}
func setChatImageHeaders(w http.ResponseWriter, contentType string) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
}
func trustedChatImageURL(u *url.URL) bool {
	if u == nil || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil {
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
func dialPublicImageHost(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	for _, addr := range ips {
		ip := addr.IP
		if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
			continue
		}
		conn, err := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if err == nil {
			return conn, nil
		}
	}
	return nil, fmt.Errorf("no reachable public image address")
}
