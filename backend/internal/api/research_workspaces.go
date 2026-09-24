package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/seagull/social-relationship-analysis/backend/internal/auth"
)

type workspaceInput struct {
	Name    string          `json:"name"`
	State   json.RawMessage `json:"state"`
	Version int             `json:"version"`
}

func (s *Server) researchDraft(w http.ResponseWriter, r *http.Request) {
	row := s.Repo.DB.QueryRow(r.Context(), `SELECT rw.id::text,rw.name,rw.state,rw.version,rw.updated_at
		FROM research_workspaces rw JOIN app_users u ON u.id=rw.user_id
		WHERE u.username=$1 AND rw.is_draft`, auth.Username(r.Context()))
	var id, name string
	var state []byte
	var version int
	var updatedAt any
	if err := row.Scan(&id, &name, &state, &version, &updatedAt); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"data": nil})
		return
	}
	writeWorkspace(w, http.StatusOK, id, name, state, version, true, updatedAt)
}

func (s *Server) updateResearchDraft(w http.ResponseWriter, r *http.Request) {
	var in workspaceInput
	if err := decodeWorkspaceInput(w, r, &in); err != nil {
		return
	}
	var id string
	var state []byte
	var version int
	var updatedAt any
	err := s.Repo.DB.QueryRow(r.Context(), `INSERT INTO research_workspaces(user_id,name,state,is_draft)
		SELECT id,'',$2,true FROM app_users WHERE username=$1
		ON CONFLICT(user_id) WHERE is_draft DO UPDATE SET state=EXCLUDED.state,
			version=research_workspaces.version+1,updated_at=now()
		RETURNING id::text,state,version,updated_at`, auth.Username(r.Context()), in.State).
		Scan(&id, &state, &version, &updatedAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeWorkspace(w, http.StatusOK, id, "", state, version, true, updatedAt)
}

func (s *Server) researchSnapshots(w http.ResponseWriter, r *http.Request) {
	rows, err := s.Repo.DB.Query(r.Context(), `SELECT rw.id::text,rw.name,rw.state,rw.version,rw.updated_at
		FROM research_workspaces rw JOIN app_users u ON u.id=rw.user_id
		WHERE u.username=$1 AND NOT rw.is_draft ORDER BY rw.updated_at DESC LIMIT 100`, auth.Username(r.Context()))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, name string
		var raw []byte
		var version int
		var updatedAt any
		if err := rows.Scan(&id, &name, &raw, &version, &updatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		var state any
		_ = json.Unmarshal(raw, &state)
		items = append(items, map[string]any{"id": id, "name": name, "state": state, "version": version, "updated_at": updatedAt})
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": items})
}

func (s *Server) createResearchSnapshot(w http.ResponseWriter, r *http.Request) {
	var in workspaceInput
	if err := decodeWorkspaceInput(w, r, &in); err != nil {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}
	var id string
	var state []byte
	var version int
	var updatedAt any
	err := s.Repo.DB.QueryRow(r.Context(), `INSERT INTO research_workspaces(user_id,name,state,is_draft)
		SELECT id,$2,$3,false FROM app_users WHERE username=$1
		RETURNING id::text,state,version,updated_at`, auth.Username(r.Context()), in.Name, in.State).
		Scan(&id, &state, &version, &updatedAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeWorkspace(w, http.StatusCreated, id, in.Name, state, version, false, updatedAt)
}

func (s *Server) deleteResearchSnapshot(w http.ResponseWriter, r *http.Request) {
	command, err := s.Repo.DB.Exec(r.Context(), `DELETE FROM research_workspaces rw USING app_users u
		WHERE rw.user_id=u.id AND u.username=$1 AND rw.id=$2 AND NOT rw.is_draft`, auth.Username(r.Context()), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if command.RowsAffected() == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "snapshot not found"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func decodeWorkspaceInput(w http.ResponseWriter, r *http.Request, in *workspaceInput) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(in); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return err
	}
	if len(in.State) == 0 || !json.Valid(in.State) {
		err := &workspaceValidationError{"state must be valid JSON"}
		writeError(w, http.StatusBadRequest, err)
		return err
	}
	return nil
}

type workspaceValidationError struct{ message string }

func (e *workspaceValidationError) Error() string { return e.message }

func writeWorkspace(w http.ResponseWriter, status int, id, name string, raw []byte, version int, draft bool, updatedAt any) {
	var state any
	_ = json.Unmarshal(raw, &state)
	writeJSON(w, status, map[string]any{"data": map[string]any{
		"id": id, "name": name, "state": state, "version": version, "is_draft": draft, "updated_at": updatedAt,
	}})
}
