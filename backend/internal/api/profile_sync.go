package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

func (s *Server) syncPersonProfile(w http.ResponseWriter, r *http.Request) {
	personID := chi.URLParam(r, "id")
	var qq string
	if err := s.Repo.DB.QueryRow(r.Context(), `SELECT platform_user_id FROM person_identifiers WHERE person_id=$1 AND platform='qq' LIMIT 1`, personID).Scan(&qq); err != nil {
		writeError(w, 404, err)
		return
	}
	var in struct {
		AccountID string `json:"account_id"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	accountID := strings.TrimSpace(in.AccountID)
	if accountID == "" {
		if err := s.Repo.DB.QueryRow(r.Context(), `SELECT id::text FROM napcat_accounts WHERE enabled=true AND http_url<>'' ORDER BY created_at LIMIT 1`).Scan(&accountID); err != nil {
			writeJSON(w, 400, map[string]string{"error": "没有可用的 NapCat HTTP 账号"})
			return
		}
	}
	account, err := s.Repo.GetAccount(r.Context(), accountID)
	if err != nil {
		writeError(w, 404, err)
		return
	}
	data, rawID, err := s.Collector.SyncPersonProfile(r.Context(), account, personID, qq)
	if err != nil {
		writeJSON(w, 502, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"data": map[string]any{"person_id": personID, "qq": qq, "profile": data, "raw_record_id": rawID}})
}
