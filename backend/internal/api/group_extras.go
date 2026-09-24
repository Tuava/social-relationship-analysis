package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/seagull/social-relationship-analysis/backend/internal/collectors/napcat"
)

// groupPlatformAccount resolves the platform (QQ) group id for an internal
// group uuid and the NapCat HTTP account to proxy through. accountID is the
// selected account id; when empty the first enabled HTTP account is used.
func (s *Server) groupPlatformAccount(w http.ResponseWriter, r *http.Request) (platformID, accountID string, ok bool) {
	if err := s.Repo.DB.QueryRow(r.Context(), `SELECT platform_group_id FROM groups WHERE id=$1`, chi.URLParam(r, "id")).Scan(&platformID); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "group not found"})
		return "", "", false
	}
	accountID = strings.TrimSpace(r.URL.Query().Get("account_id"))
	if accountID == "" {
		if err := s.Repo.DB.QueryRow(r.Context(), `SELECT id::text FROM napcat_accounts WHERE enabled=true AND http_url<>'' ORDER BY created_at LIMIT 1`).Scan(&accountID); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "没有可用的 NapCat HTTP 账号"})
			return "", "", false
		}
	}
	return platformID, accountID, true
}

// enabledAccount loads the NapCat account identified by accountID (or the first
// enabled HTTP account when empty) and validates it has an http_url.
func (s *Server) enabledAccount(w http.ResponseWriter, r *http.Request, accountID string) (napcat.HTTPClient, string, bool) {
	if accountID == "" {
		if err := s.Repo.DB.QueryRow(r.Context(), `SELECT id::text FROM napcat_accounts WHERE enabled=true AND http_url<>'' ORDER BY created_at LIMIT 1`).Scan(&accountID); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "没有可用的 NapCat HTTP 账号"})
			return napcat.HTTPClient{}, "", false
		}
	}
	account, err := s.Repo.GetAccount(r.Context(), accountID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "account not found"})
		return napcat.HTTPClient{}, "", false
	}
	if account.HTTPURL == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "http_url is required for this account"})
		return napcat.HTTPClient{}, "", false
	}
	return *napcat.NewHTTPClient(account.HTTPURL, account.HTTPToken), account.ID, true
}

// writeGroupData saves the raw NapCat payload to raw_records and responds with
// the payload + raw_record_id so the caller never loses the original response.
func (s *Server) writeGroupData(w http.ResponseWriter, r *http.Request, accountID, platformID, endpoint, source string, raw json.RawMessage) {
	rawID := ""
	if id, err := s.Repo.SaveRaw(r.Context(), accountID, source, endpoint+":"+platformID, raw); err == nil {
		rawID = id
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": raw, "raw_record_id": rawID, "group_id": platformID, "endpoint": endpoint})
}

func (s *Server) groupFiles(w http.ResponseWriter, r *http.Request) {
	platformID, accountID, ok := s.groupPlatformAccount(w, r)
	if !ok {
		return
	}
	client, accID, ok := s.enabledAccount(w, r, accountID)
	if !ok {
		return
	}
	raw, err := client.GetGroupFiles(r.Context(), platformID)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	s.writeGroupData(w, r, accID, platformID, "get_group_root_files", "napcat_http", raw)
}

func (s *Server) groupAlbums(w http.ResponseWriter, r *http.Request) {
	platformID, accountID, ok := s.groupPlatformAccount(w, r)
	if !ok {
		return
	}
	client, accID, ok := s.enabledAccount(w, r, accountID)
	if !ok {
		return
	}
	raw, err := client.GetGroupAlbums(r.Context(), platformID)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	s.writeGroupData(w, r, accID, platformID, "get_qun_album_list", "napcat_http", raw)
}

func (s *Server) groupNotices(w http.ResponseWriter, r *http.Request) {
	platformID, accountID, ok := s.groupPlatformAccount(w, r)
	if !ok {
		return
	}
	client, accID, ok := s.enabledAccount(w, r, accountID)
	if !ok {
		return
	}
	raw, err := client.GetGroupNotices(r.Context(), platformID)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	s.writeGroupData(w, r, accID, platformID, "_get_group_notice", "napcat_http", raw)
}

func (s *Server) groupHonor(w http.ResponseWriter, r *http.Request) {
	platformID, accountID, ok := s.groupPlatformAccount(w, r)
	if !ok {
		return
	}
	client, accID, ok := s.enabledAccount(w, r, accountID)
	if !ok {
		return
	}
	honorType := r.URL.Query().Get("type")
	if honorType == "" {
		honorType = "all"
	}
	raw, err := client.GetGroupHonor(r.Context(), platformID, honorType)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	s.writeGroupData(w, r, accID, platformID, "get_group_honor_info:"+honorType, "napcat_http", raw)
}

func (s *Server) recentContacts(w http.ResponseWriter, r *http.Request) {
	client, accID, ok := s.enabledAccount(w, r, strings.TrimSpace(r.URL.Query().Get("account_id")))
	if !ok {
		return
	}
	raw, err := client.GetRecentContacts(r.Context())
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	rawID := ""
	if id, err := s.Repo.SaveRaw(r.Context(), accID, "napcat_http", "get_recent_contact", raw); err == nil {
		rawID = id
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": raw, "raw_record_id": rawID, "endpoint": "get_recent_contact"})
}

func (s *Server) groupMemberInfo(w http.ResponseWriter, r *http.Request) {
	platformID, accountID, ok := s.groupPlatformAccount(w, r)
	if !ok {
		return
	}
	userQQ := chi.URLParam(r, "qq")
	if userQQ == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "qq is required"})
		return
	}
	client, accID, ok := s.enabledAccount(w, r, accountID)
	if !ok {
		return
	}
	raw, err := client.GetGroupMemberInfo(r.Context(), platformID, userQQ)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	s.writeGroupData(w, r, accID, platformID, "get_group_member_info:"+userQQ, "napcat_http", raw)
}
