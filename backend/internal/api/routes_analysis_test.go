package api

import (
	"bytes"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCosineSimilarity(t *testing.T) {
	a := []int{0, 0, 5, 10, 5, 0}
	b := []int{0, 0, 5, 10, 5, 0}
	sim := cosineSimilarity(a, b)
	if math.Abs(sim-1.0) > 0.001 {
		t.Fatalf("expected identical vectors to have similarity 1.0, got %f", sim)
	}

	c := []int{5, 10, 0, 0, 0, 0}
	simOrthogonal := cosineSimilarity(a, c)
	if simOrthogonal != 0.0 {
		t.Fatalf("expected orthogonal vectors to have similarity 0.0, got %f", simOrthogonal)
	}
}

func TestPlanRoutesValidation(t *testing.T) {
	server := &Server{}

	// Test missing target_qq
	body, _ := json.Marshal(map[string]string{"source_qq": "123456"})
	req := httptest.NewRequest("POST", "/api/v1/analysis/routes", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	server.planRoutes(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for missing target_qq, got %d", rec.Code)
	}

	// Test invalid JSON
	reqBad := httptest.NewRequest("POST", "/api/v1/analysis/routes", bytes.NewReader([]byte("not-json")))
	recBad := httptest.NewRecorder()
	server.planRoutes(recBad, reqBad)
	if recBad.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for bad JSON, got %d", recBad.Code)
	}
}

func TestRelationshipDeepValidation(t *testing.T) {
	server := &Server{}

	// Missing target_qq parameter
	req := httptest.NewRequest("GET", "/api/v1/persons/123/relationship-deep", nil)
	rec := httptest.NewRecorder()

	server.relationshipDeep(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request when target_qq is missing, got %d", rec.Code)
	}
}
