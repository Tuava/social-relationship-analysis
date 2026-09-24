package normalization

import "testing"

func TestProfileSnapshotHashIsStableAndDetectsChanges(t *testing.T) {
	first := []byte(`{"nickname":"Alice","age":20}`)
	reordered := []byte(`{"age":20,"nickname":"Alice"}`)

	if !profileSnapshotsEquivalent("Alice", "avatar", "card", first, "Alice", "avatar", "card", reordered) {
		t.Fatal("semantically equal profile payloads should match")
	}
	if profileSnapshotsEquivalent("Alice", "avatar", "card", first, "Alice", "avatar-2", "card", first) {
		t.Fatal("avatar changes must create a new profile version")
	}
	if profileSnapshotHash("Alice", "avatar", "card", first) == profileSnapshotHash("Alice", "avatar-2", "card", first) {
		t.Fatal("different profile snapshots must not share a hash")
	}
}
