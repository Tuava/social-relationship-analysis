package analysis

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestFitPersonaContextBudgetPreservesTimelineCoverage(t *testing.T) {
	messages := make([]string, 20)
	ids := make([]string, 20)
	for i := range messages {
		messages[i] = fmt.Sprintf("[%02d] %s", i, "一段用于测试的历史消息")
		ids[i] = fmt.Sprintf("id-%02d", i)
	}

	selected, selectedIDs := fitPersonaContextBudget(messages, ids, 120)
	if len(selected) == 0 || len(selected) >= len(messages) {
		t.Fatalf("expected a bounded non-empty sample, got %d messages", len(selected))
	}
	if len(selected) != len(selectedIDs) {
		t.Fatalf("messages and evidence ids diverged: %d != %d", len(selected), len(selectedIDs))
	}
	if selectedIDs[0] != "id-00" || selectedIDs[len(selectedIDs)-1] != "id-19" {
		t.Fatalf("expected oldest and newest evidence, got %v", selectedIDs)
	}
}

func TestLivePersonaAnalysis(t *testing.T) {
	if os.Getenv("TEST_LIVE_LLM") != "1" {
		t.Skip("Skipping live LLM test; set TEST_LIVE_LLM=1 to run against real AI models")
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://localhost:5432/social_relationship_analysis?sslmode=disable"
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer pool.Close()

	// Test persona analysis on user 1714610311 (mikoto)
	profile, err := AnalyzePersonPersona(ctx, pool, "1714610311")
	if err != nil {
		t.Fatalf("AnalyzePersonPersona failed: %v", err)
	}

	t.Logf("Profile for %s (%s):", profile.DisplayName, profile.QQ)
	t.Logf("Biographical anchors count: %d", len(profile.BiographicalAnchors))
	for _, ba := range profile.BiographicalAnchors {
		t.Logf("  - [%s]: %s (Evidence: %s)", ba.Aspect, ba.Detail, ba.VerbatimEvidence)
	}
	t.Logf("Psychological defense: Core=%s, Def=%s",
		profile.PsychologicalDefense.CoreWoundAndInsecurity,
		profile.PsychologicalDefense.DefenseMechanism)
	t.Logf("Linguistic: Catchphrases=%v, Slang=%v, Tone=%s",
		profile.LinguisticFingerprint.Catchphrases,
		profile.LinguisticFingerprint.SlangAndSubculture,
		profile.LinguisticFingerprint.ToneBaseline)
	t.Logf("Archetype: Role=%s, Influence=%d, Deep=%s",
		profile.SocialArchetype.PrimaryRole,
		profile.SocialArchetype.InfluenceScore,
		profile.SocialArchetype.DeepAnalysis)
	t.Logf("Anchor quotes count: %d", len(profile.VerbatimAnchorQuotes))
	t.Logf("Interests count=%d, Evidences count=%d",
		len(profile.InterestSpectrum),
		profile.EvidenceCount)
}
