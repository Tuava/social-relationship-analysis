package normalization

import (
	"encoding/json"
	"testing"
)

func TestExtractMediaReferences(t *testing.T) {
	raw := json.RawMessage(`[
		{"type":"text","data":{"text":"hello"}},
		{"type":"image","data":{"file":"image-id","url":"https://example.com/a.png"}},
		{"type":"image","data":{"file":"sticker-id","url":"https://gxh.vip.qq.com/a.gif","summary":"[动画表情]"}},
		{"type":"mface","data":{"file_id":"sticker-id"}},
		{"type":"record","data":{"file":"voice.amr"}},
		{"type":"face","data":{"id":"14"}}
	]`)
	refs := ExtractMediaReferences(raw)
	if len(refs) != 4 {
		t.Fatalf("expected 4 downloadable references, got %d", len(refs))
	}
	if refs[0].Kind != "image" || refs[0].SourceURL == "" || refs[1].Kind != "sticker" || refs[2].Kind != "sticker" || refs[3].Resolver != "/get_record" {
		t.Fatalf("unexpected references: %#v", refs)
	}
}

func TestExtractMediaReferencesIgnoresMalformedPayload(t *testing.T) {
	if refs := ExtractMediaReferences(json.RawMessage(`{"type":"image"}`)); len(refs) != 0 {
		t.Fatalf("expected no references, got %#v", refs)
	}
}
