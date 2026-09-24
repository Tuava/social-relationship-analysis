package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/seagull/social-relationship-analysis/backend/internal/collectors/qzone"
	"github.com/seagull/social-relationship-analysis/backend/internal/domain"
)

func (s *Server) qzoneConnections(w http.ResponseWriter, r *http.Request) {
	values, err := s.Repo.ListQZoneConnections(r.Context())
	if err != nil {
		writeError(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]any{"data": values})
}

func (s *Server) qzoneConnection(w http.ResponseWriter, r *http.Request) {
	value, err := s.Repo.GetQZoneConnection(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		if err == pgx.ErrNoRows {
			writeJSON(w, 404, map[string]string{"error": "QZone connection not configured"})
			return
		}
		writeError(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]any{"data": value})
}

func (s *Server) upsertQZoneConnection(w http.ResponseWriter, r *http.Request) {
	accountID := chi.URLParam(r, "id")
	account, err := s.Repo.GetAccount(r.Context(), accountID)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "account not found"})
		return
	}
	var in struct {
		HTTPURL     string `json:"http_url"`
		WSURL       string `json:"ws_url"`
		AccessToken string `json:"access_token"`
		Enabled     *bool  `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, 400, err)
		return
	}
	if in.HTTPURL == "" {
		in.HTTPURL = "http://127.0.0.1:5700"
	}
	if in.WSURL == "" {
		in.WSURL = "ws://127.0.0.1:5700/event"
	}
	if !strings.HasPrefix(in.HTTPURL, "http://") && !strings.HasPrefix(in.HTTPURL, "https://") {
		writeJSON(w, 400, map[string]string{"error": "http_url must use http or https"})
		return
	}
	if !strings.HasPrefix(in.WSURL, "ws://") && !strings.HasPrefix(in.WSURL, "wss://") {
		writeJSON(w, 400, map[string]string{"error": "ws_url must use ws or wss"})
		return
	}
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	value, err := s.Repo.UpsertQZoneConnection(r.Context(), domain.QZoneConnection{
		AccountID: accountID, HTTPURL: in.HTTPURL, WSURL: in.WSURL, AccessToken: in.AccessToken, Enabled: enabled,
	})
	if err != nil {
		writeError(w, 500, err)
		return
	}
	if s.QZone != nil {
		s.QZone.Disconnect(accountID)
		if value.Enabled {
			full, loadErr := s.Repo.GetQZoneConnection(r.Context(), accountID)
			if loadErr == nil {
				_ = s.QZone.Connect(r.Context(), account, full)
			}
		}
	}
	writeJSON(w, 200, map[string]any{"data": value})
}

func (s *Server) testQZoneConnection(w http.ResponseWriter, r *http.Request) {
	connection, err := s.Repo.GetQZoneConnection(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "QZone connection not configured"})
		return
	}
	data, err := qzone.NewHTTPClient(connection.HTTPURL, connection.AccessToken).Call(r.Context(), "get_login_info", map[string]any{})
	if err != nil {
		writeJSON(w, 502, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"data": json.RawMessage(data)})
}

func (s *Server) connectQZone(w http.ResponseWriter, r *http.Request) {
	accountID := chi.URLParam(r, "id")
	account, err := s.Repo.GetAccount(r.Context(), accountID)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "account not found"})
		return
	}
	connection, err := s.Repo.GetQZoneConnection(r.Context(), accountID)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "QZone connection not configured"})
		return
	}
	if s.QZone == nil {
		writeJSON(w, 503, map[string]string{"error": "QZone manager unavailable"})
		return
	}
	if err := s.QZone.Connect(r.Context(), account, connection); err != nil {
		writeError(w, 500, err)
		return
	}
	writeJSON(w, 202, map[string]string{"status": "connecting"})
}

func (s *Server) disconnectQZone(w http.ResponseWriter, r *http.Request) {
	if s.QZone != nil {
		s.QZone.Disconnect(chi.URLParam(r, "id"))
	}
	writeJSON(w, 200, map[string]string{"status": "disconnected"})
}
