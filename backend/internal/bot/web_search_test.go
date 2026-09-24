package bot

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestWebSearch(t *testing.T) {
	if os.Getenv("TEST_LIVE_WEB") != "1" {
		t.Skip("set TEST_LIVE_WEB=1 for network integration tests")
	}
	s := &Service{}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	res, err := s.SearchWeb(ctx, "北京今天天气", 5)
	if err != nil {
		t.Fatalf("SearchWeb error: %v", err)
	}

	if res == nil {
		t.Fatal("expected non-nil response")
	}

	t.Logf("SearchWeb returned %d results for query '%s'", len(res.Results), res.Query)
	for i, it := range res.Results {
		t.Logf("[%d] Title: %s\nURL: %s\nSnippet: %s", i+1, it.Title, it.URL, it.Snippet)
	}
}

func TestExecuteToolWebSearch(t *testing.T) {
	if os.Getenv("TEST_LIVE_WEB") != "1" {
		t.Skip("set TEST_LIVE_WEB=1 for network integration tests")
	}
	s := &Service{}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	out, err := s.executeTool(ctx, "web_search", `{"query":"北京今天天气","count":3}`)
	if err != nil {
		t.Fatalf("executeTool error: %v", err)
	}
	if out == "" {
		t.Fatal("expected non-empty output")
	}
	t.Logf("executeTool web_search output: %s", out)
}
