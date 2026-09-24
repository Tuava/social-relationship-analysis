package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/seagull/social-relationship-analysis/backend/internal/domain"
)

type CollectionRunEvent struct {
	ID          string   `json:"id"`
	Type        string   `json:"type"`
	Timestamp   string   `json:"timestamp"`
	Icon        string   `json:"icon"`
	Color       string   `json:"color"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	TargetQQ    string   `json:"target_qq,omitempty"`
	TargetName  string   `json:"target_name,omitempty"`
	Avatar      string   `json:"avatar,omitempty"`
	Depth       int      `json:"depth,omitempty"`
	Context     string   `json:"context,omitempty"`
	Count       int      `json:"count,omitempty"`
}

func (s *Server) collectionRunEvents(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "id")
	events := make([]CollectionRunEvent, 0, 60)

	// 1. Module progress events
	modules, _ := s.Repo.ListCollectionModules(r.Context(), runID)
	for _, m := range modules {
		if m.Status == "waiting" || m.Status == "skipped" {
			continue
		}
		icon := "mdi-database-outline"
		color := "info"
		title := m.Module
		desc := ""
		switch m.Module {
		case "qzone_profile":
			icon = "mdi-account-outline"
			title = "空间资料"
			desc = "目标账号空间资料读取"
		case "qzone_posts":
			icon = "mdi-image-text"
			title = "空间动态"
			desc = fmt.Sprintf("已采集 %d 条说说", m.RecordsCollected)
		case "qzone_comments":
			icon = "mdi-comment-outline"
			color = "orange"
			title = "动态评论"
			desc = fmt.Sprintf("已捕获 %d 条互动评论", m.RecordsCollected)
		case "qzone_likes":
			icon = "mdi-thumb-up-outline"
			color = "pink"
			title = "点赞穿透"
			desc = fmt.Sprintf("已穿透提取 %d 个点赞", m.RecordsCollected)
		case "friends":
			icon = "mdi-account-multiple-outline"
			color = "teal"
			title = "好友基线"
			desc = fmt.Sprintf("已同步 %d 个好友", m.RecordsCollected)
		case "qzone_visitors":
			icon = "mdi-eye-outline"
			color = "cyan"
			title = "空间访客"
			desc = fmt.Sprintf("已捕获 %d 位访客", m.RecordsCollected)
		case "media":
			icon = "mdi-folder-image"
			color = "deep-purple"
			title = "媒体资产"
			desc = fmt.Sprintf("已归档 %d 份高清资产", m.RecordsCollected)
		case "candidate_queue":
			icon = "mdi-vector-link"
			color = "primary"
			title = "候选人队列"
			desc = fmt.Sprintf("已处理 %d 个扩散节点", m.PagesCompleted)
		}
		if m.Error != "" {
			color = "warning"
			desc += " · " + m.Error
		}
		events = append(events, CollectionRunEvent{
			ID:          "mod-" + m.Module,
			Type:        "module",
			Timestamp:   m.UpdatedAt.Format(time.RFC3339),
			Icon:        icon,
			Color:       color,
			Title:       title,
			Description: desc,
			Count:       int(m.RecordsCollected),
		})
	}

	// 2. Discovered Candidates events (ordered by last_discovered_at DESC)
	rows, err := s.Repo.DB.Query(r.Context(), `
		SELECT cc.id::text, cc.entity_id, cc.depth, COALESCE(cc.contexts[1], ''), cc.last_discovered_at,
		       COALESCE(p.display_name, cc.entity_id),
		       COALESCE((SELECT '/api/v1/media/assets/'||mr.asset_id FROM media_references mr WHERE mr.person_id=p.id AND mr.media_kind='avatar' AND mr.status='completed' LIMIT 1), '')
		FROM collection_candidates cc
		LEFT JOIN person_identifiers pi ON pi.platform='qq' AND pi.platform_user_id=cc.entity_id
		LEFT JOIN persons p ON p.id=pi.person_id
		WHERE cc.run_id=$1
		ORDER BY cc.last_discovered_at DESC LIMIT 50`, runID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var id, entityID, ctxType, name, avatar string
			var depth int
			var t time.Time
			if rows.Scan(&id, &entityID, &depth, &ctxType, &t, &name, &avatar) == nil {
				icon := "mdi-account-plus-outline"
				color := "primary"
				title := "发现候选人"
				switch ctxType {
				case "like":
					icon = "mdi-thumb-up-outline"
					color = "pink"
					title = "点赞互动人"
				case "comment":
					icon = "mdi-comment-outline"
					color = "orange"
					title = "评论互动人"
				case "mention":
					icon = "mdi-at"
					color = "amber-darken-2"
					title = "被艾特提及人"
				case "visitor":
					icon = "mdi-eye-outline"
					color = "cyan"
					title = "空间访客"
				}
				events = append(events, CollectionRunEvent{
					ID:          "cand-" + id,
					Type:        "candidate",
					Timestamp:   t.Format(time.RFC3339),
					Icon:        icon,
					Color:       color,
					Title:       title,
					Description: fmt.Sprintf("%s (%s) · 深度 %d", name, entityID, depth),
					TargetQQ:    entityID,
					TargetName:  name,
					Avatar:      avatar,
					Depth:       depth,
					Context:     ctxType,
				})
			}
		}
	}

	// Sort all events by timestamp desc
	sort.Slice(events, func(i, j int) bool {
		return events[i].Timestamp > events[j].Timestamp
	})

	writeJSON(w, http.StatusOK, map[string]any{"data": events, "count": len(events)})
}

func (s *Server) collectionRunModules(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "id")
	modules, err := s.Repo.ListCollectionModules(r.Context(), runID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": modules})
}

func (s *Server) collectionRunCandidates(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	items, total, err := s.Repo.ListCollectionCandidates(r.Context(), chi.URLParam(r, "id"), r.URL.Query().Get("state"), limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	var selected int
	_ = s.Repo.DB.QueryRow(r.Context(), `SELECT count(*) FROM collection_candidates WHERE run_id=$1 AND selected=true AND state='expandable'`, chi.URLParam(r, "id")).Scan(&selected)
	writeJSON(w, http.StatusOK, map[string]any{"data": items, "total": total, "selected": selected, "limit": limit, "offset": offset})
}

func (s *Server) updateCollectionRunCandidates(w http.ResponseWriter, r *http.Request) {
	var input struct {
		IDs      []string `json:"ids"`
		State    string   `json:"state"`
		Selected *bool    `json:"selected,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	updated, err := s.Repo.UpdateCollectionCandidates(r.Context(), chi.URLParam(r, "id"), input.IDs, input.State, input.Selected)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"updated": updated})
}

