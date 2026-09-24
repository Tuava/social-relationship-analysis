package secrets

import "testing"

func TestEncryptDecryptAndLegacyCompatibility(t *testing.T) {
	key := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	protected, err := Encrypt("secret-value", key)
	if err != nil {
		t.Fatal(err)
	}
	if !IsEncrypted(protected) {
		t.Fatalf("expected encrypted prefix, got %q", protected)
	}
	got, err := Decrypt(protected, key)
	if err != nil || got != "secret-value" {
		t.Fatalf("decrypt mismatch: got %q, err %v", got, err)
	}
	legacy, err := Decrypt("legacy-value", key)
	if err != nil || legacy != "legacy-value" {
		t.Fatalf("legacy compatibility failed: got %q, err %v", legacy, err)
	}
}
