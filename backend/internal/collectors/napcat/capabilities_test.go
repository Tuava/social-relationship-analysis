package napcat

import (
	"path/filepath"
	"testing"
)

func TestLoadCapabilitiesCatalogsOpenAPI(t *testing.T) {
	caps, err := LoadCapabilities(filepath.Join(".", "openapi-4.18.18.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(caps) < 100 {
		t.Fatalf("capability catalog unexpectedly small: %d", len(caps))
	}
	var found bool
	for _, cap := range caps {
		if cap.Endpoint == "/get_group_list" && cap.Implemented && cap.ReadOnly {
			found = true
		}
	}
	if !found {
		t.Fatal("get_group_list should be a supported read-only capability")
	}
}
