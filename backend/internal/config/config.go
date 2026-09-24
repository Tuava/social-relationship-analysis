package config

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTPAddr                          string
	DatabaseURL                       string
	JWTSecret                         string
	JWTDuration                       time.Duration
	AdminUsername                     string
	AdminPassword                     string
	ObjectRoot                        string
	NapCatMediaRoot                   string
	NapCatCapabilities                string
	CookieSecure                      bool
	CORSOrigin                        string
	GraphMaxDepth                     int
	GraphBuildMaxNodes                int
	GraphBuildMaxEdges                int
	GraphBuildMaxEvents               int
	GraphViewMaxNodes                 int
	GraphExpandMaxNodes               int
	RoutingDefaultMaxHops             int
	RoutingDefaultMaxPaths            int
	RoutingDefaultLargeGroupThreshold int
	RoutingMaxHops                    int
	RoutingMaxPaths                   int
	RoutingMaxGraphNodes              int
	RoutingMaxFrontierNeighbors       int
	RoutingMaxGroupCoMembers          int
	RoutingMaxGroupMemberships        int
}

func Load() (Config, error) {
	c := Config{
		HTTPAddr:           env("HTTP_ADDR", "127.0.0.1:8000"),
		DatabaseURL:        os.Getenv("DATABASE_URL"),
		JWTSecret:          os.Getenv("JWT_SECRET"),
		AdminUsername:      env("ADMIN_USERNAME", "admin"),
		AdminPassword:      os.Getenv("ADMIN_PASSWORD"),
		ObjectRoot:         env("OBJECT_ROOT", "../data/objects"),
		NapCatMediaRoot:    env("NAPCAT_MEDIA_ROOT", defaultNapCatMediaRoot()),
		NapCatCapabilities: env("NAPCAT_OPENAPI_PATH", "internal/collectors/napcat/openapi-4.18.18.json"),
		CookieSecure:       envBool("COOKIE_SECURE", false),
		CORSOrigin:         env("CORS_ORIGIN", "http://localhost:5173"),
		// These are deployment policies, not client-side data limits. The API
		// exposes the resolved values so clients never need to guess them.
		GraphMaxDepth:                     envInt("GRAPH_MAX_DEPTH", 0, 0),
		GraphBuildMaxNodes:                envInt("GRAPH_BUILD_MAX_NODES", 0, 0),
		GraphBuildMaxEdges:                envInt("GRAPH_BUILD_MAX_EDGES", 0, 0),
		GraphBuildMaxEvents:               envInt("GRAPH_BUILD_MAX_EVENTS", 0, 0),
		GraphViewMaxNodes:                 envInt("GRAPH_VIEW_MAX_NODES", 0, 0),
		GraphExpandMaxNodes:               envInt("GRAPH_EXPAND_MAX_NODES", 0, 0),
		RoutingDefaultMaxHops:             envInt("ROUTING_DEFAULT_MAX_HOPS", 4, 1),
		RoutingDefaultMaxPaths:            envInt("ROUTING_DEFAULT_MAX_PATHS", 3, 1),
		RoutingDefaultLargeGroupThreshold: envInt("ROUTING_DEFAULT_LARGE_GROUP_THRESHOLD", 100, 1),
		RoutingMaxHops:                    envInt("ROUTING_MAX_HOPS", 0, 0),
		RoutingMaxPaths:                   envInt("ROUTING_MAX_PATHS", 0, 0),
		RoutingMaxGraphNodes:              envInt("ROUTING_MAX_GRAPH_NODES", 0, 0),
		RoutingMaxFrontierNeighbors:       envInt("ROUTING_MAX_FRONTIER_NEIGHBORS", 0, 0),
		RoutingMaxGroupCoMembers:          envInt("ROUTING_MAX_GROUP_CO_MEMBERS", 0, 0),
		RoutingMaxGroupMemberships:        envInt("ROUTING_MAX_GROUP_MEMBERSHIPS", 0, 0),
	}
	if strings.TrimSpace(c.DatabaseURL) == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}
	if len(c.JWTSecret) < 32 || c.JWTSecret == "replace-with-a-long-random-secret" {
		return Config{}, fmt.Errorf("JWT_SECRET must be a random secret of at least 32 characters")
	}
	if len(c.AdminPassword) < 12 || c.AdminPassword == "change-me-now" {
		return Config{}, fmt.Errorf("ADMIN_PASSWORD must contain at least 12 characters and must not be the example value")
	}
	key, err := hex.DecodeString(strings.TrimSpace(os.Getenv("SRA_CONFIG_ENCRYPTION_KEY")))
	if err != nil || len(key) != 32 {
		return Config{}, fmt.Errorf("SRA_CONFIG_ENCRYPTION_KEY must be 64 hex characters")
	}
	c.JWTDuration = 24 * time.Hour
	if raw := os.Getenv("JWT_DURATION"); raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil {
			return Config{}, fmt.Errorf("JWT_DURATION: %w", err)
		}
		if d <= 0 {
			return Config{}, fmt.Errorf("JWT_DURATION must be positive")
		}
		c.JWTDuration = d
	}
	return c, nil
}

func defaultNapCatMediaRoot() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	root := filepath.Join(home, "Library", "Containers", "com.tencent.qq", "Data", "Library", "Application Support", "QQ")
	if info, err := os.Stat(root); err == nil && info.IsDir() {
		return root
	}
	return ""
}

func env(key, fallback string) string {
	if v := os.Getenv(key); strings.TrimSpace(v) != "" {
		return v
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}

func envInt(key string, fallback, minimum int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < minimum {
		return fallback
	}
	return parsed
}
