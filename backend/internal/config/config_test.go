package config

import "testing"

func TestLoadRoutingPolicyFromEnvironment(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/sra_test")
	t.Setenv("JWT_SECRET", "test-secret-with-at-least-32-characters")
	t.Setenv("ADMIN_PASSWORD", "test-password-long")
	t.Setenv("SRA_CONFIG_ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	t.Setenv("ROUTING_DEFAULT_MAX_HOPS", "8")
	t.Setenv("ROUTING_DEFAULT_MAX_PATHS", "6")
	t.Setenv("ROUTING_DEFAULT_LARGE_GROUP_THRESHOLD", "240")
	t.Setenv("ROUTING_MAX_HOPS", "20")
	t.Setenv("ROUTING_MAX_PATHS", "10")
	t.Setenv("ROUTING_MAX_GRAPH_NODES", "12000")
	t.Setenv("ROUTING_MAX_FRONTIER_NEIGHBORS", "4000")
	t.Setenv("ROUTING_MAX_GROUP_CO_MEMBERS", "3000")
	t.Setenv("ROUTING_MAX_GROUP_MEMBERSHIPS", "50000")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RoutingDefaultMaxHops != 8 || cfg.RoutingDefaultMaxPaths != 6 || cfg.RoutingDefaultLargeGroupThreshold != 240 {
		t.Fatalf("routing defaults were not loaded: %+v", cfg)
	}
	if cfg.RoutingMaxHops != 20 || cfg.RoutingMaxPaths != 10 || cfg.RoutingMaxGraphNodes != 12000 ||
		cfg.RoutingMaxFrontierNeighbors != 4000 || cfg.RoutingMaxGroupCoMembers != 3000 || cfg.RoutingMaxGroupMemberships != 50000 {
		t.Fatalf("routing limits were not loaded: %+v", cfg)
	}
}
