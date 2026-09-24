package mcp

import "strings"

// toolArgVocab provides curated completion candidates for free-form tool
// arguments (arrays, nested fields, enumerated vocabularies). Arguments that
// already declare a JSON-Schema enum are completed from the schema instead.
var toolArgVocab = map[string]map[string][]string{
	"sra_collection_orchestrate": {
		"modules": {"basic_profile", "qzone_feeds", "qzone_likes", "qzone_comments", "qzone_visits", "group_memberships", "group_messages", "private_messages", "media", "avatar"},
	},
	"sra_person_dossier": {
		"sections": {"identity", "behavioral", "social", "coverage", "evolution", "all"},
	},
	"sra_mutual_analysis": {
		"sections": {"shared_groups", "mutual_friends", "interactions", "strength_score", "all"},
	},
	"sra_person_detail": {
		"sections": {"profiles", "memberships", "stats"},
	},
	"sra_media_references": {
		"kind":   {"avatar", "image", "sticker", "audio", "video", "file"},
		"status": {"pending", "downloading", "completed", "failed"},
	},
	"sra_interaction_stream": {
		"action_types": {"liked", "commented", "replied_to", "sent_message", "mentioned", "published", "member_of", "friend_visible"},
	},
	"sra_relation_events": {
		"action_types": {"member_of", "sent_message", "commented", "mentioned", "liked", "replied_to", "published", "friend_visible", "visited"},
		"context_type": {"group", "qzone_post", "qzone_comment", "private", "qzone_visit"},
	},
	"sra_discover_connections": {
		"interaction_types": {"liked", "commented", "replied_to", "sent_message", "member_of", "mentioned", "friend_visible"},
	},
	"sra_ego_network": {
		"relation_filter": {"liked", "commented", "replied_to", "sent_message", "member_of", "mentioned", "friend_visible", "published"},
	},
	"sra_find_path": {
		"relation_filter": {"liked", "commented", "replied_to", "sent_message", "member_of", "mentioned", "friend_visible", "published"},
	},
	"sra_hypothesis_check": {
		"attribute_type": {"education", "location", "interest", "hobby", "work", "school", "social_circle", "identity_link", "health", "family"},
	},
	"sra_inferences": {
		"attribute_type": {"education", "location", "interest", "hobby", "work", "school", "social_circle", "identity_link", "health", "family"},
		"review_status":  {"pending", "confirmed", "rejected"},
	},
	"sra_conversations": {
		"type": {"private", "group"},
	},
	"sra_collection_runs": {
		"type":   {"full_collect", "profile_sync", "qzone_sync", "media_backfill", "group_messages"},
		"status": {"queued", "running", "completed", "failed", "canceled", "paused"},
	},
	"sra_ai_runs": {
		"task_type": {"evidence_pack", "hypothesis_check", "profile_summary", "relationship_audit"},
		"status":    {"queued", "running", "completed", "failed"},
	},
	"sra_operations": {
		"status": {"pending", "confirmed", "executed", "rejected", "canceled"},
	},
	"sra_operation_audits": {
		"status": {"executed", "failed"},
	},
	"sra_evidence_packs": {
		"redaction_policy": {"local-only", "full"},
	},
	"sra_collection_candidates": {
		"state": {"queued", "scheduled", "processing", "completed", "failed", "skipped"},
	},
	"sra_content_feed": {
		"sort_by": {"recent", "most_liked", "most_commented", "most_engaged"},
	},
	"sra_build_evidence_pack": {
		"redaction_policy": {"local-only", "full"},
	},
}

// completeValues resolves completion candidates for a completion/complete
// request. It covers tool argument refs, prompt refs, and resource refs.
func completeValues(params CompleteParams) []string {
	switch strings.ToLower(params.Ref.Type) {
	case "ref/prompt", "prompt":
		return filterCompletions(promptNames(), params.Argument.Value)
	case "ref/resource", "resource":
		return filterCompletions(resourceURIs(), params.Argument.Value)
	default:
		// ref/toolArgument (or a legacy/empty ref) completes a tool argument.
		toolName := params.Ref.Name
		argName := params.Argument.Name
		if argName == "" {
			argName = params.Ref.Argument
		}
		return completeToolArgument(toolName, argName, params.Argument.Value)
	}
}

// completeToolArgument returns completion candidates for one argument of one
// tool: curated vocabulary first, then the declared JSON-Schema enum.
func completeToolArgument(toolName, argName, value string) []string {
	if vocab, ok := toolArgVocab[toolName]; ok {
		if candidates, ok := vocab[argName]; ok {
			return filterCompletions(candidates, value)
		}
	}
	for _, t := range AllTools() {
		if t.Name != toolName {
			continue
		}
		if prop, ok := t.InputSchema.Properties[argName]; ok && len(prop.Enum) > 0 {
			return filterCompletions(prop.Enum, value)
		}
		break
	}
	return nil
}

// filterCompletions returns candidates whose lowercase form starts with the
// lowercase prefix; an empty prefix returns all candidates.
func filterCompletions(candidates []string, prefix string) []string {
	prefix = strings.ToLower(strings.TrimSpace(prefix))
	out := make([]string, 0, len(candidates))
	for _, c := range candidates {
		if prefix == "" || strings.HasPrefix(strings.ToLower(c), prefix) {
			out = append(out, c)
		}
	}
	return out
}

func promptNames() []string {
	prompts := AllPrompts()
	names := make([]string, 0, len(prompts))
	for _, p := range prompts {
		names = append(names, p.Name)
	}
	return names
}

func resourceURIs() []string {
	uris := make([]string, 0, 8)
	for _, r := range AllResources() {
		uris = append(uris, r.URI)
	}
	for _, t := range AllResourceTemplates() {
		uris = append(uris, t.URITemplate)
	}
	return uris
}
