package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/seagull/social-relationship-analysis/backend/internal/analysis"
	"github.com/seagull/social-relationship-analysis/backend/internal/collectors/napcat"
	"github.com/seagull/social-relationship-analysis/backend/internal/config"
	"github.com/seagull/social-relationship-analysis/backend/internal/mcp"
	"github.com/seagull/social-relationship-analysis/backend/internal/persistence"
)

func main() {
	// Logger writes to stderr so stdout remains 100% pure JSON-RPC stream
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := config.Load()
	if err != nil {
		logger.Error("load config failed", "error", err)
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	pool, err := persistence.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("database connection failed", "error", err)
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

	repo := &persistence.Repository{DB: pool, EncryptionKey: os.Getenv("SRA_CONFIG_ENCRYPTION_KEY")}
	policy := mcp.NormalizedRoutingPolicy(analysis.RoutingPolicy{
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
	server := mcp.NewServerWithPolicy(pool, repo, logger, policy)
	server.SetSQLDatabase(sqlPool)
	caps, err := napcat.LoadCapabilities(resolveCapabilitiesPath(cfg.NapCatCapabilities))
	if err != nil {
		logger.Warn("load napcat capabilities", "error", err)
	} else {
		server.SetCapabilities(caps)
	}

	if err := server.Run(ctx); err != nil && err != context.Canceled {
		logger.Error("MCP server terminated with error", "error", err)
		os.Exit(1)
	}
}

func resolveCapabilitiesPath(path string) string {
	if _, err := os.Stat(path); err == nil {
		return path
	}
	executable, err := os.Executable()
	if err != nil {
		return path
	}
	return filepath.Join(filepath.Dir(executable), path)
}