func (s *Server) updateCollectionRunCandidate(w http.ResponseWriter, r *http.Request) {
	var input struct {
		State    string `json:"state"`
		Selected *bool  `json:"selected,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.Repo.UpdateCollectionCandidate(r.Context(), chi.URLParam(r, "id"), chi.URLParam(r, "candidateID"), input.State, input.Selected); err != nil {
		if err == pgx.ErrNoRows {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "candidate not found"})
			return
		}
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": input.State})
}

func (s *Server) continueCollectionRun(w http.ResponseWriter, r *http.Request) {
	parent, err := s.Repo.GetCollectionRun(r.Context(), chi.URLParam(r, "id"))
	if err != nil || parent.AccountID == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "collection run not found"})
		return
	}
	account, err := s.Repo.GetAccount(r.Context(), *parent.AccountID)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	var active bool
	if err := s.Repo.DB.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM collection_runs WHERE account_id=$1 AND status IN ('queued','running'))`, account.ID).Scan(&active); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if active {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "该账号已有采集任务运行中"})
		return
	}
	var config struct {
		Scope domain.CollectionScope `json:"scope"`
	}
	_ = json.Unmarshal(parent.Config, &config)
	scope := config.Scope.Normalize(account.QQUIN)

	// Check if there are any pending candidates that haven't been expanded yet
	var pendingCount int
	_ = s.Repo.DB.QueryRow(r.Context(), `
		SELECT count(*) FROM collection_candidates
		WHERE run_id=$1 AND entity_type='qq' AND state IN ('expandable', 'collected', 'discovered')`, parent.ID).Scan(&pendingCount)

	if pendingCount == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "候选池中已无待扩散的候选人"})
		return
	}

	// Promote any collected/discovered candidates to expandable so the queue picks them up
	_, _ = s.Repo.DB.Exec(r.Context(), `
		UPDATE collection_candidates
		SET state='expandable'
		WHERE run_id=$1 AND entity_type='qq' AND state IN ('collected', 'discovered')`, parent.ID)

	// Update existing run in-place to resume
	_, _ = s.Repo.DB.Exec(r.Context(), `
		UPDATE collection_runs
		SET status='running', error=NULL, ended_at=NULL, updated_at=now()
		WHERE id=$1`, parent.ID)

	parent.Status = "running"
	parent.Error = nil
	s.startCollectionRun(parent, account, scope)
	writeJSON(w, http.StatusAccepted, map[string]any{"data": parent})
}
