package normalization

import (
	"encoding/json"
	"testing"
)

func TestMessageIdentity(t *testing.T) {
	cases := []struct{ name, payload, peer, sender, kind, conversation string }{
		{"incoming", `{"self_id":100,"user_id":200,"sender":{"user_id":200}}`, "", "200", "private", "200"},
		{"outgoing history", `{"user_id":100,"sender":{"user_id":100}}`, "200", "100", "private", "200"},
		{"outgoing event", `{"self_id":100,"user_id":100,"target_id":200,"sender":{"user_id":100}}`, "", "100", "private", "200"},
		{"peer user field", `{"self_id":100,"user_id":200,"sender":{"user_id":100}}`, "", "100", "private", "200"},
		{"unknown outgoing peer", `{"self_id":100,"user_id":100}`, "", "100", "private", ""},
		{"group", `{"group_id":300,"user_id":100}`, "200", "100", "group", "300"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var msg oneBotMessage
			if err := json.Unmarshal([]byte(tc.payload), &msg); err != nil {
				t.Fatal(err)
			}
			sender, kind, conv := messageIdentity(msg, tc.peer)
			if sender != tc.sender || kind != tc.kind || conv != tc.conversation {
				t.Fatalf("got %s/%s/%s", sender, kind, conv)
			}
		})
	}
}
