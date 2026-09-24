package napcat

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/seagull/social-relationship-analysis/backend/internal/domain"
)

func TestHistoryBoundaryUsesOldestPageItemNotSmallestShortID(t *testing.T) {
	messages := []map[string]any{
		{"message_seq": float64(1900000000), "real_seq": "100"},
		{"message_seq": float64(12), "real_seq": "101"},
		{"message_seq": "900", "real_seq": "102"},
	}

	got, ok := historyBoundary(messages)
	if !ok || got != 1900000000 {
		t.Fatalf("historyBoundary() = %d, %v; want first short ID", got, ok)
	}
}

func TestHistoryBoundaryRejectsMissingShortID(t *testing.T) {
	if got, ok := historyBoundary([]map[string]any{{"real_seq": "100"}}); ok || got != 0 {
		t.Fatalf("historyBoundary() = %d, %v; want missing boundary", got, ok)
	}
}

func TestGroupOnlyScopeDoesNotCollectPrivateBaseline(t *testing.T) {
	includeBaseline := false
	scope := domain.CollectionScope{
		Entries:                []domain.CollectionEntry{{Type: "group", ID: "1093370579", Mode: domain.GroupModeFullCollect}},
		IncludeAccountBaseline: &includeBaseline,
	}
	if scope.PrivateConversationAllowed("123456789") {
		t.Fatal("group-only scope must not collect unrelated private conversations")
	}
}

func TestExplicitPrivateScopeAllowsOnlySelectedConversation(t *testing.T) {
	scope := domain.CollectionScope{
		Entries: []domain.CollectionEntry{{Type: "conversation", ID: "private:123456789"}},
	}
	if !scope.PrivateConversationAllowed("123456789") {
		t.Fatal("explicit private conversation should be allowed")
	}
	if scope.PrivateConversationAllowed("987654321") {
		t.Fatal("unselected private conversation should be rejected")
	}
}

func TestCallHistoryWithRetrySucceedsAfterTransientFailure(t *testing.T) {
	calls := 0
	raw, err := callHistoryWithRetry(context.Background(), func() (json.RawMessage, error) {
		calls++
		if calls < 3 {
			return nil, errors.New("NapCat HTTP 502: rate limited")
		}
		return json.RawMessage(`{"ok":true}`), nil
	})
	if err != nil {
		t.Fatalf("expected retry to succeed, got %v", err)
	}
	if string(raw) != `{"ok":true}` {
		t.Fatalf("raw = %s", raw)
	}
	if calls != 3 {
		t.Fatalf("calls = %d, want 3", calls)
	}
}

func TestCallHistoryWithRetryStopsOnPermanentError(t *testing.T) {
	calls := 0
	_, err := callHistoryWithRetry(context.Background(), func() (json.RawMessage, error) {
		calls++
		return nil, errors.New("NapCat API 1403: group not found")
	})
	if err == nil {
		t.Fatal("expected permanent error to propagate")
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1 (no retry on permanent error)", calls)
	}
}

func TestCallHistoryWithRetryHonorsContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := callHistoryWithRetry(ctx, func() (json.RawMessage, error) {
		return nil, errors.New("timeout")
	})
	if err == nil {
		t.Fatal("expected context cancellation")
	}
}

func TestHistoryPageDropsStaleAnchorAndRetriesWithoutIt(t *testing.T) {
	var requested []any
	raw, err, recovered := callHistoryPageWithAnchorRecovery(context.Background(), int64(123), func(seq any) (json.RawMessage, error) {
		requested = append(requested, seq)
		if len(requested) == 1 {
			return nil, errors.New("NapCat API 200: 消息123不存在")
		}
		return json.RawMessage(`{"messages":[]}`), nil
	})
	if err != nil {
		t.Fatalf("expected stale anchor recovery, got %v", err)
	}
	if !recovered {
		t.Fatal("expected recovery marker")
	}
	if len(requested) != 2 || requested[0] != int64(123) || requested[1] != nil {
		t.Fatalf("requests = %#v, want [123 nil]", requested)
	}
	if string(raw) != `{"messages":[]}` {
		t.Fatalf("raw = %s", raw)
	}
}

func TestHistoryPageDoesNotRetryWithoutAnchor(t *testing.T) {
	calls := 0
	_, err, recovered := callHistoryPageWithAnchorRecovery(context.Background(), nil, func(seq any) (json.RawMessage, error) {
		calls++
		return nil, errors.New("NapCat API 200: 消息123不存在")
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if recovered {
		t.Fatal("must not report recovery for an unanchored request")
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
}

func TestHistoryPageTreatsUndefinedMessageAsEmptyUnanchoredHistory(t *testing.T) {
	calls := 0
	raw, err, recovered := callHistoryPageWithAnchorRecovery(context.Background(), nil, func(seq any) (json.RawMessage, error) {
		calls++
		return nil, errors.New("NapCat API 200: 消息undefined不存在")
	})
	if err != nil {
		t.Fatalf("expected empty history to complete, got %v", err)
	}
	if recovered {
		t.Fatal("empty unanchored history is not anchor recovery")
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
	if string(raw) != `{"messages":[]}` {
		t.Fatalf("raw = %s", raw)
	}
}
