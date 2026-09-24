package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/seagull/social-relationship-analysis/backend/internal/analysis"
	"github.com/seagull/social-relationship-analysis/backend/internal/auth"
	"github.com/seagull/social-relationship-analysis/backend/internal/bot"
	"github.com/seagull/social-relationship-analysis/backend/internal/collectors/napcat"
	"github.com/seagull/social-relationship-analysis/backend/internal/collectors/qzone"
	"github.com/seagull/social-relationship-analysis/backend/internal/domain"
	"github.com/seagull/social-relationship-analysis/backend/internal/persistence"
)

type Server struct {
	Repo                persistence.Repository
	Auth                auth.Service
	NapCat              *napcat.Manager
	QZone               *qzone.Manager
	Bot                 *bot.Service
	Capabilities        []domain.Capability
	Logger              *slog.Logger
	Collector           napcat.Collector
	QZoneCollector      qzone.Collector
	Graph               analysis.EgoBuilder
	CORSOrigin          string
	ObjectRoot          string
	NapCatMediaRoot     string
	FrontendDist        string
	GraphMaxDepth       int
	GraphBuildMaxNodes  int
	GraphBuildMaxEdges  int
	GraphBuildMaxEvents int
	GraphViewMaxNodes   int
	GraphExpandMaxNodes int
	RoutingPolicy       analysis.RoutingPolicy
	PipelineMgr         *analysis.PipelineManager
	runMu               sync.Mutex
	runCancel           map[string]context.CancelFunc
}

func (s *Server) graphMaxDepth(ctx context.Context) int {
	if s.GraphMaxDepth > 0 {
		return s.GraphMaxDepth
	}
	return 0
}

func (s *Server) graphViewMaxNodes() int {
	if s.GraphViewMaxNodes > 0 {
		return s.GraphViewMaxNodes
	}
	return 0
}

func (s *Server) graphExpandMaxNodes() int {
	if s.GraphExpandMaxNodes > 0 {
		return s.GraphExpandMaxNodes
	}
	return 0
}

func (s *Server) graphBuildLimit(value, configured int) int {
	if configured <= 0 {
		return value
	}
	if value <= 0 || value > configured {
		return configured
	}
	return value
}

func (s *Server) graphBuildOptions(nodes, edges, events int) analysis.BuildOptions {
	return (analysis.BuildOptions{
		MaxNodes:  s.graphBuildLimit(nodes, s.GraphBuildMaxNodes),
		MaxEdges:  s.graphBuildLimit(edges, s.GraphBuildMaxEdges),
		MaxEvents: s.graphBuildLimit(events, s.GraphBuildMaxEvents),
	}).WithDefaults()
}

func optionalLimit(value int) any {
	if value <= 0 {
		return nil
	}
	return value
}

