package domain

import (
	"strings"
	"time"
)

const (
	GroupModeRecordOnly  = "record_only"
	GroupModeFullCollect = "full_collect"
	GroupModeFullExpand  = "full_expand"

	SpaceModePostsOnly          = "posts_only"
	SpaceModeExpandInteractions = "expand_interactions"
	SpaceModeExpandPeople       = "expand_people"
)

type CollectionEntry struct {
	Type     string         `json:"type"`
	ID       string         `json:"id"`
	Mode     string         `json:"mode"`
	Depth    int            `json:"depth,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type CollectionScope struct {
	Entries                []CollectionEntry `json:"entries"`
	ExcludedGroups         []string          `json:"excluded_groups"`
	ExcludedQQs            []string          `json:"excluded_qqs"`
	DefaultGroupMode       string            `json:"default_group_mode"`
	DefaultSpaceMode       string            `json:"default_space_mode"`
	MaxDepth               *int              `json:"max_depth,omitempty"`
	IncludeAccountBaseline *bool             `json:"include_account_baseline,omitempty"`
}

func (scope CollectionScope) Normalize(accountQQ string) CollectionScope {
	if !validGroupMode(scope.DefaultGroupMode) {
		scope.DefaultGroupMode = GroupModeFullCollect
	}
	if !validSpaceMode(scope.DefaultSpaceMode) {
		scope.DefaultSpaceMode = SpaceModeExpandInteractions
	}
	if scope.MaxDepth != nil {
		value := *scope.MaxDepth
		if value < 1 {
			scope.MaxDepth = nil
		} else if value > 20 {
			value = 20
			scope.MaxDepth = &value
		}
	}
	for index := range scope.Entries {
		scope.Entries[index].Type = strings.TrimSpace(scope.Entries[index].Type)
		scope.Entries[index].ID = strings.TrimSpace(scope.Entries[index].ID)
		if scope.Entries[index].Depth < 0 {
			scope.Entries[index].Depth = 0
		}
		if scope.Entries[index].Type == "group" && !validGroupMode(scope.Entries[index].Mode) {
			scope.Entries[index].Mode = scope.DefaultGroupMode
		}
		if scope.Entries[index].Type == "qq" && !validSpaceMode(scope.Entries[index].Mode) {
			scope.Entries[index].Mode = scope.DefaultSpaceMode
		}
		if scope.Entries[index].Type == "post" && !validSpaceMode(scope.Entries[index].Mode) {
			scope.Entries[index].Mode = scope.DefaultSpaceMode
		}
	}
	if len(scope.Entries) == 0 && strings.TrimSpace(accountQQ) != "" {
		scope.Entries = []CollectionEntry{{Type: "qq", ID: strings.TrimSpace(accountQQ), Mode: scope.DefaultSpaceMode}}
	}
	scope.ExcludedGroups = cleanIdentifiers(scope.ExcludedGroups)
	scope.ExcludedQQs = cleanIdentifiers(scope.ExcludedQQs)
	return scope
}

func (scope CollectionScope) GroupMode(groupID string) (string, bool) {
	if contains(scope.ExcludedGroups, groupID) {
		return "", false
	}
	restricted := false
	for _, entry := range scope.Entries {
		entryGroupID := entry.ID
		if entry.Type == "conversation" && strings.HasPrefix(entry.ID, "group:") {
			entryGroupID = strings.TrimPrefix(entry.ID, "group:")
		} else if entry.Type != "group" {
			continue
		}
		restricted = true
		if entryGroupID == groupID {
			return entry.Mode, true
		}
	}
	if restricted {
		return "", false
	}
	return scope.DefaultGroupMode, true
}

func (scope CollectionScope) QQMode(qq string) (string, bool) {
	if contains(scope.ExcludedQQs, qq) {
		return "", false
	}
	for _, entry := range scope.Entries {
		if entry.Type == "qq" && entry.ID == qq {
			return entry.Mode, true
		}
	}
	return scope.DefaultSpaceMode, true
}

func (scope CollectionScope) EntryQQs() []CollectionEntry {
	result := make([]CollectionEntry, 0)
	for _, entry := range scope.Entries {
		qq := entry.ID
		if entry.Type == "conversation" && strings.HasPrefix(entry.ID, "private:") {
			qq = strings.TrimPrefix(entry.ID, "private:")
		} else if entry.Type != "qq" {
			continue
		}
		if qq != "" && !contains(scope.ExcludedQQs, qq) {
			entry.ID = qq
			result = append(result, entry)
		}
	}
	return result
}

func (scope CollectionScope) EntryPosts() []CollectionEntry {
	result := make([]CollectionEntry, 0)
	for _, entry := range scope.Entries {
		if entry.Type == "post" && entry.ID != "" {
			result = append(result, entry)
		}
	}
	return result
}

func (scope CollectionScope) PrivateConversationAllowed(qq string) bool {
	if contains(scope.ExcludedQQs, qq) {
		return false
	}
	hasExplicitPrivateScope := false
	for _, entry := range scope.Entries {
		if entry.Type != "conversation" || !strings.HasPrefix(entry.ID, "private:") {
			continue
		}
		hasExplicitPrivateScope = true
		if strings.TrimPrefix(entry.ID, "private:") == qq {
			return true
		}
	}
	if hasExplicitPrivateScope {
		return false
	}
	return scope.AccountBaselineEnabled()
}

func (scope CollectionScope) CanExpand(depth int) bool {
	return scope.MaxDepth == nil || depth <= *scope.MaxDepth
}

func (scope CollectionScope) AccountBaselineEnabled() bool {
	if scope.IncludeAccountBaseline != nil {
		return *scope.IncludeAccountBaseline
	}
	if len(scope.Entries) > 0 {
		return false
	}
	return true
}

type CollectionModuleProgress struct {
	Module           string         `json:"module"`
	Status           string         `json:"status"`
	PagesCompleted   int            `json:"pages_completed"`
	PagesTotal       *int           `json:"pages_total,omitempty"`
	RecordsCollected int64          `json:"records_collected"`
	Cursor           map[string]any `json:"cursor,omitempty"`
	Error            string         `json:"error,omitempty"`
	StartedAt        *time.Time     `json:"started_at,omitempty"`
	UpdatedAt        time.Time      `json:"updated_at,omitempty"`
	EndedAt          *time.Time     `json:"ended_at,omitempty"`
}

// HistoryProgress is emitted after every persisted history page. The cursor is
// deliberately part of the public progress model so a stopped run can explain
// exactly where collection ended instead of presenting a synthetic percentage.
type HistoryProgress struct {
	Pages   int
	Records int64
	Cursor  map[string]any
}

type CollectionCandidate struct {
	ID              string         `json:"id,omitempty"`
	RunID           string         `json:"run_id,omitempty"`
	AccountID       string         `json:"account_id,omitempty"`
	EntityType      string         `json:"entity_type"`
	EntityID        string         `json:"entity_id"`
	State           string         `json:"state"`
	Depth           int            `json:"depth"`
	Priority        float64        `json:"priority"`
	Selected        bool           `json:"selected"`
	DiscoveryCount  int            `json:"discovery_count,omitempty"`
	Contexts        []string       `json:"contexts,omitempty"`
	DiscoveryPath   map[string]any `json:"discovery_path,omitempty"`
	DiscoveryPaths  []any          `json:"discovery_paths,omitempty"`
	Name            string         `json:"name,omitempty"`
	AvatarURL       string         `json:"avatar_url,omitempty"`
	FirstDiscovered time.Time      `json:"first_discovered_at,omitempty"`
	LastDiscovered  time.Time      `json:"last_discovered_at,omitempty"`
}

type CollectionObserver struct {
	Module    func(CollectionModuleProgress)
	Candidate func(CollectionCandidate)
}

func validGroupMode(value string) bool {
	return value == GroupModeRecordOnly || value == GroupModeFullCollect || value == GroupModeFullExpand
}

func validSpaceMode(value string) bool {
	return value == SpaceModePostsOnly || value == SpaceModeExpandInteractions || value == SpaceModeExpandPeople
}

func cleanIdentifiers(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}

func contains(values []string, target string) bool {
	target = strings.TrimSpace(target)
	for _, value := range values {
		if strings.TrimSpace(value) == target {
			return true
		}
	}
	return false
}
