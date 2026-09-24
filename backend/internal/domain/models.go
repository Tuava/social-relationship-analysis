package domain

import (
	"encoding/json"
	"time"
)

type AppUser struct {
	ID           string    `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
}

type NapCatAccount struct {
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	QQUIN           string     `json:"qq_uin"`
	WSURL           string     `json:"ws_url"`
	WSToken         string     `json:"-"`
	HTTPURL         string     `json:"http_url"`
	HTTPToken       string     `json:"-"`
	Enabled         bool       `json:"enabled"`
	Status          string     `json:"status"`
	LastConnectedAt *time.Time `json:"last_connected_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
}

type QZoneConnection struct {
	ID              string     `json:"id"`
	AccountID       string     `json:"account_id"`
	HTTPURL         string     `json:"http_url"`
	AccessToken     string     `json:"-"`
	WSURL           string     `json:"ws_url"`
	Enabled         bool       `json:"enabled"`
	Status          string     `json:"status"`
	LastConnectedAt *time.Time `json:"last_connected_at,omitempty"`
	LastEventAt     *time.Time `json:"last_event_at,omitempty"`
	LastError       *string    `json:"last_error,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type Capability struct {
	Endpoint             string `json:"endpoint"`
	Method               string `json:"method"`
	Summary              string `json:"summary"`
	Tag                  string `json:"tag,omitempty"`
	ReadOnly             bool   `json:"read_only"`
	RequiresConfirmation bool   `json:"requires_confirmation"`
	RequiresAdmin        bool   `json:"requires_admin"`
	Implemented          bool   `json:"implemented"`
}

type CollectionRun struct {
	ID        string          `json:"id"`
	AccountID *string         `json:"account_id,omitempty"`
	Type      string          `json:"type"`
	Status    string          `json:"status"`
	Progress  int             `json:"progress"`
	Error     *string         `json:"error,omitempty"`
	StartedAt *time.Time      `json:"started_at,omitempty"`
	EndedAt   *time.Time      `json:"ended_at,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
	Config    json.RawMessage `json:"config,omitempty"`
}

type RelationEvent struct {
	ID              string    `json:"id"`
	SourceAccountID string    `json:"source_account_id"`
	ActorPersonID   *string   `json:"actor_person_id,omitempty"`
	TargetPersonID  *string   `json:"target_person_id,omitempty"`
	TargetObjectID  *string   `json:"target_object_id,omitempty"`
	ActionType      string    `json:"action_type"`
	ContextType     string    `json:"context_type"`
	OccurredAt      time.Time `json:"occurred_at"`
	EvidenceIDs     []string  `json:"evidence_ids"`
	RawRecordID     string    `json:"raw_record_id"`
}

type EgoNetwork struct {
	ID        string    `json:"id"`
	TargetQQ  string    `json:"target_qq"`
	Depth     int       `json:"depth"`
	Status    string    `json:"status"`
	NodeCount int       `json:"node_count"`
	EdgeCount int       `json:"edge_count"`
	CreatedAt time.Time `json:"created_at"`
}

type OperationRequest struct {
	ID          string     `json:"id"`
	AccountID   string     `json:"account_id"`
	Endpoint    string     `json:"endpoint"`
	Parameters  any        `json:"parameters"`
	Status      string     `json:"status"`
	PreviewHash string     `json:"preview_hash"`
	CreatedAt   time.Time  `json:"created_at"`
	ConfirmedAt *time.Time `json:"confirmed_at,omitempty"`
}
