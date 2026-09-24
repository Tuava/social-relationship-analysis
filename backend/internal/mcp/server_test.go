package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"testing"
)

func TestMCPLatestDiscoverAndLegacyInitialize(t *testing.T) {
	// 1. Test Tools list schema
	tools := AllTools()
	if len(tools) == 0 {
		t.Fatalf("expected tools, got 0")
	}

	// 2. Test stdio server JSON-RPC roundtrip with latest 2026-07-28 server/discover and legacy initialize
	input := `{"jsonrpc":"2.0","id":1,"method":"server/discover"}` + "\n" +
		`{"jsonrpc":"2.0","id":2,"method":"initialize","params":{"protocolVersion":"2026-07-28"}}` + "\n" +
		`{"jsonrpc":"2.0","id":3,"method":"tools/list"}` + "\n" +
		`{"jsonrpc":"2.0","id":4,"method":"resources/list"}` + "\n" +
		`{"jsonrpc":"2.0","id":5,"method":"prompts/list"}` + "\n" +
		`{"jsonrpc":"2.0","id":6,"method":"ping"}` + "\n"

	inBuf := bytes.NewBufferString(input)
	outBuf := &bytes.Buffer{}

	server := NewServer(nil, nil, nil)
	server.SetIO(inBuf, outBuf)

	err := server.Run(context.Background())
	if err != nil {
		t.Fatalf("server run error: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(outBuf.String()), "\n")
	if len(lines) != 6 {
		t.Fatalf("expected 6 response lines, got %d. Output: %s", len(lines), outBuf.String())
	}

	// Check line 1 (server/discover)
	var resp1 JSONRPCResponse
	if err := json.Unmarshal([]byte(lines[0]), &resp1); err != nil {
		t.Fatalf("unmarshal resp1 failed: %v", err)
	}
	if resp1.Error != nil {
		t.Fatalf("unexpected error in server/discover: %v", resp1.Error)
	}

	// Check line 2 (initialize with 2026-07-28)
	var resp2 JSONRPCResponse
	if err := json.Unmarshal([]byte(lines[1]), &resp2); err != nil {
		t.Fatalf("unmarshal resp2 failed: %v", err)
	}
	if resp2.Error != nil {
		t.Fatalf("unexpected error in initialize: %v", resp2.Error)
	}

	// Check line 3 (tools/list)
	var resp3 JSONRPCResponse
	if err := json.Unmarshal([]byte(lines[2]), &resp3); err != nil {
		t.Fatalf("unmarshal resp3 failed: %v", err)
	}
	if resp3.Error != nil {
		t.Fatalf("unexpected error in tools/list: %v", resp3.Error)
	}

	// Check line 4 (resources/list)
	var resp4 JSONRPCResponse
	if err := json.Unmarshal([]byte(lines[3]), &resp4); err != nil {
		t.Fatalf("unmarshal resp4 failed: %v", err)
	}
	if resp4.Error != nil {
		t.Fatalf("unexpected error in resources/list: %v", resp4.Error)
	}

	// Check line 5 (prompts/list)
	var resp5 JSONRPCResponse
	if err := json.Unmarshal([]byte(lines[4]), &resp5); err != nil {
		t.Fatalf("unmarshal resp5 failed: %v", err)
	}
	if resp5.Error != nil {
		t.Fatalf("unexpected error in prompts/list: %v", resp5.Error)
	}
}

func TestAllToolsAreUniqueAndExposeThePlannedSet(t *testing.T) {
	tools := AllTools()
	if len(tools) < 54 {
		t.Fatalf("expected at least 54 advertised tools, got %d", len(tools))
	}
	seen := make(map[string]bool, len(tools))
	nameRe := regexp.MustCompile(`^sra_[a-z0-9_]{1,58}$`)
	coreWithOutput := map[string]bool{
		"sra_search": false, "sra_person_dossier": false, "sra_ego_network": false,
		"sra_find_path": false, "sra_activity_timeline": false, "sra_interaction_stream": false,
		"sra_build_evidence_pack": false, "sra_hypothesis_check": false,
		"sra_trace_claim": false, "sra_content_comments": false,
	}
	for _, tool := range tools {
		if seen[tool.Name] {
			t.Fatalf("duplicate advertised tool: %s", tool.Name)
		}
		seen[tool.Name] = true
		if !nameRe.MatchString(tool.Name) {
			t.Fatalf("tool %q violates the sra_ snake_case naming rule", tool.Name)
		}
		if tool.Title == "" {
			t.Fatalf("tool %s has no title", tool.Name)
		}
		if tool.InputSchema.Type != "object" {
			t.Fatalf("tool %s has invalid input schema type %q", tool.Name, tool.InputSchema.Type)
		}
		if tool.InputSchema.AdditionalProperties == nil || *tool.InputSchema.AdditionalProperties {
			t.Fatalf("tool %s must set additionalProperties=false", tool.Name)
		}
		// Every tool must advertise all four annotation hints so clients can
		// gate read/write/world access consistently.
		a := tool.Annotations
		if a == nil || a.ReadOnlyHint == nil || a.DestructiveHint == nil ||
			a.IdempotentHint == nil || a.OpenWorldHint == nil {
			t.Fatalf("tool %s must advertise all four annotation hints", tool.Name)
		}
		if _, isCore := coreWithOutput[tool.Name]; isCore {
			if tool.OutputSchema == nil {
				t.Fatalf("core tool %s must declare an outputSchema", tool.Name)
			}
			coreWithOutput[tool.Name] = true
		}
	}
	for name, seenSchema := range coreWithOutput {
		if !seenSchema {
			t.Fatalf("core tool %s missing from advertised catalog", name)
		}
	}
}

func TestToolsListResponseDeclaresResultTypeAndTTL(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}` + "\n"
	inBuf := bytes.NewBufferString(input)
	outBuf := &bytes.Buffer{}

	server := NewServer(nil, nil, nil)
	server.SetIO(inBuf, outBuf)
	if err := server.Run(context.Background()); err != nil {
		t.Fatalf("server run error: %v", err)
	}

	var resp JSONRPCResponse
	if err := json.Unmarshal(bytes.TrimSpace(outBuf.Bytes()), &resp); err != nil {
		t.Fatalf("unmarshal tools/list response: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
	result, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatalf("tools/list result is not an object")
	}
	if result["resultType"] != "complete" {
		t.Fatalf("tools/list missing resultType=\"complete\": %v", result)
	}
	if _, ok := result["ttlMs"]; !ok {
		t.Fatalf("tools/list missing ttlMs: %v", result)
	}
	tools, ok := result["tools"].([]any)
	if !ok || len(tools) == 0 {
		t.Fatalf("tools/list missing tools array")
	}
}

func TestToolErrorsAreStructuredResults(t *testing.T) {
	// errorResult now returns machine-readable {ok:false, code, message}.
	res := errorResult("person not found for QQ 123")
	if !res.IsError {
		t.Fatalf("expected IsError true")
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(res.Content[0].Text), &body); err != nil {
		t.Fatalf("error body is not JSON: %v", err)
	}
	if body["ok"] != false || body["code"] != "not_found" {
		t.Fatalf("unexpected error body: %v", body)
	}
	if res.ResultType != "complete" {
		t.Fatalf("expected resultType complete on error result")
	}

	// Tool failures must arrive as structured isError results over
	// tools/call, never as JSON-RPC transport errors. "unknown tool" is
	// exercised here because it needs no database.
	input := `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"sra_does_not_exist","arguments":{}}}` + "\n"
	inBuf := bytes.NewBufferString(input)
	outBuf := &bytes.Buffer{}

	server := NewServer(nil, nil, nil)
	server.SetIO(inBuf, outBuf)
	if err := server.Run(context.Background()); err != nil {
		t.Fatalf("server run error: %v", err)
	}
	assertIsErrorResult(t, outBuf.String(), "invalid_arguments")
}

func TestArgumentErrorsAreStructuredOverTheWire(t *testing.T) {
	// sra_visitor_stream rejects calls with neither actor_qq nor target_qq;
	// this path needs no database and validates the structured error body.
	input := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"sra_visitor_stream","arguments":{}}}` + "\n"
	inBuf := bytes.NewBufferString(input)
	outBuf := &bytes.Buffer{}

	server := NewServer(nil, nil, nil)
	server.SetIO(inBuf, outBuf)
	if err := server.Run(context.Background()); err != nil {
		t.Fatalf("server run error: %v", err)
	}
	assertIsErrorResult(t, outBuf.String(), "invalid_arguments")
}

