package normalization

import (
	"encoding/json"
	"testing"
)

func TestMessageTextPreservesTextSegments(t *testing.T) {
	var raw json.RawMessage = []byte(`[{"type":"text","data":{"text":"hello"}},{"type":"face","data":{}},{"type":"text","data":{"text":" world"}}]`)
	if got := messageText(raw); got != "hello world" {
		t.Fatalf("got %q", got)
	}
}
