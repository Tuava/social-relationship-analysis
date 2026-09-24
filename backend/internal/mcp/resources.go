package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/seagull/social-relationship-analysis/backend/internal/analysis"
)

func AllResources() []Resource {
	return []Resource{
		{
			URI:         "sra://collection-runs/latest",
			Name:        "Latest Collection Run Status",
			Description: "Live status, progress percentage, and candidate queue of the most recent collection run.",
			MimeType:    "application/json",
		},
		{
			URI:         "sra://stats/overview",
			Name:        "System Intelligence Overview",
			Description: "Aggregated counts of indexed persons, groups, relations, contents, and media assets.",
			MimeType:    "application/json",
		},
	}
}

func AllResourceTemplates() []ResourceTemplate {
	return []ResourceTemplate{
		{
			URITemplate: "sra://persons/{qq}",
			Name:        "Person OSINT Profile",
			Description: "Full dossier, stranger profile snapshot, and group memberships for a specific QQ.",
			MimeType:    "application/json",
		},
		{
			URITemplate: "sra://groups/{group_id}",
			Name:        "Group Roster and Details",
			Description: "Membership roster and metadata for a specific QQ group.",
			MimeType:    "application/json",
		},
		{
			URITemplate: "sra://ego-networks/{qq}",
			Name:        "Ego Network Topology",
			Description: "1-hop and 2-hop relationship network topology JSON for a target QQ.",
			MimeType:    "application/json",
		},
		{
			URITemplate: "sra://raw/{id}",
			Name:        "Raw Record (Original Payload)",
			Description: "Original API response or realtime event payload with hash and collection time, for forensic verification.",
			MimeType:    "application/json",
		},
		{
			URITemplate: "sra://contents/{id}",
			Name:        "Content Item",
			Description: "Single QZone content record: platform id, body, publish time, author, and raw record link.",
			MimeType:    "application/json",
		},
		{
			URITemplate: "sra://messages/{id}",
			Name:        "Message Record",
			Description: "Single archived message: sender, conversation, sent time, segments, and raw record link.",
			MimeType:    "application/json",
		},
	}
}

func ReadResource(ctx context.Context, db *pgxpool.Pool, uri string) (*ReadResourceResult, error) {
	if uri == "sra://collection-runs/latest" {
		var runID, collectorType, status string
		var progress int
		err := db.QueryRow(ctx, `
			SELECT id, type, status, progress FROM collection_runs ORDER BY created_at DESC LIMIT 1`).
			Scan(&runID, &collectorType, &status, &progress)
		if err != nil {
			return nil, fmt.Errorf("no collection run found: %w", err)
		}
		data, _ := json.MarshalIndent(map[string]any{
			"run_id":   runID,
			"type":     collectorType,
			"status":   status,
			"progress": progress,
		}, "", "  ")
		return &ReadResourceResult{
			Contents: []ResourceContent{
				{URI: uri, MimeType: "application/json", Text: string(data)},
			},
		}, nil
	}

	if uri == "sra://stats/overview" {
		var personCount, groupCount, relationCount, contentCount int
		_ = db.QueryRow(ctx, `SELECT COUNT(*) FROM persons`).Scan(&personCount)
		_ = db.QueryRow(ctx, `SELECT COUNT(*) FROM "groups"`).Scan(&groupCount)
		_ = db.QueryRow(ctx, `SELECT COUNT(*) FROM relation_events`).Scan(&relationCount)
		_ = db.QueryRow(ctx, `SELECT COUNT(*) FROM contents`).Scan(&contentCount)

		data, _ := json.MarshalIndent(map[string]int{
			"persons":   personCount,
			"groups":    groupCount,
			"relations": relationCount,
			"contents":  contentCount,
		}, "", "  ")
		return &ReadResourceResult{
			Contents: []ResourceContent{
				{URI: uri, MimeType: "application/json", Text: string(data)},
			},
		}, nil
	}

	if strings.HasPrefix(uri, "sra://persons/") {
		qq := strings.TrimPrefix(uri, "sra://persons/")
		var id, name string
		err := db.QueryRow(ctx, `
			SELECT p.id, p.display_name FROM persons p
			JOIN person_identifiers pi ON pi.person_id = p.id AND pi.platform = 'qq'
			WHERE pi.platform_user_id = $1 LIMIT 1`, qq).Scan(&id, &name)
		if err != nil {
			return nil, fmt.Errorf("person not found for QQ %s", qq)
		}
		data, _ := json.MarshalIndent(map[string]any{
			"person_id":    id,
			"qq":           qq,
			"display_name": name,
		}, "", "  ")
		return &ReadResourceResult{
			Contents: []ResourceContent{
				{URI: uri, MimeType: "application/json", Text: string(data)},
			},
		}, nil
	}

	if strings.HasPrefix(uri, "sra://ego-networks/") {
		qq := strings.TrimPrefix(uri, "sra://ego-networks/")
		builder := analysis.EgoBuilder{DB: db}
		graph, err := builder.BuildWithOptions(ctx, qq, 1, analysis.BuildOptions{MaxNodes: 50})
		if err != nil {
			return nil, fmt.Errorf("failed to build ego network for %s: %w", qq, err)
		}
		data, _ := json.MarshalIndent(graph, "", "  ")
		return &ReadResourceResult{
			Contents: []ResourceContent{
				{URI: uri, MimeType: "application/json", Text: string(data)},
			},
		}, nil
	}

	if strings.HasPrefix(uri, "sra://raw/") {
		id := strings.TrimPrefix(uri, "sra://raw/")
		var payload json.RawMessage
		err := db.QueryRow(ctx,
			`SELECT to_jsonb(t) FROM raw_records t WHERE id::text = $1`, id).Scan(&payload)
		if err != nil {
			return nil, resourceLookupError("raw record", id, err)
		}
		return jsonResource(uri, payload), nil
	}

	if strings.HasPrefix(uri, "sra://contents/") {
		id := strings.TrimPrefix(uri, "sra://contents/")
		var payload json.RawMessage
		err := db.QueryRow(ctx,
			`SELECT to_jsonb(t) FROM contents t WHERE id::text = $1`, id).Scan(&payload)
		if err != nil {
			return nil, resourceLookupError("content", id, err)
		}
		return jsonResource(uri, payload), nil
	}

	if strings.HasPrefix(uri, "sra://messages/") {
		id := strings.TrimPrefix(uri, "sra://messages/")
		var payload json.RawMessage
		err := db.QueryRow(ctx,
			`SELECT to_jsonb(t) FROM messages t WHERE id::text = $1`, id).Scan(&payload)
		if err != nil {
			return nil, resourceLookupError("message", id, err)
		}
		return jsonResource(uri, payload), nil
	}

	return nil, fmt.Errorf("resource URI not found: %s", uri)
}

// resourceLookupError converts a database lookup failure into an error whose
// message classifies as not_found via deriveErrorCode when no row exists.
func resourceLookupError(kind, id string, err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%s not found for id: %s", kind, id)
	}
	return fmt.Errorf("database error while reading %s %s: %w", kind, id, err)
}

// jsonResource wraps a raw JSON payload as a JSON resource read result.
func jsonResource(uri string, payload json.RawMessage) *ReadResourceResult {
	return &ReadResourceResult{
		Contents: []ResourceContent{
			{URI: uri, MimeType: "application/json", Text: string(payload)},
		},
	}
}