func assertIsErrorResult(t *testing.T, output, wantCode string) {
	t.Helper()
	var resp JSONRPCResponse
	if err := json.Unmarshal(bytes.TrimSpace([]byte(output)), &resp); err != nil {
		t.Fatalf("unmarshal tools/call response: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("tool failure leaked as JSON-RPC error: %v", resp.Error)
	}
	result, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatalf("tools/call result is not an object")
	}
	if result["isError"] != true {
		t.Fatalf("expected isError=true in result: %v", result)
	}
	if result["resultType"] != "complete" {
		t.Fatalf("expected resultType=complete in result: %v", result)
	}
	content, ok := result["content"].([]any)
	if !ok || len(content) == 0 {
		t.Fatalf("expected content in result")
	}
	text, ok := content[0].(map[string]any)["text"].(string)
	if !ok {
		t.Fatalf("expected text in content[0]")
	}
	var errBody map[string]any
	if err := json.Unmarshal([]byte(text), &errBody); err != nil {
		t.Fatalf("error content is not JSON: %v", err)
	}
	if errBody["ok"] != false || errBody["code"] != wantCode {
		t.Fatalf("expected ok=false code=%s in error body, got %v", wantCode, errBody)
	}
}

func TestResourceTemplatesExposeEvidenceRecords(t *testing.T) {
	templates := AllResourceTemplates()
	uriSet := make(map[string]bool, len(templates))
	for _, tmpl := range templates {
		uriSet[tmpl.URITemplate] = true
	}
	for _, uri := range []string{"sra://raw/{id}", "sra://contents/{id}", "sra://messages/{id}"} {
		if !uriSet[uri] {
			t.Fatalf("missing resource template %s", uri)
		}
	}
}

