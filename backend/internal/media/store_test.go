package media

import (
	"os"
	"strings"
	"testing"
)

func TestStorePutUsesContentHashPath(t *testing.T) {
	root := t.TempDir()
	asset, err := (Store{Root: root}).Put(strings.NewReader("hello"), "text/plain")
	if err != nil {
		t.Fatal(err)
	}
	if asset.SHA256 != "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824" {
		t.Fatalf("unexpected hash %s", asset.SHA256)
	}
	if _, err := os.Stat(root + "/" + asset.ObjectPath); err != nil {
		t.Fatal(err)
	}
	second, err := (Store{Root: root}).Put(strings.NewReader("hello"), "text/plain")
	if err != nil || second.ObjectPath != asset.ObjectPath {
		t.Fatalf("deduplication failed: %#v %v", second, err)
	}
}

func TestPutLimitedRejectsLargeObject(t *testing.T) {
	root := t.TempDir()
	_, err := (Store{Root: root}).PutLimited(strings.NewReader("123456"), "text/plain", 5)
	if err == nil {
		t.Fatal("expected oversized object to be rejected")
	}
}