func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(s.cors)
	r.Get("/health", s.health)
	r.Get("/api/v1/system/capabilities", s.systemCapabilities)
	r.Get("/api/v1/napcat/capabilities", s.capabilities)
	r.Post("/api/v1/auth/login", s.login)
	r.Group(func(r chi.Router) {
		r.Use(s.Auth.Middleware)
		r.Get("/api/v1/auth/me", s.me)
		r.Post("/api/v1/auth/refresh", s.refresh)
		r.Post("/api/v1/auth/logout", s.logout)
		r.Get("/api/v1/accounts", s.accounts)
		r.Post("/api/v1/accounts", s.createAccount)
		r.Get("/api/v1/accounts/{id}", s.account)
		r.Patch("/api/v1/accounts/{id}", s.updateAccount)
		r.Delete("/api/v1/accounts/{id}", s.deleteAccount)
		r.Post("/api/v1/accounts/{id}/test", s.testAccount)
		r.Post("/api/v1/accounts/{id}/connect", s.connectAccount)
		r.Post("/api/v1/accounts/{id}/disconnect", s.disconnectAccount)
		r.Get("/api/v1/qzone/connections", s.qzoneConnections)
		r.Get("/api/v1/accounts/{id}/qzone", s.qzoneConnection)
		r.Put("/api/v1/accounts/{id}/qzone", s.upsertQZoneConnection)
		r.Post("/api/v1/accounts/{id}/qzone/test", s.testQZoneConnection)
		r.Post("/api/v1/accounts/{id}/qzone/connect", s.connectQZone)
		r.Post("/api/v1/accounts/{id}/qzone/disconnect", s.disconnectQZone)
		r.Get("/api/v1/collection-runs", s.collectionRuns)
		r.Post("/api/v1/collection-runs", s.createCollectionRun)
		r.Get("/api/v1/collection-runs/{id}", s.collectionRun)
		r.Get("/api/v1/collection-runs/{id}/modules", s.collectionRunModules)
		r.Get("/api/v1/collection-runs/{id}/events", s.collectionRunEvents)
		r.Get("/api/v1/collection-runs/{id}/candidates", s.collectionRunCandidates)
		r.Patch("/api/v1/collection-runs/{id}/candidates", s.updateCollectionRunCandidates)
		r.Patch("/api/v1/collection-runs/{id}/candidates/{candidateID}", s.updateCollectionRunCandidate)
		r.Post("/api/v1/collection-runs/{id}/continue", s.continueCollectionRun)
		r.Post("/api/v1/collection-runs/{id}/cancel", s.cancelCollectionRun)
		r.Get("/api/v1/overview", s.overview)
		r.Get("/api/v1/realtime/stats", s.realtimeStats)
		r.Get("/api/v1/media/stats", s.mediaStats)
		r.Get("/api/v1/media/downloads", s.mediaDownloads)
		r.Patch("/api/v1/media/downloads", s.updateMediaDownloads)
		r.Get("/api/v1/media/references", s.mediaReferences)
		r.Post("/api/v1/media/references/{id}/download", s.selectMediaDownload)
		r.Post("/api/v1/media/references/retry", s.retryMediaDownloads)
		r.Get("/api/v1/media/avatars/{kind}/{id}", s.remoteAvatar)
		r.Get("/api/v1/media/chat-image", s.chatImage)
		r.Get("/api/v1/media/assets/{id}", s.mediaAsset)
		r.Get("/api/v1/media/ocr-search", s.mediaOCRSearch)
		r.Get("/api/v1/media/{id}/diffusion", s.mediaDiffusion)
		r.Post("/api/v1/media/{id}/analyze", s.analyzeMediaAsset)
		r.Get("/api/v1/conversations/{id}/threads", s.conversationThreads)
		r.Post("/api/v1/conversations/{id}/disentangle", s.disentangleConversation)
		r.Get("/api/v1/threads/{id}", s.threadDetail)
		r.Get("/api/v1/persons/{id}/persona", s.personPersona)
		r.Post("/api/v1/persons/{id}/analyze-persona", s.analyzePersonPersona)
		r.Get("/api/v1/persons/{id}/feed-dynamics", s.personFeedDynamics)
		r.Post("/api/v1/persons/{id}/analyze-feeds", s.analyzePersonFeedDynamics)
		r.Get("/api/v1/pipelines/batch-persona", s.listPersonaPipelines)
		r.Post("/api/v1/pipelines/batch-persona", s.createPersonaPipeline)
		r.Get("/api/v1/pipelines/batch-persona/{id}", s.getPersonaPipeline)
		r.Post("/api/v1/pipelines/batch-persona/{id}/cancel", s.cancelPersonaPipeline)
		r.Post("/api/v1/pipelines/batch-persona/{id}/retry-failed", s.retryFailedPersonaPipeline)
		r.Get("/api/v1/pipelines/batch-persona/{id}/summary", s.getPersonaPipelineSummary)
		r.Get("/api/v1/persons", s.persons)
		r.Get("/api/v1/persons/{id}", s.person)
		r.Get("/api/v1/persons/{id}/relationship", s.relationship)
		r.Get("/api/v1/persons/{id}/relationship-deep", s.relationshipDeep)
		r.Get("/api/v1/persons/{id}/coverage", s.coverage)
		r.Post("/api/v1/persons/{id}/collect", s.collectPerson)
		r.Get("/api/v1/persons/{id}/timeline", s.timeline)
		r.Post("/api/v1/analysis/routes", s.planRoutes)
		r.Post("/api/v1/analysis/evidence-details", s.evidenceDetails)
		r.Get("/api/v1/groups", s.groups)
		r.Get("/api/v1/groups/{id}", s.group)
		r.Get("/api/v1/groups/{id}/members", s.groupMembers)
		r.Get("/api/v1/groups/{id}/messages", s.groupMessages)
		r.Get("/api/v1/groups/{id}/files", s.groupFiles)
		r.Get("/api/v1/groups/{id}/albums", s.groupAlbums)
		r.Get("/api/v1/groups/{id}/notices", s.groupNotices)
		r.Get("/api/v1/groups/{id}/honor", s.groupHonor)
		r.Get("/api/v1/recent-contacts", s.recentContacts)
		r.Get("/api/v1/groups/{id}/members/{qq}/info", s.groupMemberInfo)
		r.Get("/api/v1/conversations", s.conversations)
		r.Get("/api/v1/conversations/{id}", s.conversation)
		r.Get("/api/v1/conversations/{id}/messages", s.conversationMessages)
		r.Get("/api/v1/conversations/{id}/context", s.conversationContext)
		r.Post("/api/v1/conversations/{id}/send", s.sendMessage)
		r.Post("/api/v1/messages/{id}/recall", s.recallMessage)
		r.Post("/api/v1/messages/{id}/reaction", s.reactMessage)
		r.Post("/api/v1/messages/{id}/essence", s.essenceMessage)
		r.Get("/api/v1/system/settings", s.systemSettingsGet)
		r.Put("/api/v1/system/settings", s.systemSettingsUpdate)
		r.Post("/api/v1/system/settings/test-llm", s.systemTestLLM)
		r.Get("/api/v1/bot/instances", s.botInstances)
		r.Post("/api/v1/bot/instances", s.botCreateInstance)
		r.Patch("/api/v1/bot/instances/{id}", s.botUpdateInstance)
		r.Delete("/api/v1/bot/instances/{id}", s.botDeleteInstance)
		r.Post("/api/v1/bot/instances/{id}/chat", s.botChatTest)
		r.Get("/api/v1/bot/instances/{id}/sessions", s.botListSessions)
		r.Post("/api/v1/bot/instances/{id}/sessions/inject", s.botInjectSessionMemory)
		r.Delete("/api/v1/bot/instances/{id}/session", s.botClearSession)
		r.Delete("/api/v1/bot/instances/{id}/sessions", s.botClearSession)
		r.Get("/api/v1/bot/instances/{id}/groups", s.botWhitelist)
		r.Post("/api/v1/bot/instances/{id}/groups", s.botAddWhitelist)
		r.Delete("/api/v1/bot/instances/{id}/groups/{group_id}", s.botDeleteWhitelist)
		r.Get("/api/v1/bot/instances/{id}/users", s.botUsersWhitelist)
		r.Post("/api/v1/bot/instances/{id}/users", s.botAddUserWhitelist)
		r.Delete("/api/v1/bot/instances/{id}/users/{user_qq}", s.botDeleteUserWhitelist)
		r.Get("/api/v1/bot/audit", s.botAudit)
		r.Get("/api/v1/messages", s.messages)
		r.Get("/api/v1/messages/{id}", s.message)
		r.Get("/api/v1/contents", s.contents)
		r.Get("/api/v1/contents/{id}", s.content)
		r.Get("/api/v1/contents/{id}/likes", s.contentLikes)
		r.Get("/api/v1/relation-events", s.relationEvents)
		r.Get("/api/v1/raw-records/{id}", s.rawRecord)
		r.Get("/api/v1/napcat/capabilities/{endpoint:.*}", s.capability)
		r.Post("/api/v1/ego-networks", s.egoPreview)
		r.Post("/api/v1/ego-networks/preview", s.egoPreview)
		r.Get("/api/v1/ego-networks", s.egoNetworks)
		r.Get("/api/v1/ego-networks/{id}", s.egoNetwork)
		r.Get("/api/v1/ego-networks/{id}/nodes", s.egoNetworkNodes)
		r.Get("/api/v1/ego-networks/{id}/edges", s.egoNetworkEdges)
		r.Get("/api/v1/ego-networks/{id}/evidence", s.egoNetworkEvidence)
		r.Get("/api/v1/ego-networks/{id}/view", s.egoNetworkView)
		r.Get("/api/v1/ego-networks/{id}/expand", s.expandEgoNetworkNode)
		r.Get("/api/v1/ego-networks/{id}/communities", s.egoNetworkCommunities)
		r.Get("/api/v1/ego-networks/{id}/communities/{communityID}", s.egoNetworkCommunity)
		r.Get("/api/v1/research-workspaces/draft", s.researchDraft)
		r.Put("/api/v1/research-workspaces/draft", s.updateResearchDraft)
		r.Get("/api/v1/research-workspaces/snapshots", s.researchSnapshots)
		r.Post("/api/v1/research-workspaces/snapshots", s.createResearchSnapshot)
		r.Delete("/api/v1/research-workspaces/snapshots/{id}", s.deleteResearchSnapshot)
		r.Post("/api/v1/operations/preview", s.operationPreview)
		r.Get("/api/v1/operations", s.operations)
		r.Post("/api/v1/operations/{id}/confirm", s.confirmOperation)
		r.Post("/api/v1/operations/{id}/cancel", s.cancelOperation)
		r.Get("/api/v1/operation-audits", s.operationAudits)
		r.Post("/api/v1/ai/runs", s.createAIRun)
		r.Get("/api/v1/ai/runs", s.aiRuns)
		r.Get("/api/v1/ai/runs/{id}", s.aiRun)
		r.Get("/api/v1/ai/evidence-packs/{id}", s.evidencePack)
	})
	if s.FrontendDist != "" {
		r.Get("/*", s.serveFrontend)
	}
	return r
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"status": "ok", "service": "sra-backend"})
}
func (s *Server) capabilities(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"data": s.Capabilities, "count": len(s.Capabilities)})
}
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, 400, err)
		return
	}
	u, err := s.Repo.FindUser(r.Context(), in.Username)
	if err != nil || !s.Auth.CheckPassword(u.PasswordHash, in.Password) {
		writeJSON(w, 401, map[string]string{"error": "invalid credentials"})
		return
	}
	token, err := s.Auth.Issue(u.Username)
	if err != nil {
		writeError(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]any{"token": token, "user": map[string]string{"username": u.Username}})
}
func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]string{"username": auth.Username(r.Context())})
}
func (s *Server) refresh(w http.ResponseWriter, r *http.Request) {
	username := auth.Username(r.Context())
	token, err := s.Auth.Issue(username)
	if err != nil {
		writeError(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]any{"token": token, "user": map[string]string{"username": username}})
}
func (s *Server) logout(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }
func (s *Server) accounts(w http.ResponseWriter, r *http.Request) {
	v, err := s.Repo.ListAccounts(r.Context())
	if err != nil {
		writeError(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]any{"data": v})
}
func (s *Server) createAccount(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name      string `json:"name"`
		QQUIN     string `json:"qq_uin"`
		WSURL     string `json:"ws_url"`
		WSToken   string `json:"ws_token"`
		HTTPURL   string `json:"http_url"`
		HTTPToken string `json:"http_token"`
		Enabled   *bool  `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, 400, err)
		return
	}
	a := domain.NapCatAccount{Name: in.Name, QQUIN: in.QQUIN, WSURL: in.WSURL, WSToken: in.WSToken, HTTPURL: in.HTTPURL, HTTPToken: in.HTTPToken}
	if strings.TrimSpace(a.Name) == "" || strings.TrimSpace(a.WSURL) == "" {
		writeJSON(w, 400, map[string]string{"error": "name and ws_url are required"})
		return
	}
	a.Enabled = true
	if in.Enabled != nil {
		a.Enabled = *in.Enabled
	}
	v, err := s.Repo.CreateAccount(r.Context(), a)
	if err != nil {
		writeError(w, 500, err)
		return
	}
	if v.Enabled {
		fullAccount, loadErr := s.Repo.GetAccount(r.Context(), v.ID)
		if loadErr == nil {
			_ = s.NapCat.Connect(r.Context(), fullAccount)
		}
	}
	writeJSON(w, 201, map[string]any{"data": v})
}
func (s *Server) account(w http.ResponseWriter, r *http.Request) {
	v, err := s.Repo.GetAccount(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		if err == pgx.ErrNoRows {
			writeJSON(w, 404, map[string]string{"error": "account not found"})
			return
		}
		writeError(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]any{"data": v})
}
func (s *Server) updateAccount(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name      string `json:"name"`
		QQUIN     string `json:"qq_uin"`
		WSURL     string `json:"ws_url"`
		WSToken   string `json:"ws_token"`
		HTTPURL   string `json:"http_url"`
		HTTPToken string `json:"http_token"`
		Enabled   bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, 400, err)
		return
	}
	a := domain.NapCatAccount{Name: in.Name, QQUIN: in.QQUIN, WSURL: in.WSURL, WSToken: in.WSToken, HTTPURL: in.HTTPURL, HTTPToken: in.HTTPToken, Enabled: in.Enabled}
	if err := s.Repo.UpdateAccount(r.Context(), chi.URLParam(r, "id"), a); err != nil {
		writeError(w, 500, err)
		return
	}
	a, err := s.Repo.GetAccount(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, 500, err)
		return
	}
	if a.Enabled {
		s.NapCat.Disconnect(a.ID)
		_ = s.NapCat.Connect(r.Context(), a)
	} else {
		s.NapCat.Disconnect(a.ID)
	}
	writeJSON(w, 200, map[string]any{"data": a})
}
func (s *Server) deleteAccount(w http.ResponseWriter, r *http.Request) {
	s.NapCat.Disconnect(chi.URLParam(r, "id"))
	if s.QZone != nil {
		s.QZone.Disconnect(chi.URLParam(r, "id"))
	}
	if err := s.Repo.DeleteAccount(r.Context(), chi.URLParam(r, "id")); err != nil {
		writeError(w, 500, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) testAccount(w http.ResponseWriter, r *http.Request) {
	a, err := s.Repo.GetAccount(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, 404, err)
		return
	}
	if a.HTTPURL == "" {
		writeJSON(w, 400, map[string]string{"error": "http_url is required for API test"})
		return
	}
	data, err := napcat.NewHTTPClient(a.HTTPURL, a.HTTPToken).Call(r.Context(), "/get_login_info", map[string]any{})
	if err != nil {
		writeJSON(w, 502, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"data": data})
}
func (s *Server) connectAccount(w http.ResponseWriter, r *http.Request) {
	a, err := s.Repo.GetAccount(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, 404, err)
		return
	}
	if a.WSURL == "" {
		writeJSON(w, 400, map[string]string{"error": "ws_url is required"})
		return
	}
	if err := s.NapCat.Connect(r.Context(), a); err != nil {
		writeError(w, 500, err)
		return
	}
	writeJSON(w, 202, map[string]string{"status": "connecting"})
}
func (s *Server) disconnectAccount(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	s.NapCat.Disconnect(id)
	writeJSON(w, 200, map[string]string{"status": "disconnected"})
}
func (s *Server) collectionRuns(w http.ResponseWriter, r *http.Request) {
	v, err := s.Repo.ListCollectionRuns(r.Context())
	if err != nil {
		writeError(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]any{"data": v})
}

func (s *Server) createCollectionRun(w http.ResponseWriter, r *http.Request) {
	var in struct {
		AccountID string                 `json:"account_id"`
		Type      string                 `json:"type"`
		Scope     domain.CollectionScope `json:"scope"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, 400, err)
		return
	}
	if in.Type == "" {
		in.Type = "all_sources_sync"
	}
	if in.Type != "all_sources_sync" && in.Type != "full_visible_data" && in.Type != "profile_sync" && in.Type != "qzone_sync" {
		writeJSON(w, 400, map[string]string{"error": "unsupported collection type"})
		return
	}
	a, err := s.Repo.GetAccount(r.Context(), in.AccountID)
	if err != nil {
		writeError(w, 404, err)
		return
	}
	var active bool
	if err := s.Repo.DB.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM collection_runs WHERE account_id=$1 AND type=$2 AND status IN ('queued','running'))`, a.ID, in.Type).Scan(&active); err != nil {
		writeError(w, 500, err)
		return
	}
	if active {
		typeLabel := map[string]string{
			"all_sources_sync":  "全源同步 (NapCat + QQ空间)",
			"full_visible_data": "全部可见数据 (NapCat)",
			"profile_sync":      "详细资料同步",
			"qzone_sync":        "QQ 空间同步",
		}[in.Type]
		if typeLabel == "" {
			typeLabel = "采集"
		}
		writeJSON(w, 409, map[string]string{"error": fmt.Sprintf("该账号已有「%s」任务正在运行中，请等待其完成或停止后再启动同类型任务", typeLabel)})
		return
	}
	scope := in.Scope.Normalize(a.QQUIN)
	run, err := s.Repo.CreateCollectionRunWithScope(r.Context(), &a.ID, in.Type, scope)
	if err != nil {
		writeError(w, 500, err)
		return
	}
	s.startCollectionRun(run, a, scope)
	writeJSON(w, 202, map[string]any{"data": run})
}

func (s *Server) startCollectionRun(run domain.CollectionRun, account domain.NapCatAccount, scope domain.CollectionScope) {
	moduleNames := map[string][]string{
		"all_sources_sync":  {"profile", "friends", "groups", "group_members", "group_messages", "private_messages", "qzone_profile", "qzone_posts", "qzone_comments", "qzone_likes", "qzone_visitors", "media", "candidate_queue"},
		"full_visible_data": {"profile", "friends", "groups", "group_members", "group_messages", "private_messages", "media", "candidate_queue"},
		"profile_sync":      {"profile"},
		"qzone_sync":        {"qzone_profile", "friends", "qzone_posts", "qzone_comments", "qzone_likes", "qzone_visitors", "media", "candidate_queue"},
	}[run.Type]
	for _, module := range moduleNames {
		_, _ = s.Repo.DB.Exec(context.Background(), `
			INSERT INTO collection_run_modules (run_id, module, status, pages_completed, records_collected, created_at, updated_at)
			VALUES ($1, $2, 'waiting', 0, 0, now(), now())
			ON CONFLICT (run_id, module) DO NOTHING`, run.ID, module)
	}
	runCtx, cancel := context.WithCancel(context.Background())
	s.runMu.Lock()
	if s.runCancel == nil {
		s.runCancel = make(map[string]context.CancelFunc)
	}
	s.runCancel[run.ID] = cancel
	s.runMu.Unlock()
	go func(runID, runType string, account domain.NapCatAccount, scope domain.CollectionScope, ctx context.Context) {
		defer func() { s.runMu.Lock(); delete(s.runCancel, runID); s.runMu.Unlock() }()
		_ = s.Repo.UpdateCollectionRun(ctx, runID, "running", 1, nil)
		observer := domain.CollectionObserver{
			Module: func(progress domain.CollectionModuleProgress) {
				_ = s.Repo.UpsertCollectionModule(context.Background(), runID, progress)
			},
			Candidate: func(candidate domain.CollectionCandidate) {
				_ = s.Repo.UpsertCollectionCandidate(context.Background(), runID, account.ID, candidate)
			},
		}
		var err error
		var completionNote *string
		if runType == "all_sources_sync" {
			// Phase 1: NapCat visible data
			napcatErr := s.Collector.RunVisibleDataWithScope(ctx, account, runID, scope, observer, func(progress int, stepErr error) {
				scaledProgress := progress / 2
				if stepErr != nil {
					msg := stepErr.Error()
					_ = s.Repo.UpdateCollectionRun(ctx, runID, "running", scaledProgress, &msg)
				} else {
					_ = s.Repo.UpdateCollectionRun(ctx, runID, "running", scaledProgress, nil)
				}
			})
			if napcatErr != nil && ctx.Err() == nil && s.Logger != nil {
				s.Logger.Warn("all_sources_sync: NapCat collection finished with warning", "error", napcatErr)
			}

			// Phase 2: QZone sync (if configured & enabled)
			if ctx.Err() == nil {
				connection, connectionErr := s.Repo.GetQZoneConnection(ctx, account.ID)
				if connectionErr == nil && connection.Enabled {
					qzoneErr := s.QZoneCollector.RunSyncWithScope(ctx, account, connection, runID, scope, observer, func(progress int) {
						scaledProgress := 50 + progress/2
						_ = s.Repo.UpdateCollectionRun(ctx, runID, "running", scaledProgress, nil)
					})
					if qzoneErr != nil && ctx.Err() == nil && s.Logger != nil {
						s.Logger.Warn("all_sources_sync: QZone collection finished with warning", "error", qzoneErr)
					}
					if err == nil {
						err = qzoneErr
					}
				} else {
					for _, mod := range []string{"qzone_profile", "qzone_posts", "qzone_comments", "qzone_likes", "qzone_visitors"} {
						observer.Module(domain.CollectionModuleProgress{
							Module: mod,
							Status: "complete",
							Error:  "未配置或未启用 QQ 空间连接，已跳过",
						})
					}
				}
			}
			if err == nil {
				err = napcatErr
			}
		} else if runType == "profile_sync" {
			observer.Module(domain.CollectionModuleProgress{Module: "profile", Status: "running"})
			result, syncErr := s.Collector.RunProfileSyncWithScope(ctx, account, scope, func(progress int) {
				_ = s.Repo.UpdateCollectionRun(ctx, runID, "running", progress, nil)
			})
			err = syncErr
			moduleStatus := "complete"
			moduleError := ""
			if syncErr != nil {
				moduleStatus = "failed"
				moduleError = syncErr.Error()
			} else if result.Failed > 0 {
				moduleStatus = "partial"
				moduleError = fmt.Sprintf("%d profiles failed", result.Failed)
			}
			observer.Module(domain.CollectionModuleProgress{Module: "profile", Status: moduleStatus, PagesCompleted: result.Synced + result.Failed, RecordsCollected: int64(result.Synced), Error: moduleError})
			if result.Failed > 0 && syncErr == nil {
				note := fmt.Sprintf("同步 %d，失败 %d，共 %d", result.Synced, result.Failed, result.Total)
				completionNote = &note
			}
		} else if runType == "qzone_sync" {
			connection, connectionErr := s.Repo.GetQZoneConnection(ctx, account.ID)
			if connectionErr != nil {
				err = fmt.Errorf("QZone connection is not configured: %w", connectionErr)
			} else if !connection.Enabled {
				err = fmt.Errorf("QZone connection is disabled")
			} else {
				err = s.QZoneCollector.RunSyncWithScope(ctx, account, connection, runID, scope, observer, func(progress int) {
					_ = s.Repo.UpdateCollectionRun(ctx, runID, "running", progress, nil)
				})
			}
		} else {
			err = s.Collector.RunVisibleDataWithScope(ctx, account, runID, scope, observer, func(progress int, stepErr error) {
				if stepErr != nil {
					msg := stepErr.Error()
					_ = s.Repo.UpdateCollectionRun(ctx, runID, "failed", progress, &msg)
				} else {
					_ = s.Repo.UpdateCollectionRun(ctx, runID, "running", progress, nil)
				}
			})
		}
		if ctx.Err() != nil {
			_, _ = s.Repo.DB.Exec(context.Background(), `UPDATE collection_run_modules SET status='cancelled',updated_at=now(),ended_at=now() WHERE run_id=$1 AND status IN ('waiting','running','retrying')`, runID)
			_, _ = s.Repo.DB.Exec(context.Background(), `UPDATE collection_runs SET status='cancelled',error=NULL,ended_at=now() WHERE id=$1`, runID)
		} else if err != nil {
			msg := err.Error()
			_, _ = s.Repo.DB.Exec(context.Background(), `
				UPDATE collection_run_modules
				SET status=CASE WHEN status='waiting' THEN 'cancelled' ELSE 'failed' END,
					error=CASE WHEN status='waiting' THEN error ELSE COALESCE(error,$2) END,
					updated_at=now(),ended_at=now()
				WHERE run_id=$1 AND status IN ('waiting','running','retrying')`, runID, msg)
			_ = s.Repo.UpdateCollectionRun(ctx, runID, "failed", 100, &msg)
		} else {
			var candidateCount int64
			if countErr := s.Repo.DB.QueryRow(context.Background(), `SELECT count(*) FROM collection_candidates WHERE run_id=$1`, runID).Scan(&candidateCount); countErr == nil {
				_ = s.Repo.UpsertCollectionModule(context.Background(), runID, domain.CollectionModuleProgress{Module: "candidate_queue", Status: "complete", PagesCompleted: 1, RecordsCollected: candidateCount})
			}
			if partial, partialErr := s.Repo.CollectionRunHasPartial(ctx, runID); partialErr == nil && partial {
				note := "部分模块未完整采集，请查看模块进度"
				_ = s.Repo.UpdateCollectionRun(ctx, runID, "partial", 100, &note)
			} else {
				_ = s.Repo.UpdateCollectionRun(ctx, runID, "completed", 100, completionNote)
			}
		}
	}(run.ID, run.Type, account, scope, runCtx)
}
func (s *Server) collectionRun(w http.ResponseWriter, r *http.Request) {
	v, err := s.Repo.GetCollectionRun(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		if err == pgx.ErrNoRows {
			writeJSON(w, 404, map[string]string{"error": "run not found"})
			return
		}
		writeError(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]any{"data": v})
}
func (s *Server) cancelCollectionRun(w http.ResponseWriter, r *http.Request) {
	s.runMu.Lock()
	if cancel := s.runCancel[chi.URLParam(r, "id")]; cancel != nil {
		cancel()
	}
	s.runMu.Unlock()
	if _, err := s.Repo.DB.Exec(r.Context(), `UPDATE collection_runs SET status='cancelled',error=NULL,ended_at=now() WHERE id=$1`, chi.URLParam(r, "id")); err != nil {
		writeError(w, 500, err)
		return
	}
	_, _ = s.Repo.DB.Exec(r.Context(), `UPDATE collection_run_modules SET status='cancelled',updated_at=now(),ended_at=now() WHERE run_id=$1 AND status IN ('waiting','running','retrying')`, chi.URLParam(r, "id"))
	writeJSON(w, 200, map[string]string{"status": "cancelled"})
}
func (s *Server) overview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	result := map[string]any{}
	for _, table := range []string{"napcat_accounts", "raw_records", "messages", "relation_events", "contents"} {
		n, err := s.Repo.Count(ctx, table)
		if err != nil {
			writeError(w, 500, err)
			return
		}
		result[table] = n
	}
	writeJSON(w, 200, map[string]any{
		"accounts":        result["napcat_accounts"],
		"raw_records":     result["raw_records"],
		"messages":        result["messages"],
		"relation_events": result["relation_events"],
		"contents":        result["contents"],
	})
}

func (s *Server) realtimeStats(w http.ResponseWriter, r *http.Request) {
	type point struct {
		Time  interface{} `json:"time"`
		Count int64       `json:"count"`
	}
	series := make([]point, 0, 60)
	rows, err := s.Repo.DB.Query(r.Context(), `WITH minutes AS (
		SELECT generate_series(date_trunc('minute',now())-interval '59 minutes',date_trunc('minute',now()),interval '1 minute') bucket
	), counts AS (
		SELECT date_trunc('minute',re.created_at) bucket,count(*) value
		FROM raw_events re JOIN raw_records rr ON rr.id=re.raw_record_id
		WHERE rr.source='napcat_ws' AND re.post_type='message' AND re.created_at>=now()-interval '60 minutes'
		GROUP BY 1
	) SELECT m.bucket,COALESCE(c.value,0) FROM minutes m LEFT JOIN counts c USING(bucket) ORDER BY m.bucket`)
	if err != nil {
		writeError(w, 500, err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var value point
		if err := rows.Scan(&value.Time, &value.Count); err != nil {
			writeError(w, 500, err)
			return
		}
		series = append(series, value)
	}
	var current, lastFive, total, connected int64
	var lastMessage interface{}
	err = s.Repo.DB.QueryRow(r.Context(), `SELECT
		count(*) FILTER(WHERE re.created_at>=date_trunc('minute',now())),
		count(*) FILTER(WHERE re.created_at>=now()-interval '5 minutes'),count(*),max(re.created_at)
		FROM raw_events re JOIN raw_records rr ON rr.id=re.raw_record_id
		WHERE rr.source='napcat_ws' AND re.post_type='message'`).Scan(&current, &lastFive, &total, &lastMessage)
	if err != nil {
		writeError(w, 500, err)
		return
	}
	_ = s.Repo.DB.QueryRow(r.Context(), `SELECT count(*) FROM source_connections WHERE kind='ws' AND status='connected'`).Scan(&connected)
	writeJSON(w, 200, map[string]any{"current_per_minute": current, "last_5_minutes": lastFive, "total_realtime_messages": total, "last_message_at": lastMessage, "connected_accounts": connected, "series": series})
}

func (s *Server) mediaStats(w http.ResponseWriter, r *http.Request) {
	result := map[string]int64{"pending": 0, "downloading": 0, "completed": 0, "failed": 0, "assets": 0, "bytes": 0}
	rows, err := s.Repo.DB.Query(r.Context(), `SELECT status,count(*) FROM media_references GROUP BY status`)
	if err != nil {
		writeError(w, 500, err)
		return
	}
	for rows.Next() {
		var status string
		var count int64
		if rows.Scan(&status, &count) == nil {
			result[status] = count
		}
	}
	rows.Close()
	var assets, bytes int64
	if err := s.Repo.DB.QueryRow(r.Context(), `SELECT count(*),COALESCE(sum(size),0) FROM media_assets`).Scan(&assets, &bytes); err != nil {
		writeError(w, 500, err)
		return
	}
	result["assets"], result["bytes"] = assets, bytes
	writeJSON(w, 200, result)
}

func (s *Server) mediaAsset(w http.ResponseWriter, r *http.Request) {
	var objectPath, mimeType, filename string
	err := s.Repo.DB.QueryRow(r.Context(), `SELECT object_path,mime_type,original_filename FROM media_assets WHERE id=$1`, chi.URLParam(r, "id")).Scan(&objectPath, &mimeType, &filename)
	if err != nil {
		writeError(w, 404, err)
		return
	}
	root, rootErr := filepath.Abs(s.ObjectRoot)
	path, pathErr := filepath.Abs(filepath.Join(s.ObjectRoot, objectPath))
	if rootErr != nil || pathErr != nil || (path != root && !strings.HasPrefix(path, root+string(os.PathSeparator))) {
		writeJSON(w, 400, map[string]string{"error": "invalid object path"})
		return
	}
	file, err := os.Open(path)
	if err != nil {
		writeError(w, 404, err)
		return
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		writeError(w, 500, err)
		return
	}
	if mimeType != "" {
		w.Header().Set("Content-Type", mimeType)
	}
	if filename != "" {
		w.Header().Set("Content-Disposition", "inline")
	}
	w.Header().Set("Cache-Control", "private, max-age=86400")
	http.ServeContent(w, r, filename, stat.ModTime(), file)
}

func (s *Server) egoPreview(w http.ResponseWriter, r *http.Request) {
	var in struct {
		TargetQQ  string `json:"target_qq"`
		Depth     int    `json:"depth"`
		MaxNodes  int    `json:"max_nodes"`
		MaxEdges  int    `json:"max_edges"`
		MaxEvents int    `json:"max_events"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, 400, err)
		return
	}
	if in.TargetQQ == "" {
		writeJSON(w, 400, map[string]string{"error": "target_qq is required"})
		return
	}
	if in.Depth < analysis.MinDepth {
		in.Depth = analysis.MinDepth
	}
	maxDepth := s.graphMaxDepth(r.Context())
	if maxDepth > 0 && in.Depth > maxDepth {
		writeJSON(w, 400, map[string]string{"error": fmt.Sprintf("depth must be between %d and %d", analysis.MinDepth, maxDepth)})
		return
	}
	options := s.graphBuildOptions(in.MaxNodes, in.MaxEdges, in.MaxEvents)
	graph, err := s.Graph.BuildWithOptions(r.Context(), in.TargetQQ, in.Depth, options)
	if err != nil {
		writeError(w, 404, err)
		return
	}
	networkID, err := s.saveEgoNetwork(r.Context(), in.TargetQQ, in.Depth, options, graph)
	if err != nil {
		writeError(w, 500, err)
		return
	}
	writeJSON(w, 201, map[string]any{"data": graph, "network_id": networkID, "limits": map[string]any{
		"max_nodes":         optionalLimit(s.GraphBuildMaxNodes),
		"max_edges":         optionalLimit(s.GraphBuildMaxEdges),
		"max_events":        optionalLimit(s.GraphBuildMaxEvents),
		"default_unlimited": true,
		"returned_nodes":    len(graph.Nodes),
		"returned_edges":    len(graph.Edges),
		"processed_events":  graph.ProcessedEventCount,
		"truncated":         graph.Truncated,
	}})
}