func TestDeriveTitle(t *testing.T) {
	cases := map[string]string{
		"sra_build_evidence_pack": "Build Evidence Pack",
		"sra_sql_query":           "SQL Query",
		"sra_person_dossier":      "Person Dossier",
	}
	for in, want := range cases {
		if got := deriveTitle(in); got != want {
			t.Fatalf("deriveTitle(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCompletionComplete(t *testing.T) {
	cases := []struct {
		label string
		req   string
		want  []string
	}{
		{
			label: "tool argument from schema enum",
			req:   `{"jsonrpc":"2.0","id":1,"method":"completion/complete","params":{"ref":{"type":"ref/toolArgument","name":"sra_search","argument":"entity_type"},"argument":{"name":"entity_type","value":"per"}}}`,
			want:  []string{"person"},
		},
		{
			label: "tool argument from curated vocabulary",
			req:   `{"jsonrpc":"2.0","id":2,"method":"completion/complete","params":{"ref":{"type":"ref/toolArgument","name":"sra_person_dossier","argument":"sections"},"argument":{"name":"sections","value":"soc"}}}`,
			want:  []string{"social"},
		},
		{
			label: "prompt ref",
			req:   `{"jsonrpc":"2.0","id":3,"method":"completion/complete","params":{"ref":{"type":"ref/prompt","name":"osint"},"argument":{"name":"prompt","value":""}}}`,
			want:  []string{"osint_person_deep_dive", "pairwise_relationship_audit"},
		},
		{
			label: "resource ref prefix",
			req:   `{"jsonrpc":"2.0","id":4,"method":"completion/complete","params":{"ref":{"type":"ref/resource","name":"sra"},"argument":{"name":"uri","value":"sra://raw"}}}`,
			want:  []string{"sra://raw/{id}"},
		},
		{
			label: "unknown argument yields empty set",
			req:   `{"jsonrpc":"2.0","id":5,"method":"completion/complete","params":{"ref":{"type":"ref/toolArgument","name":"sra_search","argument":"nonsense"},"argument":{"name":"nonsense","value":"x"}}}`,
			want:  []string{},
		},
	}

	for _, tc := range cases {
		inBuf := bytes.NewBufferString(tc.req + "\n")
		outBuf := &bytes.Buffer{}
		server := NewServer(nil, nil, nil)
		server.SetIO(inBuf, outBuf)
		if err := server.Run(context.Background()); err != nil {
			t.Fatalf("%s: server run error: %v", tc.label, err)
		}

		var resp JSONRPCResponse
		if err := json.Unmarshal(bytes.TrimSpace(outBuf.Bytes()), &resp); err != nil {
			t.Fatalf("%s: unmarshal response: %v", tc.label, err)
		}
		if resp.Error != nil {
			t.Fatalf("%s: unexpected error: %v", tc.label, resp.Error)
		}
		result, ok := resp.Result.(map[string]any)
		if !ok {
			t.Fatalf("%s: result is not an object", tc.label)
		}
		completion, ok := result["completion"].(map[string]any)
		if !ok {
			t.Fatalf("%s: missing completion", tc.label)
		}
		values, ok := completion["values"].([]any)
		if !ok {
			t.Fatalf("%s: missing values array", tc.label)
		}
		got := make([]string, 0, len(values))
		for _, v := range values {
			got = append(got, v.(string))
		}
		if len(got) != len(tc.want) {
			t.Fatalf("%s: got %v, want %v", tc.label, got, tc.want)
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Fatalf("%s: got %v, want %v", tc.label, got, tc.want)
			}
		}
	}
}

func TestValidateReadOnlySQL(t *testing.T) {
	valid := []string{
		"SELECT id FROM persons",
		"WITH recent AS (SELECT id FROM messages) SELECT * FROM recent",
	}
	for _, query := range valid {
		if err := validateReadOnlySQL(query); err != nil {
			t.Errorf("valid query rejected: %q: %v", query, err)
		}
	}
	invalid := []string{
		"INSERT INTO persons VALUES (1)",
		"UPDATE persons SET display_name = 'x'",
		"SELECT 1; DELETE FROM persons",
		"DELETE FROM persons",
	}
	for _, query := range invalid {
		if err := validateReadOnlySQL(query); err == nil {
			t.Errorf("unsafe query accepted: %q", query)
		}
	}
}
