package qzone

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPClientCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Fatalf("Authorization = %q", got)
		}
		var params map[string]any
		if err := json.NewDecoder(r.Body).Decode(&params); err != nil || params["num"] != float64(20) {
			t.Fatalf("params = %#v, err = %v", params, err)
		}
		_, _ = w.Write([]byte(`{"status":"ok","retcode":0,"data":{"msglist":[]}}`))
	}))
	defer server.Close()

	raw, err := NewHTTPClient(server.URL, "secret").Call(context.Background(), "get_emotion_list", map[string]any{"num": 20})
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"msglist":[]}` {
		t.Fatalf("data = %s", raw)
	}
}

func TestHTTPClientReportsOneBotError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"failed","retcode":1401,"data":null,"message":"未登录"}`))
	}))
	defer server.Close()
	if _, err := NewHTTPClient(server.URL, "").Call(context.Background(), "get_login_info", map[string]any{}); err == nil {
		t.Fatal("expected API error")
	}
}