func (s *Server) saveEgoNetwork(ctx context.Context, targetQQ string, depth int, options analysis.BuildOptions, graph analysis.Graph) (string, error) {
	tx, err := s.Repo.DB.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	input, _ := json.Marshal(map[string]any{"target_qq": targetQQ, "depth": depth, "max_nodes": options.MaxNodes, "max_edges": options.MaxEdges, "max_events": options.MaxEvents})
	var id string
	err = tx.QueryRow(ctx, `INSERT INTO ego_networks(target_qq,depth,status,input,node_count,edge_count,truncated,processed_event_count,completed_at) VALUES($1,$2,'completed',$3,$4,$5,$6,$7,now()) RETURNING id`, targetQQ, depth, input, len(graph.Nodes), len(graph.Edges), graph.Truncated, graph.ProcessedEventCount).Scan(&id)
	if err != nil {
		return "", err
	}

	batch := &pgx.Batch{}
	for _, node := range graph.Nodes {
		metadata, _ := json.Marshal(node.Metadata)
		if string(metadata) == "null" {
			metadata = []byte(`{}`)
		}
		batch.Queue(`INSERT INTO ego_network_nodes(network_id,node_key,node_type,label,metadata) VALUES($1,$2,$3,$4,$5)`, id, node.Key, node.Type, node.Label, metadata)
	}
	for _, edge := range graph.Edges {
		batch.Queue(`INSERT INTO ego_network_edges(network_id,source_key,target_key,relation_type,weight,event_count,evidence_ids,first_seen,last_seen) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, id, edge.Source, edge.Target, edge.RelationType, edge.Weight, edge.EventCount, edge.EvidenceIDs, edge.FirstSeen, edge.LastSeen)
	}

	br := tx.SendBatch(ctx, batch)
	for i := 0; i < batch.Len(); i++ {
		if _, err := br.Exec(); err != nil {
			br.Close()
			return "", err
		}
	}
	if err := br.Close(); err != nil {
		return "", err
	}

	if err = tx.Commit(ctx); err != nil {
		return "", err
	}

	// Asynchronously update media priorities in background pool without blocking HTTP response
	go func() {
		bgCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = reprioritizeGraphMediaPool(bgCtx, s.Repo.DB, graph, targetQQ)
	}()

	return id, nil
}
func (s *Server) egoNetworks(w http.ResponseWriter, r *http.Request) {
	rows, err := s.Repo.DB.Query(r.Context(), `SELECT id::text,target_qq,depth,status,node_count,edge_count,truncated,processed_event_count,created_at,completed_at FROM ego_networks ORDER BY created_at DESC, id DESC LIMIT 100`)
	if err != nil {
		writeError(w, 500, err)
		return
	}
	defer rows.Close()
	data := []map[string]any{}
	for rows.Next() {
		var id, qq, status string
		var depth, nodes, edges, processedEvents int
		var truncated bool
		var created, completed interface{}
		if err := rows.Scan(&id, &qq, &depth, &status, &nodes, &edges, &truncated, &processedEvents, &created, &completed); err != nil {
			writeError(w, 500, err)
			return
		}
		data = append(data, map[string]any{"id": id, "target_qq": qq, "depth": depth, "status": status, "node_count": nodes, "edge_count": edges, "truncated": truncated, "processed_event_count": processedEvents, "created_at": created, "completed_at": completed})
	}
	writeJSON(w, 200, map[string]any{"data": data})
}
func (s *Server) egoNetwork(w http.ResponseWriter, r *http.Request) {
	var id, qq, status string
	var depth, nodes, edges, processedEvents int
	var truncated bool
	var input []byte
	var created, completed interface{}
	err := s.Repo.DB.QueryRow(r.Context(), `SELECT id::text,target_qq,depth,status,input,node_count,edge_count,truncated,processed_event_count,created_at,completed_at FROM ego_networks WHERE id=$1`, chi.URLParam(r, "id")).Scan(&id, &qq, &depth, &status, &input, &nodes, &edges, &truncated, &processedEvents, &created, &completed)
	if err != nil {
		writeError(w, 404, err)
		return
	}
	var value any
	_ = json.Unmarshal(input, &value)
	writeJSON(w, 200, map[string]any{"data": map[string]any{"id": id, "target_qq": qq, "depth": depth, "status": status, "input": value, "node_count": nodes, "edge_count": edges, "truncated": truncated, "processed_event_count": processedEvents, "created_at": created, "completed_at": completed}})
}
func (s *Server) egoNetworkNodes(w http.ResponseWriter, r *http.Request) {
	rows, err := s.Repo.DB.Query(r.Context(), `SELECT node_key,node_type,label,metadata FROM ego_network_nodes WHERE network_id=$1 ORDER BY node_key`, chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, 500, err)
		return
	}
	defer rows.Close()
	data := []map[string]any{}
	for rows.Next() {
		var key, typ, label string
		var raw []byte
		if err := rows.Scan(&key, &typ, &label, &raw); err != nil {
			writeError(w, 500, err)
			return
		}
		var metadata any
		_ = json.Unmarshal(raw, &metadata)
		data = append(data, map[string]any{"key": key, "type": typ, "label": label, "metadata": metadata})
	}
	writeJSON(w, 200, map[string]any{"data": data})
}
func (s *Server) egoNetworkEdges(w http.ResponseWriter, r *http.Request) {
	rows, err := s.Repo.DB.Query(r.Context(), `SELECT source_key,target_key,relation_type,weight,event_count,evidence_ids,first_seen,last_seen FROM ego_network_edges WHERE network_id=$1 ORDER BY source_key,target_key,relation_type`, chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, 500, err)
		return
	}
	defer rows.Close()
	data := []map[string]any{}
	for rows.Next() {
		var source, target, typ string
		var weight float64
		var eventCount int
		var evidence []string
		var firstSeen, lastSeen interface{}
		if err := rows.Scan(&source, &target, &typ, &weight, &eventCount, &evidence, &firstSeen, &lastSeen); err != nil {
			writeError(w, 500, err)
			return
		}
		data = append(data, map[string]any{"source": source, "target": target, "relation_type": typ, "weight": weight, "event_count": eventCount, "evidence_ids": evidence, "first_seen": firstSeen, "last_seen": lastSeen})
	}
	writeJSON(w, 200, map[string]any{"data": data})
}
func (s *Server) egoNetworkEvidence(w http.ResponseWriter, r *http.Request) {
	rows, err := s.Repo.DB.Query(r.Context(), `SELECT DISTINCT rr.id::text,rr.source,rr.endpoint_or_event_type,rr.payload,rr.collected_at FROM ego_network_edges ee CROSS JOIN LATERAL unnest(ee.evidence_ids) evidence_id JOIN raw_records rr ON rr.id=evidence_id WHERE ee.network_id=$1 ORDER BY rr.collected_at DESC LIMIT 1000`, chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, 500, err)
		return
	}
	defer rows.Close()
	data := []map[string]any{}
	for rows.Next() {
		var id, source, endpoint string
		var payload []byte
		var at interface{}
		if err := rows.Scan(&id, &source, &endpoint, &payload, &at); err != nil {
			writeError(w, 500, err)
			return
		}
		var value any
		_ = json.Unmarshal(payload, &value)
		data = append(data, map[string]any{"id": id, "source": source, "endpoint": endpoint, "payload": value, "collected_at": at})
	}
	writeJSON(w, 200, map[string]any{"data": data})
}

func pageParams(r *http.Request) (int, int) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

// sliceLimit parses an optional per-slice limit query param for detail
// payloads, clamped to [1, MaxPageSize]. It falls back to def when the param
// is absent or invalid so existing clients keep today's behavior exactly.
func sliceLimit(r *http.Request, name string, def int) int {
	if v, err := strconv.Atoi(r.URL.Query().Get(name)); err == nil && v > 0 {
		if v > MaxPageSize {
			return MaxPageSize
		}
		return v
	}
	return def
}

func (s *Server) persons(w http.ResponseWriter, r *http.Request) {
	limit, offset := pageParams(r)
	query := r.URL.Query().Get("q")
	var total int64
	if err := s.Repo.DB.QueryRow(r.Context(), `SELECT count(*) FROM persons p WHERE ($1='' OR p.display_name ILIKE '%'||$1||'%' OR EXISTS(SELECT 1 FROM person_identifiers pi WHERE pi.person_id=p.id AND pi.platform_user_id=$1))`, query).Scan(&total); err != nil {
		writeError(w, 500, err)
		return
	}
	rows, err := s.Repo.DB.Query(r.Context(), `SELECT p.id::text,p.display_name,p.first_seen_at,p.last_seen_at,COALESCE(string_agg(pi.platform_user_id,',' ORDER BY pi.platform_user_id),''),COALESCE(
        (SELECT '/api/v1/media/assets/'||mr.asset_id::text FROM media_references mr WHERE mr.person_id=p.id AND mr.media_kind='avatar' AND mr.status='completed' AND mr.asset_id IS NOT NULL ORDER BY mr.completed_at DESC LIMIT 1),
        (SELECT pp.avatar_uri FROM person_profiles pp WHERE pp.person_id=p.id AND pp.avatar_uri LIKE '/api/v1/media/assets/%' ORDER BY pp.valid_from DESC LIMIT 1),
        (SELECT '/api/v1/media/avatars/person/'||pi2.platform_user_id FROM person_identifiers pi2 WHERE pi2.person_id=p.id AND pi2.platform='qq' AND pi2.platform_user_id<>'' LIMIT 1),'')
		FROM persons p LEFT JOIN person_identifiers pi ON pi.person_id=p.id WHERE ($1='' OR p.display_name ILIKE '%'||$1||'%' OR pi.platform_user_id=$1) GROUP BY p.id ORDER BY p.last_seen_at DESC LIMIT $2 OFFSET $3`, query, limit, offset)
	if err != nil {
		writeError(w, 500, err)
		return
	}
	defer rows.Close()
	data := []map[string]any{}
	for rows.Next() {
		var id, name, identifiers, avatar string
		var first, last interface{}
		if err := rows.Scan(&id, &name, &first, &last, &identifiers, &avatar); err != nil {
			writeError(w, 500, err)
			return
		}
		data = append(data, map[string]any{"id": id, "display_name": name, "identifiers": strings.Split(identifiers, ","), "avatar_uri": avatar, "first_seen_at": first, "last_seen_at": last})
	}
	writeJSON(w, 200, map[string]any{"data": data, "total": total, "limit": limit, "offset": offset})
}
func (s *Server) person(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	membershipLimit := sliceLimit(r, "membership_limit", 200)
	messageLimit := sliceLimit(r, "message_limit", 50)
	contentLimit := sliceLimit(r, "content_limit", 50)
	var name, avatarURI string
	var first, last interface{}
	var profileCount, membershipCount, messageCount, relationCount, contentCount int64
	if err := s.Repo.DB.QueryRow(r.Context(), `SELECT p.display_name,p.first_seen_at,p.last_seen_at,COALESCE(
        (SELECT '/api/v1/media/assets/'||mr.asset_id::text FROM media_references mr WHERE mr.person_id=p.id AND mr.media_kind='avatar' AND mr.status='completed' AND mr.asset_id IS NOT NULL ORDER BY mr.completed_at DESC LIMIT 1),
        (SELECT pp.avatar_uri FROM person_profiles pp WHERE pp.person_id=p.id AND pp.avatar_uri LIKE '/api/v1/media/assets/%' ORDER BY pp.valid_from DESC LIMIT 1),
        (SELECT '/api/v1/media/avatars/person/'||pi2.platform_user_id FROM person_identifiers pi2 WHERE pi2.person_id=p.id AND pi2.platform='qq' AND pi2.platform_user_id<>'' LIMIT 1),'')
        FROM persons p WHERE p.id=$1`, id).Scan(&name, &first, &last, &avatarURI); err != nil {
		writeError(w, 404, err)
		return
	}
	if err := s.Repo.DB.QueryRow(r.Context(), `SELECT
		(SELECT count(*) FROM person_profiles WHERE person_id=$1),
		(SELECT count(*) FROM group_memberships WHERE person_id=$1),
		(SELECT count(*) FROM messages WHERE sender_id=$1),
		(SELECT count(*) FROM relation_events WHERE actor_person_id=$1 OR target_person_id=$1),
		(SELECT count(*) FROM contents WHERE author_id=$1)`, id).Scan(&profileCount, &membershipCount, &messageCount, &relationCount, &contentCount); err != nil {
		writeError(w, 500, err)
		return
	}
	identifiers := []map[string]any{}
	rows, err := s.Repo.DB.Query(r.Context(), `SELECT platform,platform_user_id,confidence FROM person_identifiers WHERE person_id=$1 ORDER BY platform`, id)
	if err != nil {
		writeError(w, 500, err)
		return
	}
	for rows.Next() {
		var platform, value string
		var confidence float64
		if err := rows.Scan(&platform, &value, &confidence); err != nil {
			rows.Close()
			writeError(w, 500, err)
			return
		}
		identifiers = append(identifiers, map[string]any{"platform": platform, "value": value, "confidence": confidence})
	}
	rows.Close()
	profiles := []map[string]any{}
	rows, err = s.Repo.DB.Query(r.Context(), `SELECT nickname,avatar_uri,card_or_remark,source,valid_from,valid_to,raw_record_id::text,profile_data,source_account_id::text,sex,age,area,signature,reg_time,login_days,version_number,observation_count,first_observed_at,last_observed_at,snapshot_hash FROM person_profiles WHERE person_id=$1 ORDER BY valid_from DESC LIMIT 100`, id)
	if err != nil {
		writeError(w, 500, err)
		return
	}
	for rows.Next() {
		var nickname, avatar, card, source string
		var validFrom, validTo, firstObservedAt, lastObservedAt interface{}
		var rawID, sourceAccountID *string
		var profileData []byte
		var sex, area, signature *string
		var age, loginDays *int
		var regTime *int64
		var versionNumber int
		var observationCount int64
		var snapshotHash string
		if err := rows.Scan(&nickname, &avatar, &card, &source, &validFrom, &validTo, &rawID, &profileData, &sourceAccountID, &sex, &age, &area, &signature, &regTime, &loginDays, &versionNumber, &observationCount, &firstObservedAt, &lastObservedAt, &snapshotHash); err != nil {
			rows.Close()
			writeError(w, 500, err)
			return
		}
		var fields any = map[string]any{}
		_ = json.Unmarshal(profileData, &fields)
		structuredFields := map[string]any{}
		if sex != nil {
			structuredFields["sex"] = *sex
		}
		if age != nil {
			structuredFields["age"] = *age
		}
		if area != nil {
			structuredFields["area"] = *area
		}
		if signature != nil {
			structuredFields["signature"] = domain.CleanDisplayText(*signature)
		}
		if regTime != nil {
			structuredFields["reg_time"] = *regTime
		}
		if loginDays != nil {
			structuredFields["login_days"] = *loginDays
		}
		profiles = append(profiles, map[string]any{"nickname": domain.CleanDisplayText(nickname), "avatar_uri": avatar, "card_or_remark": domain.CleanDisplayText(card), "source": source, "valid_from": validFrom, "valid_to": validTo, "raw_record_id": rawID, "source_account_id": sourceAccountID, "fields": fields, "structured": structuredFields, "version_number": versionNumber, "observation_count": observationCount, "first_observed_at": firstObservedAt, "last_observed_at": lastObservedAt, "snapshot_hash": snapshotHash})
	}
	rows.Close()
	memberships := []map[string]any{}
	rows, err = s.Repo.DB.Query(r.Context(), fmt.Sprintf(`SELECT g.id::text,g.platform_group_id,g.group_name,gm.role,gm.card,gm.valid_from,gm.valid_to,COALESCE(
		(SELECT '/api/v1/media/assets/'||mr.asset_id::text FROM media_references mr WHERE mr.group_id=g.id AND mr.media_kind='avatar' AND mr.status='completed' AND mr.asset_id IS NOT NULL ORDER BY mr.completed_at DESC LIMIT 1),'')
		FROM group_memberships gm JOIN groups g ON g.id=gm.group_id WHERE gm.person_id=$1 ORDER BY gm.valid_from DESC LIMIT %d`, membershipLimit), id)
	if err != nil {
		writeError(w, 500, err)
		return
	}
	for rows.Next() {
		var gid, platformID, groupName, role, card, groupAvatar string
		var validFrom, validTo interface{}
		if err := rows.Scan(&gid, &platformID, &groupName, &role, &card, &validFrom, &validTo, &groupAvatar); err != nil {
			rows.Close()
			writeError(w, 500, err)
			return
		}
		if groupAvatar == "" {
			groupAvatar = groupAvatarURL(platformID)
		}
		memberships = append(memberships, map[string]any{"id": gid, "group_id": platformID, "group_name": domain.CleanDisplayText(groupName), "role": role, "card": domain.CleanDisplayText(card), "avatar_uri": groupAvatar, "valid_from": validFrom, "valid_to": validTo})
	}
	rows.Close()
	messages := []map[string]any{}
	rows, err = s.Repo.DB.Query(r.Context(), fmt.Sprintf(`SELECT m.id::text,m.raw_text,m.sent_at,c.conversation_type,c.platform_conversation_id FROM messages m LEFT JOIN conversations c ON c.id=m.conversation_id WHERE m.sender_id=$1 ORDER BY m.sent_at DESC NULLS LAST LIMIT %d`, messageLimit), id)
	if err != nil {
		writeError(w, 500, err)
		return
	}
	for rows.Next() {
		var mid, text, kind, conversationID string
		var at interface{}
		if err := rows.Scan(&mid, &text, &at, &kind, &conversationID); err != nil {
			rows.Close()
			writeError(w, 500, err)
			return
		}
		messages = append(messages, map[string]any{"id": mid, "text": domain.CleanDisplayText(text), "sent_at": at, "conversation_type": kind, "conversation_id": conversationID})
	}
	rows.Close()
	contents := []map[string]any{}
	contentRows, err := s.Repo.DB.Query(r.Context(), fmt.Sprintf(`SELECT c.id::text,c.platform,c.platform_content_id,COALESCE(c.body,''),c.context_type,c.published_at,
		c.metadata,COALESCE(media.items,'[]'::jsonb)
		FROM contents c
		LEFT JOIN LATERAL (SELECT jsonb_agg(jsonb_build_object(
			'id',mr.id::text,'kind',mr.media_kind,'status',mr.status,'source_url',mr.source_url,
			'asset_url',CASE WHEN mr.asset_id IS NOT NULL THEN '/api/v1/media/assets/'||mr.asset_id::text ELSE '' END,
			'mime_type',COALESCE(ma.mime_type,''),'position',cm.position) ORDER BY cm.position) AS items
			FROM content_media cm JOIN media_references mr ON mr.id=cm.media_reference_id
			LEFT JOIN media_assets ma ON ma.id=mr.asset_id WHERE cm.content_id=c.id) media ON true
		WHERE c.author_id=$1
		ORDER BY c.published_at DESC NULLS LAST LIMIT %d`, contentLimit), id)
	if err == nil {
		for contentRows.Next() {
			var cid, platform, platformContentID, body, contextType string
			var publishedAt interface{}
			var metadata, media []byte
			if err := contentRows.Scan(&cid, &platform, &platformContentID, &body, &contextType, &publishedAt, &metadata, &media); err == nil {
				var metadataValue, mediaValue any
				_ = json.Unmarshal(metadata, &metadataValue)
				_ = json.Unmarshal(media, &mediaValue)
				contents = append(contents, map[string]any{
					"id": cid, "platform": platform, "content_id": platformContentID,
					"body": domain.CleanDisplayText(body), "context_type": contextType, "published_at": publishedAt,
					"metadata": metadataValue, "media": mediaValue,
				})
			}
		}
		contentRows.Close()
	}
	relations := []map[string]any{}
	rows, err = s.Repo.DB.Query(r.Context(), `SELECT action_type,context_type,count(*),min(occurred_at),max(occurred_at) FROM relation_events WHERE actor_person_id=$1 OR target_person_id=$1 GROUP BY action_type,context_type ORDER BY count(*) DESC`, id)
	if err != nil {
		writeError(w, 500, err)
		return
	}
	for rows.Next() {
		var action, contextType string
		var count int64
		var minAt, maxAt interface{}
		if err := rows.Scan(&action, &contextType, &count, &minAt, &maxAt); err != nil {
			rows.Close()
			writeError(w, 500, err)
			return
		}
		relations = append(relations, map[string]any{"action_type": action, "context_type": contextType, "count": count, "first_seen": minAt, "last_seen": maxAt})
	}
	rows.Close()
	writeJSON(w, 200, map[string]any{"data": map[string]any{"id": id, "display_name": domain.CleanDisplayText(name), "avatar_uri": avatarURI, "first_seen_at": first, "last_seen_at": last, "identifiers": identifiers, "profiles": profiles, "profile_count": profileCount, "memberships": memberships, "membership_count": membershipCount, "messages": messages, "message_count": messageCount, "contents": contents, "content_count": contentCount, "relations": relations, "relation_count": relationCount}})
}
func (s *Server) groups(w http.ResponseWriter, r *http.Request) {
	limit, offset := pageParams(r)
	query := r.URL.Query().Get("q")
	var total int64
	if err := s.Repo.DB.QueryRow(r.Context(), `SELECT count(*) FROM groups g WHERE ($1='' OR g.group_name ILIKE '%'||$1||'%' OR g.platform_group_id=$1)`, query).Scan(&total); err != nil {
		writeError(w, 500, err)
		return
	}
	rows, err := s.Repo.DB.Query(r.Context(), `SELECT g.id::text,g.platform_group_id,g.group_name,g.first_seen_at,count(gm.id),COALESCE(
		(SELECT '/api/v1/media/assets/'||mr.asset_id::text FROM media_references mr WHERE mr.group_id=g.id AND mr.media_kind='avatar' AND mr.status='completed' AND mr.asset_id IS NOT NULL ORDER BY mr.completed_at DESC LIMIT 1),'')
		FROM groups g LEFT JOIN group_memberships gm ON gm.group_id=g.id WHERE ($1='' OR g.group_name ILIKE '%'||$1||'%' OR g.platform_group_id=$1) GROUP BY g.id ORDER BY g.first_seen_at DESC LIMIT $2 OFFSET $3`, r.URL.Query().Get("q"), limit, offset)
	if err != nil {
		writeError(w, 500, err)
		return
	}
	defer rows.Close()
	data := []map[string]any{}
	for rows.Next() {
		var id, gid, name, avatar string
		var at interface{}
		var count int
		if err := rows.Scan(&id, &gid, &name, &at, &count, &avatar); err != nil {
			writeError(w, 500, err)
			return
		}
		if avatar == "" {
			avatar = groupAvatarURL(gid)
		}
		data = append(data, map[string]any{"id": id, "group_id": gid, "group_name": domain.CleanDisplayText(name), "avatar_uri": avatar, "first_seen_at": at, "member_count": count})
	}
	writeJSON(w, 200, map[string]any{"data": data, "total": total, "limit": limit, "offset": offset})
}
func (s *Server) group(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	memberLimit := sliceLimit(r, "member_limit", 500)
	var platformID, name, avatar string
	var first interface{}
	var memberCount, messageCount int64
	if err := s.Repo.DB.QueryRow(r.Context(), `SELECT g.platform_group_id,g.group_name,g.first_seen_at,COALESCE(
		(SELECT '/api/v1/media/assets/'||mr.asset_id::text FROM media_references mr WHERE mr.group_id=g.id AND mr.media_kind='avatar' AND mr.status='completed' AND mr.asset_id IS NOT NULL ORDER BY mr.completed_at DESC LIMIT 1),'')
		FROM groups g WHERE g.id=$1`, id).Scan(&platformID, &name, &first, &avatar); err != nil {
		writeError(w, 404, err)
		return
	}
	if err := s.Repo.DB.QueryRow(r.Context(), `SELECT
		(SELECT count(*) FROM group_memberships WHERE group_id=$1),
		(SELECT count(*) FROM messages m JOIN conversations c ON c.id=m.conversation_id WHERE c.conversation_type='group' AND c.platform_conversation_id=$2)`, id, platformID).Scan(&memberCount, &messageCount); err != nil {
		writeError(w, 500, err)
		return
	}
	members := []map[string]any{}
	rows, err := s.Repo.DB.Query(r.Context(), fmt.Sprintf(`SELECT p.id::text,p.display_name,COALESCE(pi.platform_user_id,''),gm.role,gm.valid_from,COALESCE(
		(SELECT '/api/v1/media/assets/'||mr.asset_id::text FROM media_references mr WHERE mr.person_id=p.id AND mr.media_kind='avatar' AND mr.status='completed' AND mr.asset_id IS NOT NULL ORDER BY mr.completed_at DESC LIMIT 1),
		(SELECT pp.avatar_uri FROM person_profiles pp WHERE pp.person_id=p.id AND pp.avatar_uri LIKE '/api/v1/media/assets/%%' ORDER BY pp.valid_from DESC LIMIT 1),
		CASE WHEN pi.platform_user_id IS NOT NULL AND pi.platform_user_id<>'' THEN '/api/v1/media/avatars/person/'||pi.platform_user_id ELSE '' END,
		'')
		FROM group_memberships gm JOIN persons p ON p.id=gm.person_id LEFT JOIN person_identifiers pi ON pi.person_id=p.id AND pi.platform='qq' WHERE gm.group_id=$1 ORDER BY CASE gm.role WHEN 'owner' THEN 0 WHEN 'admin' THEN 1 ELSE 2 END,p.display_name LIMIT %d`, memberLimit), id)
	if err != nil {
		writeError(w, 500, err)
		return
	}
	for rows.Next() {
		var pid, display, qq, role, avatar string
		var at interface{}
		if err := rows.Scan(&pid, &display, &qq, &role, &at, &avatar); err != nil {
			rows.Close()
			writeError(w, 500, err)
			return
		}
		members = append(members, map[string]any{"id": pid, "display_name": domain.CleanDisplayText(display), "qq": qq, "role": role, "valid_from": at, "avatar_uri": avatar})
	}
	rows.Close()
	messages := []map[string]any{}
	rows, err = s.Repo.DB.Query(r.Context(), `SELECT m.id::text,COALESCE(p.display_name,''),m.raw_text,m.sent_at FROM messages m JOIN conversations c ON c.id=m.conversation_id LEFT JOIN persons p ON p.id=m.sender_id WHERE c.conversation_type='group' AND c.platform_conversation_id=$1 ORDER BY m.sent_at DESC NULLS LAST LIMIT 100`, platformID)
	if err != nil {
		writeError(w, 500, err)
		return
	}
	for rows.Next() {
		var mid, sender, text string
		var at interface{}
		if err := rows.Scan(&mid, &sender, &text, &at); err != nil {
			rows.Close()
			writeError(w, 500, err)
			return
		}
		messages = append(messages, map[string]any{"id": mid, "sender": sender, "text": text, "sent_at": at})
	}
	rows.Close()
	if avatar == "" {
		avatar = groupAvatarURL(platformID)
	}
	writeJSON(w, 200, map[string]any{"data": map[string]any{"id": id, "group_id": platformID, "group_name": domain.CleanDisplayText(name), "avatar_uri": avatar, "first_seen_at": first, "member_count": memberCount, "message_count": messageCount, "members": members, "messages": messages}})
}

func groupAvatarURL(groupID string) string {
	if groupID == "" {
		return ""
	}
	return fmt.Sprintf("https://p.qlogo.cn/gh/%s/%s/640/", groupID, groupID)
}
func (s *Server) messages(w http.ResponseWriter, r *http.Request) {
	limit, offset := pageParams(r)
	query, conversationType := r.URL.Query().Get("q"), r.URL.Query().Get("conversation_type")
	var total int64
	if err := s.Repo.DB.QueryRow(r.Context(), `SELECT count(*) FROM messages m LEFT JOIN persons p ON p.id=m.sender_id LEFT JOIN person_identifiers pi ON pi.person_id=p.id AND pi.platform='qq' LEFT JOIN conversations c ON c.id=m.conversation_id WHERE ($1='' OR m.raw_text ILIKE '%'||$1||'%' OR p.display_name ILIKE '%'||$1||'%' OR pi.platform_user_id=$1) AND ($2='' OR c.conversation_type=$2)`, query, conversationType).Scan(&total); err != nil {
		writeError(w, 500, err)
		return
	}
	rows, err := s.Repo.DB.Query(r.Context(), `SELECT m.id::text,COALESCE(m.source_message_id,''),COALESCE(p.display_name,''),COALESCE(pi.platform_user_id,''),COALESCE(m.raw_text,''),m.sent_at,COALESCE(c.conversation_type,''),COALESCE(c.platform_conversation_id,''),COALESCE(
		(SELECT '/api/v1/media/assets/'||mr.asset_id::text FROM media_references mr WHERE mr.person_id=p.id AND mr.media_kind='avatar' AND mr.status='completed' AND mr.asset_id IS NOT NULL ORDER BY mr.completed_at DESC LIMIT 1),
		(SELECT pp.avatar_uri FROM person_profiles pp WHERE pp.person_id=p.id AND pp.avatar_uri LIKE '/api/v1/media/assets/%' ORDER BY pp.valid_from DESC LIMIT 1),
		CASE WHEN pi.platform_user_id IS NOT NULL AND pi.platform_user_id<>'' THEN '/api/v1/media/avatars/person/'||pi.platform_user_id ELSE '' END,
		''),
		COALESCE(media.items,'[]'::jsonb),COALESCE(media.total,0)
		FROM messages m LEFT JOIN persons p ON p.id=m.sender_id LEFT JOIN person_identifiers pi ON pi.person_id=p.id AND pi.platform='qq' LEFT JOIN conversations c ON c.id=m.conversation_id
		LEFT JOIN LATERAL (SELECT count(*) AS total,jsonb_agg(item ORDER BY position) AS items FROM (
			SELECT cm.segment_index AS position,jsonb_build_object('reference_id',mr.id::text,'kind',mr.media_kind,'status',mr.status,
				'asset_url',CASE WHEN mr.asset_id IS NOT NULL THEN '/api/v1/media/assets/'||mr.asset_id::text ELSE '' END,
				'mime_type',COALESCE(ma.mime_type,''),'filename',COALESCE(ma.original_filename,mr.original_filename,'')) AS item
			FROM message_media cm JOIN media_references mr ON mr.id=cm.media_reference_id LEFT JOIN media_assets ma ON ma.id=mr.asset_id
			WHERE cm.message_id=m.id ORDER BY cm.segment_index LIMIT 4) preview) media ON true
		WHERE ($1='' OR m.raw_text ILIKE '%'||$1||'%' OR p.display_name ILIKE '%'||$1||'%' OR pi.platform_user_id=$1) AND ($2='' OR c.conversation_type=$2) ORDER BY m.sent_at DESC NULLS LAST LIMIT $3 OFFSET $4`, query, conversationType, limit, offset)
	if err != nil {
		writeError(w, 500, err)
		return
	}
	defer rows.Close()
	data := []map[string]any{}
	for rows.Next() {
		var id, source, sender, senderQQ, text, kind, cid, avatar string
		var at interface{}
		var media []byte
		var mediaCount int
		if err := rows.Scan(&id, &source, &sender, &senderQQ, &text, &at, &kind, &cid, &avatar, &media, &mediaCount); err != nil {
			writeError(w, 500, err)
			return
		}
		var mediaValue any
		_ = json.Unmarshal(media, &mediaValue)
		data = append(data, map[string]any{"id": id, "source_message_id": source, "sender": sender, "sender_qq": senderQQ, "sender_avatar": avatar, "text": text, "sent_at": at, "conversation_type": kind, "conversation_id": cid, "media_preview": mediaValue, "media_count": mediaCount})
	}
	writeJSON(w, 200, map[string]any{"data": data, "total": total, "limit": limit, "offset": offset})
}
func (s *Server) message(w http.ResponseWriter, r *http.Request) {
	var id, source, sender, senderQQ, senderAvatar, text, kind, cid, internalConvID, rawID, replyTo, replySender, replyText string
	var isRecalled bool
	var segments []byte
	var sentAt interface{}
	err := s.Repo.DB.QueryRow(r.Context(), `SELECT
		m.id::text,
		COALESCE(m.source_message_id,''),
		COALESCE(NULLIF(p.display_name,''), NULLIF(na.name,''), CASE WHEN pi.platform_user_id IS NOT NULL AND pi.platform_user_id<>'' THEN 'QQ '||pi.platform_user_id WHEN na.qq_uin IS NOT NULL AND na.qq_uin<>'' THEN 'QQ '||na.qq_uin ELSE '我' END),
		COALESCE(pi.platform_user_id, na.qq_uin, ''),
		COALESCE(
			(SELECT '/api/v1/media/assets/'||mr.asset_id::text FROM media_references mr WHERE mr.person_id=p.id AND mr.media_kind='avatar' AND mr.status='completed' AND mr.asset_id IS NOT NULL ORDER BY mr.completed_at DESC LIMIT 1),
			(SELECT pp.avatar_uri FROM person_profiles pp WHERE pp.person_id=p.id AND pp.avatar_uri LIKE '/api/v1/media/assets/%' ORDER BY pp.valid_from DESC LIMIT 1),
			CASE WHEN pi.platform_user_id IS NOT NULL AND pi.platform_user_id<>'' THEN '/api/v1/media/avatars/person/'||pi.platform_user_id WHEN na.qq_uin IS NOT NULL AND na.qq_uin<>'' THEN '/api/v1/media/avatars/person/'||na.qq_uin ELSE '' END,
			''
		),
		COALESCE(m.raw_text, ''),
		COALESCE(m.message_segments, '[]'::jsonb),
		m.sent_at,
		COALESCE(c.conversation_type,''),
		COALESCE(c.platform_conversation_id,''),
		COALESCE(c.id::text, ''),
		m.raw_record_id::text,
		COALESCE(m.reply_to_message_id, ''),
		COALESCE(NULLIF(rp.display_name, ''), CASE WHEN rpi.platform_user_id IS NOT NULL AND rpi.platform_user_id<>'' THEN 'QQ ' || rpi.platform_user_id ELSE '' END, ''),
		COALESCE(rm.raw_text, ''),
		COALESCE(m.is_recalled, false)
	FROM messages m
	LEFT JOIN persons p ON p.id=m.sender_id
	LEFT JOIN person_identifiers pi ON pi.person_id=p.id AND pi.platform='qq'
	LEFT JOIN napcat_accounts na ON na.id=m.source_account_id
	LEFT JOIN conversations c ON c.id=m.conversation_id
	LEFT JOIN messages rm ON (rm.source_message_id = m.reply_to_message_id OR rm.id::text = m.reply_to_message_id) AND rm.conversation_id = m.conversation_id
	LEFT JOIN persons rp ON rp.id = rm.sender_id
	LEFT JOIN person_identifiers rpi ON rpi.person_id = rp.id AND rpi.platform = 'qq'
	WHERE m.id::text=$1 OR m.source_message_id=$1
	LIMIT 1`, chi.URLParam(r, "id")).Scan(&id, &source, &sender, &senderQQ, &senderAvatar, &text, &segments, &sentAt, &kind, &cid, &internalConvID, &rawID, &replyTo, &replySender, &replyText, &isRecalled)
	if err != nil {
		writeError(w, 404, err)
		return
	}
	mediaItems := []map[string]any{}
	rows, mediaErr := s.Repo.DB.Query(r.Context(), `SELECT mr.id::text,mr.segment_index,mr.segment_type,mr.media_kind,mr.status,COALESCE(mr.asset_id::text,''),COALESCE(ma.mime_type,''),COALESCE(ma.original_filename,''),COALESCE(mr.last_error,'') FROM message_media mm JOIN media_references mr ON mr.id=mm.media_reference_id LEFT JOIN media_assets ma ON ma.id=mm.media_asset_id WHERE mm.message_id=$1::uuid ORDER BY mm.segment_index`, id)
	if mediaErr == nil {
		defer rows.Close()
		for rows.Next() {
			var refID, segmentType, mediaKind, status, assetID, mimeType, filename, lastError string
			var segmentIndex int
			if rows.Scan(&refID, &segmentIndex, &segmentType, &mediaKind, &status, &assetID, &mimeType, &filename, &lastError) == nil {
				assetURL := ""
				if assetID != "" {
					assetURL = "/api/v1/media/assets/" + assetID
				}
				mediaItems = append(mediaItems, map[string]any{
					"reference_id":      refID,
					"segment_index":     segmentIndex,
					"segment_type":      segmentType,
					"kind":              mediaKind,
					"status":            status,
					"asset_id":          assetID,
					"asset_url":         assetURL,
					"mime_type":         mimeType,
					"original_filename": filename,
					"last_error":        lastError,
				})
			}
		}
	}
	writeJSON(w, 200, map[string]any{
		"data": map[string]any{
			"id":                id,
			"source_message_id": source,
			"sender":            domain.CleanDisplayText(sender),
			"sender_qq":         senderQQ,
			"sender_avatar":     senderAvatar,
			"text":              domain.CleanDisplayText(text),
			"message_segments":  string(segments),
			"sent_at":           sentAt,
			"conversation_type": kind,
			"conversation_id":   cid,
			"conversation_uuid": internalConvID,
			"reply_to":          replyTo,
			"reply_sender":      domain.CleanDisplayText(replySender),
			"reply_text":        domain.CleanDisplayText(replyText),
			"is_recalled":       isRecalled,
			"raw_record_id":     rawID,
			"media":             mediaItems,
		},
	})
}
func (s *Server) contents(w http.ResponseWriter, r *http.Request) {
	limit, offset := pageParams(r)
	query, contextType, author := r.URL.Query().Get("q"), r.URL.Query().Get("context_type"), r.URL.Query().Get("author")
	dateRange, mediaFilter, completeness := r.URL.Query().Get("date_range"), r.URL.Query().Get("media_filter"), r.URL.Query().Get("completeness")
	dateDays := 0
	switch dateRange {
	case "7d":
		dateDays = 7
	case "30d":
		dateDays = 30
	case "90d":
		dateDays = 90
	case "1y":
		dateDays = 365
	}
	filter := ` FROM contents c LEFT JOIN persons p ON p.id=c.author_id LEFT JOIN person_identifiers pi ON pi.person_id=p.id AND pi.platform='qq'
		WHERE ($1='' OR c.body ILIKE '%'||$1||'%' OR p.display_name ILIKE '%'||$1||'%' OR pi.platform_user_id=$1)
		AND ($2='' OR c.context_type=$2)
		AND ($3='' OR p.display_name ILIKE '%'||$3||'%' OR pi.platform_user_id=$3)
		AND ($4=0 OR c.published_at >= now()-($4 * interval '1 day'))
		AND ($5='' OR $5='all'
			OR ($5='with' AND EXISTS(SELECT 1 FROM content_media cm WHERE cm.content_id=c.id))
			OR ($5='without' AND NOT EXISTS(SELECT 1 FROM content_media cm WHERE cm.content_id=c.id))
			OR ($5='image' AND EXISTS(SELECT 1 FROM content_media cm WHERE cm.content_id=c.id AND cm.media_kind IN ('image','sticker','avatar')))
			OR ($5='av' AND EXISTS(SELECT 1 FROM content_media cm WHERE cm.content_id=c.id AND cm.media_kind IN ('audio','video','record'))))
		AND ($6='' OR $6='all' OR COALESCE(NULLIF(c.metadata->>'collection_status',''),NULLIF(c.metadata->>'completeness',''),'partial')=$6)`
	var total int64
	if err := s.Repo.DB.QueryRow(r.Context(), `SELECT count(*)`+filter, query, contextType, author, dateDays, mediaFilter, completeness).Scan(&total); err != nil {
		writeError(w, 500, err)
		return
	}
	rows, err := s.Repo.DB.Query(r.Context(), `SELECT c.id::text,c.platform,c.platform_content_id,COALESCE(c.body,''),c.context_type,c.published_at,
		COALESCE(c.author_id::text,''),COALESCE(p.display_name,''),COALESCE(pi.platform_user_id,''),COALESCE(
			(SELECT '/api/v1/media/assets/'||mr.asset_id::text FROM media_references mr WHERE mr.person_id=p.id AND mr.media_kind='avatar' AND mr.status='completed' AND mr.asset_id IS NOT NULL ORDER BY mr.completed_at DESC LIMIT 1),
			(SELECT pp.avatar_uri FROM person_profiles pp WHERE pp.person_id=p.id AND pp.avatar_uri LIKE '/api/v1/media/assets/%' ORDER BY pp.valid_from DESC LIMIT 1),
			CASE WHEN pi.platform_user_id IS NOT NULL AND pi.platform_user_id<>'' THEN '/api/v1/media/avatars/person/'||pi.platform_user_id ELSE '' END,
			''),
		COALESCE(c.source_account_id::text,''),COALESCE(c.parent_content_id::text,''),c.metadata,
		COALESCE(media.items,'[]'::jsonb)
		FROM contents c LEFT JOIN persons p ON p.id=c.author_id
		LEFT JOIN person_identifiers pi ON pi.person_id=p.id AND pi.platform='qq'
		LEFT JOIN LATERAL (SELECT jsonb_agg(jsonb_build_object(
			'id',mr.id::text,'kind',mr.media_kind,'status',mr.status,'source_url',mr.source_url,
			'asset_url',CASE WHEN mr.asset_id IS NOT NULL THEN '/api/v1/media/assets/'||mr.asset_id::text ELSE '' END,
			'mime_type',COALESCE(ma.mime_type,''),'position',cm.position) ORDER BY cm.position) AS items
			FROM content_media cm JOIN media_references mr ON mr.id=cm.media_reference_id
			LEFT JOIN media_assets ma ON ma.id=mr.asset_id WHERE cm.content_id=c.id) media ON true
		WHERE ($1='' OR c.body ILIKE '%'||$1||'%' OR p.display_name ILIKE '%'||$1||'%' OR pi.platform_user_id=$1)
		AND ($2='' OR c.context_type=$2)
		AND ($3='' OR p.display_name ILIKE '%'||$3||'%' OR pi.platform_user_id=$3)
		AND ($4=0 OR c.published_at >= now()-($4 * interval '1 day'))
		AND ($5='' OR $5='all'
			OR ($5='with' AND EXISTS(SELECT 1 FROM content_media cm WHERE cm.content_id=c.id))
			OR ($5='without' AND NOT EXISTS(SELECT 1 FROM content_media cm WHERE cm.content_id=c.id))
			OR ($5='image' AND EXISTS(SELECT 1 FROM content_media cm WHERE cm.content_id=c.id AND cm.media_kind IN ('image','sticker','avatar')))
			OR ($5='av' AND EXISTS(SELECT 1 FROM content_media cm WHERE cm.content_id=c.id AND cm.media_kind IN ('audio','video','record'))))
		AND ($6='' OR $6='all' OR COALESCE(NULLIF(c.metadata->>'collection_status',''),NULLIF(c.metadata->>'completeness',''),'partial')=$6)
		ORDER BY c.published_at DESC NULLS LAST,c.id DESC LIMIT $7 OFFSET $8`,
		query, contextType, author, dateDays, mediaFilter, completeness, limit, offset)
	if err != nil {
		writeError(w, 500, err)
		return
	}
	defer rows.Close()
	data := []map[string]any{}
	for rows.Next() {
		var id, platform, cid, body, kind, authorID, authorName, authorQQ, avatar, sourceAccountID, parentID string
		var at interface{}
		var metadata, media []byte
		if err := rows.Scan(&id, &platform, &cid, &body, &kind, &at, &authorID, &authorName, &authorQQ, &avatar, &sourceAccountID, &parentID, &metadata, &media); err != nil {
			writeError(w, 500, err)
			return
		}
		var metadataValue, mediaValue any
		_ = json.Unmarshal(metadata, &metadataValue)
		_ = json.Unmarshal(media, &mediaValue)
		data = append(data, map[string]any{"id": id, "platform": platform, "content_id": cid, "body": domain.CleanDisplayText(body), "context_type": kind,
			"published_at": at, "author_id": authorID, "author_name": domain.CleanDisplayText(authorName), "author_qq": authorQQ, "author_avatar": avatar,
			"source_account_id": sourceAccountID, "parent_content_id": parentID, "metadata": metadataValue, "media": mediaValue})
	}
	nextOffset := offset + len(data)
	var nextCursor any
	if int64(nextOffset) < total {
		nextCursor = strconv.Itoa(nextOffset)
	}
	writeJSON(w, 200, map[string]any{"data": data, "total": total, "limit": limit, "offset": offset, "page": map[string]any{"limit": limit, "total": total, "next_cursor": nextCursor}})
}

func (s *Server) content(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	commentLimit := sliceLimit(r, "comment_limit", 500)
	likeLimit := sliceLimit(r, "like_limit", 500)
	var platform, platformID, body, contextType, authorID, authorName, authorQQ, authorAvatar, rawID, parentID string
	var publishedAt interface{}
	var metadata []byte
	err := s.Repo.DB.QueryRow(r.Context(), `SELECT c.id::text,c.platform,c.platform_content_id,c.body,c.context_type,c.published_at,
		COALESCE(c.author_id::text,''),COALESCE(p.display_name,''),COALESCE(pi.platform_user_id,''),COALESCE(
			(SELECT '/api/v1/media/assets/'||mr.asset_id::text FROM media_references mr WHERE mr.person_id=p.id AND mr.media_kind='avatar' AND mr.status='completed' AND mr.asset_id IS NOT NULL ORDER BY mr.completed_at DESC LIMIT 1),
			(SELECT pp.avatar_uri FROM person_profiles pp WHERE pp.person_id=p.id AND pp.avatar_uri LIKE '/api/v1/media/assets/%' ORDER BY pp.valid_from DESC LIMIT 1),
			CASE WHEN pi.platform_user_id IS NOT NULL AND pi.platform_user_id<>'' THEN '/api/v1/media/avatars/person/'||pi.platform_user_id ELSE '' END,
			''),
		COALESCE(c.raw_record_id::text,''),COALESCE(c.parent_content_id::text,''),c.metadata
		FROM contents c LEFT JOIN persons p ON p.id=c.author_id
		LEFT JOIN person_identifiers pi ON pi.person_id=p.id AND pi.platform='qq' WHERE c.id::text=$1 OR c.platform_content_id=$1 LIMIT 1`, id).
		Scan(&id, &platform, &platformID, &body, &contextType, &publishedAt, &authorID, &authorName, &authorQQ, &authorAvatar, &rawID, &parentID, &metadata)
	if err != nil {
		writeError(w, 404, err)
		return
	}
	mediaItems := []map[string]any{}
	rows, err := s.Repo.DB.Query(r.Context(), `SELECT mr.id::text,mr.media_kind,mr.status,COALESCE(mr.source_url,''),
		COALESCE(mr.asset_id::text,''),COALESCE(ma.mime_type,''),cm.position FROM content_media cm
		JOIN media_references mr ON mr.id=cm.media_reference_id LEFT JOIN media_assets ma ON ma.id=mr.asset_id
		WHERE cm.content_id=$1 ORDER BY cm.position`, id)
	if err == nil {
		for rows.Next() {
			var refID, kind, status, sourceURL, assetID, mimeType string
			var position int
			if rows.Scan(&refID, &kind, &status, &sourceURL, &assetID, &mimeType, &position) == nil {
				assetURL := ""
				if assetID != "" {
					assetURL = "/api/v1/media/assets/" + assetID
				}
				mediaItems = append(mediaItems, map[string]any{"id": refID, "kind": kind, "status": status, "source_url": sourceURL, "asset_url": assetURL, "mime_type": mimeType, "position": position})
			}
		}
		rows.Close()
	}
	comments := []map[string]any{}
	rows, err = s.Repo.DB.Query(r.Context(), fmt.Sprintf(`SELECT c.id::text,c.body,c.published_at,COALESCE(p.display_name,''),COALESCE(pi.platform_user_id,'')
		FROM contents c LEFT JOIN persons p ON p.id=c.author_id LEFT JOIN person_identifiers pi ON pi.person_id=p.id AND pi.platform='qq'
		WHERE c.parent_content_id=$1 ORDER BY c.published_at LIMIT %d`, commentLimit), id)
	if err == nil {
		for rows.Next() {
			var commentID, text, name, qq string
			var at interface{}
			if rows.Scan(&commentID, &text, &at, &name, &qq) == nil {
				comments = append(comments, map[string]any{"id": commentID, "body": domain.CleanDisplayText(text), "published_at": at, "author_name": domain.CleanDisplayText(name), "author_qq": qq})
			}
		}
		rows.Close()
	}
	likes := []map[string]any{}
	likeRows, err := s.Repo.DB.Query(r.Context(), fmt.Sprintf(`
		SELECT re.id::text, re.actor_person_id::text, COALESCE(p.display_name,''),
		       COALESCE(pi.platform_user_id,''), re.occurred_at,
		       COALESCE(
		           (SELECT '/api/v1/media/assets/'||mr.asset_id::text FROM media_references mr WHERE mr.person_id=p.id AND mr.media_kind='avatar' AND mr.status='completed' AND mr.asset_id IS NOT NULL ORDER BY mr.completed_at DESC LIMIT 1),
		           (SELECT pp.avatar_uri FROM person_profiles pp WHERE pp.person_id=p.id AND pp.avatar_uri<>'' ORDER BY pp.valid_from DESC LIMIT 1),'')
		FROM relation_events re
		LEFT JOIN persons p ON p.id=re.actor_person_id
		LEFT JOIN person_identifiers pi ON pi.person_id=p.id AND pi.platform='qq'
		WHERE re.action_type='liked' AND re.target_object_id=$1
		ORDER BY re.occurred_at DESC LIMIT %d`, likeLimit), id)
	if err == nil {
		for likeRows.Next() {
			var likeID, actorID, name, qq, avatar string
			var occurredAt interface{}
			if likeRows.Scan(&likeID, &actorID, &name, &qq, &occurredAt, &avatar) == nil {
				likes = append(likes, map[string]any{
					"id":          likeID,
					"person_id":   actorID,
					"nickname":    name,
					"name":        name,
					"uin":         qq,
					"qq":          qq,
					"avatar_url":  avatar,
					"occurred_at": occurredAt,
				})
			}
		}
		likeRows.Close()
	}
	var metadataValue any
	_ = json.Unmarshal(metadata, &metadataValue)
	writeJSON(w, 200, map[string]any{"data": map[string]any{"id": id, "platform": platform, "content_id": platformID,
		"body": body, "context_type": contextType, "published_at": publishedAt, "author_id": authorID, "author_name": authorName,
		"author_qq": authorQQ, "author_avatar": authorAvatar, "raw_record_id": rawID, "parent_content_id": parentID, "metadata": metadataValue,
		"media": mediaItems, "comments": comments, "likes": likes}})
}
func (s *Server) relationEvents(w http.ResponseWriter, r *http.Request) {
	limit, offset := pageParams(r)
	rows, err := s.Repo.DB.Query(r.Context(), `SELECT re.id::text,re.action_type,re.context_type,re.occurred_at,re.raw_record_id::text,COALESCE(ap.display_name,''),COALESCE(tp.display_name,'') FROM relation_events re LEFT JOIN persons ap ON ap.id=re.actor_person_id LEFT JOIN persons tp ON tp.id=re.target_person_id ORDER BY re.occurred_at DESC LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		writeError(w, 500, err)
		return
	}
	defer rows.Close()
	data := []map[string]any{}
	for rows.Next() {
		var id, action, kind, raw, actor, target string
		var at interface{}
		if err := rows.Scan(&id, &action, &kind, &at, &raw, &actor, &target); err != nil {
			writeError(w, 500, err)
			return
		}
		data = append(data, map[string]any{"id": id, "action_type": action, "context_type": kind, "occurred_at": at, "raw_record_id": raw, "actor": actor, "target": target})
	}
	writeJSON(w, 200, map[string]any{"data": data, "limit": limit, "offset": offset})
}
func (s *Server) rawRecord(w http.ResponseWriter, r *http.Request) {
	var id, source, endpoint, hash string
	var payload []byte
	var collectedAt interface{}
	err := s.Repo.DB.QueryRow(r.Context(), `SELECT id::text,source,endpoint_or_event_type,payload,payload_hash,collected_at FROM raw_records WHERE id=$1`, chi.URLParam(r, "id")).Scan(&id, &source, &endpoint, &payload, &hash, &collectedAt)
	if err != nil {
		writeError(w, 404, err)
		return
	}
	var value any
	_ = json.Unmarshal(payload, &value)
	writeJSON(w, 200, map[string]any{"data": map[string]any{"id": id, "source": source, "endpoint": endpoint, "payload": value, "payload_hash": hash, "collected_at": collectedAt}})
}

func (s *Server) capability(w http.ResponseWriter, r *http.Request) {
	endpoint := "/" + strings.TrimPrefix(chi.URLParam(r, "endpoint"), "/")
	for _, c := range s.Capabilities {
		if c.Endpoint == endpoint {
			writeJSON(w, 200, map[string]any{"data": c})
			return
		}
	}
	writeJSON(w, 404, map[string]string{"error": "capability not found"})
}

func (s *Server) operationPreview(w http.ResponseWriter, r *http.Request) {
	var in struct {
		AccountID  string `json:"account_id"`
		Endpoint   string `json:"endpoint"`
		Parameters any    `json:"parameters"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, 400, err)
		return
	}
	if in.AccountID == "" || in.Endpoint == "" {
		writeJSON(w, 400, map[string]string{"error": "account_id and endpoint are required"})
		return
	}
	var cap *domain.Capability
	for i := range s.Capabilities {
		if s.Capabilities[i].Endpoint == in.Endpoint {
			cap = &s.Capabilities[i]
			break
		}
	}
	if cap == nil {
		writeJSON(w, 404, map[string]string{"error": "endpoint is not in capability catalog"})
		return
	}
	if cap.ReadOnly {
		writeJSON(w, 400, map[string]string{"error": "read-only endpoint does not require operation center"})
		return
	}
	hash, err := persistence.PreviewHash(in.Endpoint, in.Parameters)
	if err != nil {
		writeError(w, 400, err)
		return
	}
	op, err := s.Repo.CreateOperation(r.Context(), in.AccountID, in.Endpoint, in.Parameters, hash, auth.Username(r.Context()))
	if err != nil {
		writeError(w, 500, err)
		return
	}
	writeJSON(w, 201, map[string]any{"data": op, "capability": cap})
}
func (s *Server) operations(w http.ResponseWriter, r *http.Request) {
	rows, err := s.Repo.DB.Query(r.Context(), `SELECT id::text,account_id::text,endpoint,parameters,status,preview_hash,created_at,confirmed_at FROM operation_requests ORDER BY created_at DESC LIMIT 100`)
	if err != nil {
		writeError(w, 500, err)
		return
	}
	defer rows.Close()
	data := []map[string]any{}
	for rows.Next() {
		var id, account, endpoint, status, hash string
		var params []byte
		var created, confirmed interface{}
		if err := rows.Scan(&id, &account, &endpoint, &params, &status, &hash, &created, &confirmed); err != nil {
			writeError(w, 500, err)
			return
		}
		var p any
		_ = json.Unmarshal(params, &p)
		data = append(data, map[string]any{"id": id, "account_id": account, "endpoint": endpoint, "parameters": p, "status": status, "preview_hash": hash, "created_at": created, "confirmed_at": confirmed})
	}
	writeJSON(w, 200, map[string]any{"data": data})
}
func (s *Server) confirmOperation(w http.ResponseWriter, r *http.Request) {
	op, err := s.Repo.GetOperation(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, 404, err)
		return
	}
	if op.Status != "pending" {
		writeJSON(w, 409, map[string]string{"error": "operation is no longer pending"})
		return
	}
	hash, _ := persistence.PreviewHash(op.Endpoint, op.Parameters)
	if hash != op.PreviewHash {
		writeJSON(w, 409, map[string]string{"error": "preview hash mismatch"})
		return
	}
	a, err := s.Repo.GetAccount(r.Context(), op.AccountID)
	if err != nil {
		writeError(w, 404, err)
		return
	}
	client := napcat.NewHTTPClient(a.HTTPURL, a.HTTPToken)
	response, callErr := client.Call(r.Context(), op.Endpoint, op.Parameters)
	status := "executed"
	if callErr != nil {
		status = "failed"
		response = json.RawMessage(fmt.Sprintf(`{"error":%q}`, callErr.Error()))
	}
	_ = s.Repo.SaveOperationAudit(r.Context(), op.ID, map[string]any{"account_id": op.AccountID, "endpoint": op.Endpoint, "parameters": op.Parameters}, response, status)
	_ = s.Repo.SetOperationStatus(r.Context(), op.ID, status)
	if callErr != nil {
		writeError(w, 502, callErr)
		return
	}
	writeJSON(w, 200, map[string]any{"data": response, "operation_id": op.ID})
}
func (s *Server) cancelOperation(w http.ResponseWriter, r *http.Request) {
	if err := s.Repo.SetOperationStatus(r.Context(), chi.URLParam(r, "id"), "cancelled"); err != nil {
		writeError(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]string{"status": "cancelled"})
}
func (s *Server) operationAudits(w http.ResponseWriter, r *http.Request) {
	limit, offset := pageParams(r)
	var total int64
	if err := s.Repo.DB.QueryRow(r.Context(), `SELECT count(*) FROM operation_audits`).Scan(&total); err != nil {
		writeError(w, 500, err)
		return
	}
	rows, err := s.Repo.DB.Query(r.Context(), `SELECT id::text,operation_id::text,request,response,status,created_at FROM operation_audits ORDER BY created_at DESC LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		writeError(w, 500, err)
		return
	}
	defer rows.Close()
	data := []map[string]any{}
	for rows.Next() {
		var id, op, status string
		var req, res []byte
		var at interface{}
		if err := rows.Scan(&id, &op, &req, &res, &status, &at); err != nil {
			writeError(w, 500, err)
			return
		}
		var request, response any
		_ = json.Unmarshal(req, &request)
		_ = json.Unmarshal(res, &response)
		item := map[string]any{"id": id, "operation_id": op, "request": request, "response": response, "status": status, "created_at": at}
		if requestMap, ok := request.(map[string]any); ok {
			item["account_id"] = requestMap["account_id"]
			item["endpoint"] = requestMap["endpoint"]
			item["parameters"] = requestMap["parameters"]
		}
		data = append(data, item)
	}
	writeJSON(w, 200, map[string]any{"data": data, "total": total, "limit": limit, "offset": offset})
}

func (s *Server) createAIRun(w http.ResponseWriter, r *http.Request) {
	var in struct {
		SubjectID     string   `json:"subject_id"`
		Scope         any      `json:"scope"`
		EventIDs      []string `json:"event_ids"`
		ModelProvider string   `json:"model_provider"`
		ModelName     string   `json:"model_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, 400, err)
		return
	}
	if strings.TrimSpace(in.SubjectID) == "" {
		writeJSON(w, 400, map[string]string{"error": "subject_id is required"})
		return
	}
	scope, err := json.Marshal(in.Scope)
	if err != nil {
		writeError(w, 400, err)
		return
	}
	if string(scope) == "null" {
		scope = []byte(`{}`)
	}
	var packID, runID string
	if err := s.Repo.DB.QueryRow(r.Context(), `INSERT INTO evidence_packs(subject_id,scope,event_ids) VALUES($1,$2,$3) RETURNING id`, in.SubjectID, scope, in.EventIDs).Scan(&packID); err != nil {
		writeError(w, 500, err)
		return
	}
	if err := s.Repo.DB.QueryRow(r.Context(), `INSERT INTO ai_runs(task_type,model_provider,model_name,evidence_pack_id) VALUES('analysis',$1,$2,$3) RETURNING id`, in.ModelProvider, in.ModelName, packID).Scan(&runID); err != nil {
		writeError(w, 500, err)
		return
	}
	writeJSON(w, 201, map[string]any{"data": map[string]any{"id": runID, "evidence_pack_id": packID, "status": "queued", "remote_execution": false}})
}

func (s *Server) aiRuns(w http.ResponseWriter, r *http.Request) {
	rows, err := s.Repo.DB.Query(r.Context(), `SELECT id::text,task_type,model_provider,model_name,evidence_pack_id::text,status,created_at,completed_at FROM ai_runs ORDER BY created_at DESC LIMIT 100`)
	if err != nil {
		writeError(w, 500, err)
		return
	}
	defer rows.Close()
	data := []map[string]any{}
	for rows.Next() {
		var id, task, provider, name, pack, status string
		var created, completed interface{}
		if err := rows.Scan(&id, &task, &provider, &name, &pack, &status, &created, &completed); err != nil {
			writeError(w, 500, err)
			return
		}
		data = append(data, map[string]any{"id": id, "task_type": task, "model_provider": provider, "model_name": name, "evidence_pack_id": pack, "status": status, "created_at": created, "completed_at": completed})
	}
	writeJSON(w, 200, map[string]any{"data": data})
}

func (s *Server) aiRun(w http.ResponseWriter, r *http.Request) {
	var id, task, provider, name, pack, status string
	var output []byte
	var created, completed interface{}
	err := s.Repo.DB.QueryRow(r.Context(), `SELECT id::text,task_type,model_provider,model_name,evidence_pack_id::text,status,output,created_at,completed_at FROM ai_runs WHERE id=$1`, chi.URLParam(r, "id")).Scan(&id, &task, &provider, &name, &pack, &status, &output, &created, &completed)
	if err != nil {
		writeError(w, 404, err)
		return
	}
	var value any
	_ = json.Unmarshal(output, &value)
	writeJSON(w, 200, map[string]any{"data": map[string]any{"id": id, "task_type": task, "model_provider": provider, "model_name": name, "evidence_pack_id": pack, "status": status, "output": value, "created_at": created, "completed_at": completed}})
}

func (s *Server) evidencePack(w http.ResponseWriter, r *http.Request) {
	var id, subject, policy string
	var scope []byte
	var events []string
	var created interface{}
	err := s.Repo.DB.QueryRow(r.Context(), `SELECT id::text,subject_id,scope,event_ids,redaction_policy,created_at FROM evidence_packs WHERE id=$1`, chi.URLParam(r, "id")).Scan(&id, &subject, &scope, &events, &policy, &created)
	if err != nil {
		writeError(w, 404, err)
		return
	}
	var value any
	_ = json.Unmarshal(scope, &value)
	writeJSON(w, 200, map[string]any{"data": map[string]any{"id": id, "subject_id": subject, "scope": value, "event_ids": events, "redaction_policy": policy, "created_at": created}})
}

func (s *Server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == s.CORSOrigin || (s.CORSOrigin == "http://localhost:5173" && origin == "http://127.0.0.1:5173") {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Add("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(204)
			return
		}
		next.ServeHTTP(w, r)
	})
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

var _ = context.Background
var _ = fmt.Sprint
