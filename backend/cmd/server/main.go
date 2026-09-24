package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/seagull/social-relationship-analysis/backend/internal/analysis"
	"github.com/seagull/social-relationship-analysis/backend/internal/api"
	"github.com/seagull/social-relationship-analysis/backend/internal/auth"
	"github.com/seagull/social-relationship-analysis/backend/internal/bot"
	"github.com/seagull/social-relationship-analysis/backend/internal/collectors/napcat"
	"github.com/seagull/social-relationship-analysis/backend/internal/collectors/qzone"
	"github.com/seagull/social-relationship-analysis/backend/internal/config"
	"github.com/seagull/social-relationship-analysis/backend/internal/mcp"
	"github.com/seagull/social-relationship-analysis/backend/internal/media"
	"github.com/seagull/social-relationship-analysis/backend/internal/normalization"
	"github.com/seagull/social-relationship-analysis/backend/internal/persistence"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg, err := config.Load()
	if err != nil {
		logger.Error("load config", "error", err)
		os.Exit(1)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	pool, err := persistence.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("database unavailable", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	sqlPool, err := mcp.OpenSQLReadPool(ctx, os.Getenv("SRA_SQL_DATABASE_URL"))
	if err != nil {
		logger.Error("configure SQL reader", "error", err)
		os.Exit(1)
	}
	if sqlPool != nil {
		defer sqlPool.Close()
	}

	migrationDir := env("MIGRATIONS_DIR", "migrations")
	if _, statErr := os.Stat(migrationDir); statErr != nil {
		migrationDir = filepath.Join("backend", "migrations")
	}
	if err := persistence.Migrate(ctx, pool, migrationDir); err != nil {
		logger.Error("migration failed", "error", err)
		os.Exit(1)
	}
	repo := persistence.Repository{DB: pool, EncryptionKey: os.Getenv("SRA_CONFIG_ENCRYPTION_KEY")}
	if err := repo.ProtectSystemConfigSecrets(ctx, os.Getenv("SRA_CONFIG_ENCRYPTION_KEY")); err != nil {
		logger.Error("protect system configuration secrets", "error", err)
		os.Exit(1)
	}
	if recovered, recoverErr := repo.RecoverInterruptedCollectionRuns(ctx); recoverErr != nil {
		logger.Error("recover interrupted collection runs", "error", recoverErr)
		os.Exit(1)
	} else if recovered > 0 {
		logger.Warn("recovered interrupted collection runs", "count", recovered)
	}
	hash, err := auth.Service{}.HashPassword(cfg.AdminPassword)
	if err != nil {
		logger.Error("hash admin password", "error", err)
		os.Exit(1)
	}
	if err := repo.EnsureAdmin(ctx, cfg.AdminUsername, hash); err != nil {
		logger.Error("ensure admin", "error", err)
		os.Exit(1)
	}
	caps, err := napcat.LoadCapabilities(cfg.NapCatCapabilities)
	if err != nil {
		logger.Error("load NapCat capabilities", "error", err)
		os.Exit(1)
	}
	routingPolicy := mcp.NormalizedRoutingPolicy(analysis.RoutingPolicy{
		DefaultMaxHops:             cfg.RoutingDefaultMaxHops,
		DefaultMaxPaths:            cfg.RoutingDefaultMaxPaths,
		DefaultLargeGroupThreshold: cfg.RoutingDefaultLargeGroupThreshold,
		MaxHops:                    cfg.RoutingMaxHops,
		MaxPaths:                   cfg.RoutingMaxPaths,
		MaxGraphNodes:              cfg.RoutingMaxGraphNodes,
		MaxFrontierNeighbors:       cfg.RoutingMaxFrontierNeighbors,
		MaxGroupCoMembers:          cfg.RoutingMaxGroupCoMembers,
		MaxGroupMemberships:        cfg.RoutingMaxGroupMemberships,
	})
	mcpRegistry := &mcp.HandlerRegistry{
		DB:           pool,
		SQLDB:        sqlPool,
		Repo:         &repo,
		Policy:       routingPolicy,
		Capabilities: caps,
	}

	authService := auth.Service{Secret: []byte(cfg.JWTSecret), Duration: cfg.JWTDuration}
	manager := napcat.NewManager(ctx, repo, logger)
	botService := bot.NewService(pool, repo, logger)
	botService.SetMCP(mcpRegistry)
	manager.MessageHook = botService.HandleRealtime
	pipelineManager := analysis.NewPipelineManager(pool)
	if err := pipelineManager.ResumeActivePipelines(ctx); err != nil {
		logger.Error("resume persona pipelines", "error", err)
		os.Exit(1)
	}
	normalizer := normalization.Normalizer{DB: pool}
	qzoneNormalizer := qzone.Normalizer{Repo: repo, Normalizer: normalizer}
	qzoneManager := qzone.NewManager(ctx, repo, logger, qzoneNormalizer)
	mediaWorker := &media.Worker{DB: pool, Store: media.Store{Root: cfg.ObjectRoot}, Logger: logger, AllowedMediaRoot: cfg.NapCatMediaRoot}
	go mediaWorker.Run(ctx)
	go media.BackfillReferences(ctx, pool, logger)
	srv := &api.Server{Repo: repo, Auth: authService, NapCat: manager, QZone: qzoneManager, Bot: botService, PipelineMgr: pipelineManager,
		Collector:      napcat.Collector{Repo: repo, Logger: logger, Normalizer: normalizer, Cursors: napcat.CursorStore{DB: pool}},
		QZoneCollector: qzone.Collector{Repo: repo, Logger: logger, Normalizer: qzoneNormalizer},
		Graph:          analysis.EgoBuilder{DB: pool}, Capabilities: caps, Logger: logger, CORSOrigin: cfg.CORSOrigin, ObjectRoot: cfg.ObjectRoot,
		NapCatMediaRoot: cfg.NapCatMediaRoot, FrontendDist: os.Getenv("FRONTEND_DIST"),
		GraphMaxDepth: cfg.GraphMaxDepth, GraphBuildMaxNodes: cfg.GraphBuildMaxNodes, GraphBuildMaxEdges: cfg.GraphBuildMaxEdges, GraphBuildMaxEvents: cfg.GraphBuildMaxEvents,
		GraphViewMaxNodes: cfg.GraphViewMaxNodes, GraphExpandMaxNodes: cfg.GraphExpandMaxNodes,
		RoutingPolicy: routingPolicy}
	accounts, err := repo.ListAccounts(ctx)
	if err != nil {
		logger.Warn("load enabled NapCat accounts", "error", err)
	}
	for _, account := range accounts {
		if account.Enabled && account.WSURL != "" {
			fullAccount, loadErr := repo.GetAccount(ctx, account.ID)
			if loadErr == nil {
				_ = manager.Connect(ctx, fullAccount)
			}
		}
	}
	qzoneConnections, err := repo.ListQZoneConnections(ctx)
	if err != nil {
		logger.Warn("load enabled QZone connections", "error", err)
	}
	for _, connection := range qzoneConnections {
		if !connection.Enabled || connection.WSURL == "" {
			continue
		}
		account, loadErr := repo.GetAccount(ctx, connection.AccountID)
		fullConnection, connectionErr := repo.GetQZoneConnection(ctx, connection.AccountID)
		if loadErr == nil && connectionErr == nil {
			_ = qzoneManager.Connect(ctx, account, fullConnection)
		}
	}
	httpServer := &http.Server{Addr: cfg.HTTPAddr, Handler: srv.Router(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		logger.Info("server listening", "addr", cfg.HTTPAddr, "capabilities", len(caps))
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server stopped", "error", err)
			cancel()
		}
	}()
	<-ctx.Done()
	shutdownCtx, stop := context.WithTimeout(context.Background(), 10*time.Second)
	defer stop()
	_ = httpServer.Shutdown(shutdownCtx)
}
func env(k, f string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return f
}
