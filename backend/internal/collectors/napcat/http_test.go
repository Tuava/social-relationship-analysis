package napcat

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHistoryRequestUsesOpenAPIStringTypes(t *testing.T) {
	requests := make(chan map[string]any, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		requests <- body
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","retcode":0,"data":{"messages":[]}}`))
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, "")
	if _, err := client.GetFriendMsgHistory(context.Background(), int64(123456), 100, nil); err != nil {
		t.Fatal(err)
	}
	first := <-requests
	if first["user_id"] != "123456" {
		t.Fatalf("user_id = %#v", first["user_id"])
	}
	if _, exists := first["message_seq"]; exists {
		t.Fatalf("initial request must omit message_seq, got %#v", first["message_seq"])
	}
	if first["reverse_order"] != false || first["reverseOrder"] != false {
		t.Fatalf("initial request must use forward/default order: %#v", first)
	}

	if _, err := client.GetGroupHistory(context.Background(), "654321", 100, int64(987)); err != nil {
		t.Fatal(err)
	}
	second := <-requests
	if second["group_id"] != "654321" || second["message_seq"] != "987" {
		t.Fatalf("group history params = %#v", second)
	}
	if second["reverse_order"] != true || second["reverseOrder"] != true {
		t.Fatalf("anchored request must walk backward: %#v", second)
	}
}
