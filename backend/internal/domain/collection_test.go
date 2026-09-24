package domain

import "testing"

func TestCollectionScopeSeparatesCollectionAndExpansion(t *testing.T) {
	maxDepth := 4
	scope := CollectionScope{
		Entries: []CollectionEntry{
			{Type: "qq", ID: "10001", Mode: SpaceModeExpandPeople},
			{Type: "group", ID: "20001", Mode: GroupModeRecordOnly},
			{Type: "group", ID: "20002", Mode: GroupModeFullExpand},
			{Type: "conversation", ID: "private:30001", Mode: GroupModeFullCollect},
		},
		ExcludedGroups: []string{"20003"},
		ExcludedQQs:    []string{"40001"},
		MaxDepth:       &maxDepth,
	}.Normalize("10001")

	if mode, allowed := scope.GroupMode("20001"); !allowed || mode != GroupModeRecordOnly {
		t.Fatalf("record-only group = %q, %v", mode, allowed)
	}
	if mode, allowed := scope.GroupMode("20002"); !allowed || mode != GroupModeFullExpand {
		t.Fatalf("expandable group = %q, %v", mode, allowed)
	}
	if _, allowed := scope.GroupMode("20003"); allowed {
		t.Fatal("excluded group was allowed")
	}
	if scope.PrivateConversationAllowed("30002") {
		t.Fatal("explicit private conversation scope did not restrict other conversations")
	}
	if !scope.PrivateConversationAllowed("30001") {
		t.Fatal("explicit private conversation was rejected")
	}
	if scope.CanExpand(5) {
		t.Fatal("depth fuse allowed depth 5")
	}
}

func TestCollectionScopeHasNoImplicitDepthLimit(t *testing.T) {
	scope := (CollectionScope{}).Normalize("10001")
	if !scope.CanExpand(50) {
		t.Fatal("scope without a depth fuse must not impose a fixed hop limit")
	}
	if len(scope.EntryQQs()) != 1 || scope.EntryQQs()[0].ID != "10001" {
		t.Fatalf("default entry = %#v", scope.EntryQQs())
	}
}

func TestCollectionScopePreservesContinuationDepth(t *testing.T) {
	maxDepth := 4
	scope := CollectionScope{
		Entries:  []CollectionEntry{{Type: "qq", ID: "20002", Mode: SpaceModeExpandPeople, Depth: 3}},
		MaxDepth: &maxDepth,
	}.Normalize("10001")

	entries := scope.EntryQQs()
	if len(entries) != 1 || entries[0].Depth != 3 {
		t.Fatalf("continuation entry = %#v", entries)
	}
	if !scope.CanExpand(entries[0].Depth+1) || scope.CanExpand(entries[0].Depth+2) {
		t.Fatal("depth fuse did not continue from the parent candidate depth")
	}
}

func TestCollectionScopeAccountBaselineDefaultsOnAndCanBeDisabled(t *testing.T) {
	if !(CollectionScope{}).AccountBaselineEnabled() {
		t.Fatal("ordinary collection without entries must include the account baseline")
	}
	disabled := false
	if (CollectionScope{IncludeAccountBaseline: &disabled}).AccountBaselineEnabled() {
		t.Fatal("continuation scope must be able to disable the account baseline")
	}
	customScope := CollectionScope{Entries: []CollectionEntry{{Type: "qq", ID: "10000006"}}}
	if customScope.AccountBaselineEnabled() {
		t.Fatal("explicit target scope should not enable account baseline by default")
	}
	enabled := true
	customScopeWithBaseline := CollectionScope{Entries: []CollectionEntry{{Type: "qq", ID: "10000006"}}, IncludeAccountBaseline: &enabled}
	if !customScopeWithBaseline.AccountBaselineEnabled() {
		t.Fatal("explicitly requested baseline on custom target scope should be enabled")
	}
}
