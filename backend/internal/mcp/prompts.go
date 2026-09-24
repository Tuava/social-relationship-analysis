package mcp

import (
	"fmt"
)

func AllPrompts() []Prompt {
	return []Prompt{
		{
			Name:        "osint_person_deep_dive",
			Description: "Conduct a comprehensive multi-dimensional OSINT investigation and persona analysis on a target QQ.",
			Arguments: []PromptArgument{
				{
					Name:        "target_qq",
					Description: "Target QQ account to investigate",
					Required:    true,
				},
			},
		},
		{
			Name:        "pairwise_relationship_audit",
			Description: "Audit connections, mutual communities, and social dynamics between two targets.",
			Arguments: []PromptArgument{
				{
					Name:        "source_qq",
					Description: "First QQ account",
					Required:    true,
				},
				{
					Name:        "target_qq",
					Description: "Second QQ account",
					Required:    true,
				},
			},
		},
	}
}

func GetPrompt(name string, args map[string]string) (*GetPromptResult, error) {
	switch name {
	case "osint_person_deep_dive":
		targetQQ := args["target_qq"]
		if targetQQ == "" {
			return nil, fmt.Errorf("target_qq is required")
		}
		instruction := fmt.Sprintf(`Please perform an evidence-based analysis of target QQ: %s.
Step 1: Use 'sra_person_dossier' to inspect identity, profile evolution, coverage, and group memberships.
Step 2: Use 'sra_ego_network' with depth=1 to identify the direct network and interaction partners.
Step 3: Use 'sra_content_feed' and 'sra_interaction_stream' to examine dated posts and interactions.
Step 4: Summarize the findings into a clear intelligence dossier covering:
- Target Profile & Digital Footprint
- Core Social Circle & Key Influencers
- Active Discussion Groups & Topics
- Behavioral Patterns and explicit data gaps.
Every material claim must cite returned evidence IDs or source records. Separate observed facts from hypotheses; do not assert sensitive identity, education, location, or intent without direct evidence.`, targetQQ)

		return &GetPromptResult{
			Description: fmt.Sprintf("OSINT Deep Dive on QQ %s", targetQQ),
			Messages: []PromptMessage{
				{
					Role: "user",
					Content: ContentItem{
						Type: "text",
						Text: instruction,
					},
				},
			},
		}, nil

	case "pairwise_relationship_audit":
		src := args["source_qq"]
		dst := args["target_qq"]
		if src == "" || dst == "" {
			return nil, fmt.Errorf("both source_qq and target_qq are required")
		}
		instruction := fmt.Sprintf(`Please audit the connection between %s and %s.
Step 1: Call 'sra_mutual_analysis' to find common groups, shared contacts, and direct interactions.
Step 2: Call 'sra_interaction_stream' and 'sra_content_feed' to examine dated contexts.
Step 3: Provide an objective assessment of relationship strength, interaction directionality, and shared communities.`, src, dst)

		return &GetPromptResult{
			Description: fmt.Sprintf("Pairwise Relationship Audit: %s <-> %s", src, dst),
			Messages: []PromptMessage{
				{
					Role: "user",
					Content: ContentItem{
						Type: "text",
						Text: instruction,
					},
				},
			},
		}, nil

	default:
		return nil, fmt.Errorf("unknown prompt: %s", name)
	}
}
