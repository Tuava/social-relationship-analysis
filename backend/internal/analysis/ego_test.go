package analysis

import "testing"

func TestBuildOptionsAreUnlimitedByDefault(t *testing.T) {
	options := (BuildOptions{}).WithDefaults()
	if options.MaxNodes != 0 || options.MaxEdges != 0 || options.MaxEvents != 0 {
		t.Fatalf("default build options = %+v, want unlimited zero limits", options)
	}
	bounded := (BuildOptions{MaxNodes: 100, MaxEdges: 200, MaxEvents: 300}).WithDefaults()
	if bounded.MaxNodes != 100 || bounded.MaxEdges != 200 || bounded.MaxEvents != 300 {
		t.Fatalf("explicit build options changed: %+v", bounded)
	}
}

func TestRelationEndpointUsesResolvedObjectType(t *testing.T) {
	objectID := "object-id"
	for _, test := range []struct {
		actionType string
		objectType string
		wantKey    string
		wantType   string
	}{
		{actionType: "replied_to", objectType: "content", wantKey: "content:object-id", wantType: "content"},
		{actionType: "replied_to", objectType: "message", wantKey: "message:object-id", wantType: "message"},
		{actionType: "sent_message", objectType: "conversation", wantKey: "conversation:object-id", wantType: "conversation"},
		{actionType: "sent_message", objectType: "group", wantKey: "group:group-entity-id", wantType: "group"},
		{actionType: "member_of", objectType: "group", wantKey: "group:object-id", wantType: "group"},
	} {
		groupEntityID := ""
		if test.objectType == "group" && test.actionType == "sent_message" {
			groupEntityID = "group-entity-id"
		}
		key, nodeType, _ := relationEndpoint(nil, &objectID, test.actionType, test.objectType, groupEntityID)
		if key != test.wantKey || nodeType != test.wantType {
			t.Fatalf("relationEndpoint(%q, %q) = %q/%q, want %q/%q", test.actionType, test.objectType, key, nodeType, test.wantKey, test.wantType)
		}
	}
}
