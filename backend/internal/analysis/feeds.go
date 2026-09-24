package analysis

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type FeedDynamicsResult struct {
	PersonID            string               `json:"person_id"`
	QQ                  string               `json:"qq"`
	DisplayName         string               `json:"display_name"`
	TotalPosts          int                  `json:"total_posts"`
	TotalInteractions   int                  `json:"total_interactions"`
	SentimentFlow       []MonthlySentiment   `json:"sentiment_flow"`
	NarrativeMilestones []NarrativeMilestone `json:"narrative_milestones"`
	CircleHierarchy     CircleHierarchy      `json:"circle_hierarchy"`
	CircleSummary       string               `json:"circle_summary"`
	InferenceID         string               `json:"inference_id"`
	AnalyzedAt          time.Time            `json:"analyzed_at"`
}

type MonthlySentiment struct {
	Month           string  `json:"month"`
	SentimentScore  float64 `json:"sentiment_score"` // -1.0 to 1.0
	DominantEmotion string  `json:"dominant_emotion"`
	PostCount       int     `json:"post_count"`
	Summary         string  `json:"summary"`
}

type NarrativeMilestone struct {
	Date        string `json:"date"`
	Type        string `json:"type"` // 'mood_shift', 'milestone', 'circle_change', 'identity_shift'
	Description string `json:"description"`
	Evidence    string `json:"evidence"`
}

type CircleHierarchy struct {
	Tier1Speedy []CircleMember `json:"tier1_speedy"` // 秒赞秒评 (5分钟内)
	Tier2Deep   []CircleMember `json:"tier2_deep"`   // 深度常评互动
	Tier3Casual []CircleMember `json:"tier3_casual"` // 轻量/偶发互动
}

type CircleMember struct {
	QQ          string `json:"qq"`
	Name        string `json:"name"`
	Count       int    `json:"count"`
	Interaction string `json:"interaction"`
}

// AnalyzeFeedDynamics performs deep sentiment flow and social circle hierarchy analysis on a person's QZone feeds.
func AnalyzeFeedDynamics(ctx context.Context, pool *pgxpool.Pool, personIDOrQQ string) (*FeedDynamicsResult, error) {
	// 1. Resolve Person ID
	var personID, qq, displayName string
	err := pool.QueryRow(ctx, `
		SELECT p.id::text, COALESCE(pi.platform_user_id, ''), COALESCE(p.display_name, '')
		FROM persons p
		LEFT JOIN person_identifiers pi ON pi.person_id = p.id AND pi.platform = 'qq'
		WHERE p.id::text = $1 OR pi.platform_user_id = $1
		LIMIT 1
	`, personIDOrQQ).Scan(&personID, &qq, &displayName)
	if err != nil {
		return nil, fmt.Errorf("resolve target person: %w", err)
	}

	// 2. Fetch QZone feeds according to qzone.time_window
	timeWindow := "1year"
	tier1Minutes := 5
	tier2MinCount := 3
	if pool != nil {
		_ = pool.QueryRow(ctx, `SELECT COALESCE(value #>> '{}', '1year') FROM system_configs WHERE key = 'qzone.time_window'`).Scan(&timeWindow)
		_ = pool.QueryRow(ctx, `SELECT COALESCE((value #>> '{}')::int, 5) FROM system_configs WHERE key = 'qzone.tier1_minutes'`).Scan(&tier1Minutes)
		_ = pool.QueryRow(ctx, `SELECT COALESCE((value #>> '{}')::int, 3) FROM system_configs WHERE key = 'qzone.tier2_min_count'`).Scan(&tier2MinCount)
	}

	timeClause := ""
	switch timeWindow {
	case "3months":
		timeClause = "AND c.published_at >= NOW() - INTERVAL '3 months'"
	case "6months":
		timeClause = "AND c.published_at >= NOW() - INTERVAL '6 months'"
	case "1year":
		timeClause = "AND c.published_at >= NOW() - INTERVAL '1 year'"
	}

	query := fmt.Sprintf(`
		SELECT c.id::text, c.body, c.published_at,
		       (SELECT COUNT(*) FROM relation_events re WHERE re.target_object_id = c.id AND re.action_type = 'liked') as like_count,
		       (SELECT COUNT(*) FROM relation_events re WHERE re.target_object_id = c.id AND re.action_type = 'commented') as comment_count
		FROM contents c
		WHERE c.author_id = $1::uuid AND c.body IS NOT NULL AND TRIM(c.body) != '' %s
		ORDER BY c.published_at DESC
		LIMIT 50
	`, timeClause)

	rows, err := pool.Query(ctx, query, personID)
	if err != nil {
		return nil, fmt.Errorf("query author feeds: %w", err)
	}
	defer rows.Close()

	type feedItem struct {
		ID        string
		Body      string
		PubAt     *time.Time
		LikeCount int
		CommCount int
	}
	var feeds []feedItem
	var evidenceIDs []string
	for rows.Next() {
		var item feedItem
		if err := rows.Scan(&item.ID, &item.Body, &item.PubAt, &item.LikeCount, &item.CommCount); err == nil {
			item.Body = sanitizeMessageText(item.Body)
			feeds = append(feeds, item)
			evidenceIDs = append(evidenceIDs, item.ID)
		}
	}
	rows.Close()

	if len(feeds) == 0 {
		return nil, fmt.Errorf("该用户在空间中没有已采集的说说动态 (时间窗口: %s)", timeWindow)
	}

	// 3. Compute Objective Circle Hierarchy from relation_events
	tier1Map := make(map[string]*CircleMember)
	tier2Map := make(map[string]*CircleMember)
	tier3Map := make(map[string]*CircleMember)

	relRows, err := pool.Query(ctx, `
		SELECT re.action_type, re.occurred_at, c.published_at,
		       COALESCE(pi.platform_user_id, ''), COALESCE(p.display_name, '')
		FROM relation_events re
		JOIN contents c ON c.id = re.target_object_id
		LEFT JOIN persons p ON p.id = re.actor_person_id
		LEFT JOIN person_identifiers pi ON pi.person_id = p.id AND pi.platform = 'qq'
		WHERE c.author_id = $1::uuid AND re.actor_person_id != $1::uuid
		ORDER BY re.occurred_at ASC
	`, personID)
	if err == nil {
		defer relRows.Close()
		for relRows.Next() {
			var actType string
			var occAt, pubAt *time.Time
			var actorQQ, actorName string
			if err := relRows.Scan(&actType, &occAt, &pubAt, &actorQQ, &actorName); err == nil && actorQQ != "" {
				name := actorName
				if name == "" {
					name = actorQQ
				}

				if occAt != nil && pubAt != nil && occAt.Sub(*pubAt) <= time.Duration(tier1Minutes)*time.Minute && occAt.After(*pubAt) {
					if m, exists := tier1Map[actorQQ]; exists {
						m.Count++
					} else {
						tier1Map[actorQQ] = &CircleMember{QQ: actorQQ, Name: name, Count: 1, Interaction: fmt.Sprintf("%d分钟内极速秒赞/秒评", tier1Minutes)}
					}
				} else if actType == "commented" {
					if m, exists := tier2Map[actorQQ]; exists {
						m.Count++
					} else {
						tier2Map[actorQQ] = &CircleMember{QQ: actorQQ, Name: name, Count: 1, Interaction: "深度评论互动"}
					}
				} else {
					if m, exists := tier3Map[actorQQ]; exists {
						m.Count++
					} else {
						tier3Map[actorQQ] = &CircleMember{QQ: actorQQ, Name: name, Count: 1, Interaction: "点赞/偶发互动"}
					}
				}
			}
		}
		relRows.Close()
	}

	tier1 := make([]CircleMember, 0, len(tier1Map))
	for _, m := range tier1Map {
		tier1 = append(tier1, *m)
	}
	tier2 := make([]CircleMember, 0, len(tier2Map))
	tier3 := make([]CircleMember, 0, len(tier3Map))
	for _, m := range tier2Map {
		if m.Count >= tier2MinCount {
			tier2 = append(tier2, *m)
		} else {
			m.Interaction = "偶发评论互动"
			tier3 = append(tier3, *m)
		}
	}
	for _, m := range tier3Map {
		tier3 = append(tier3, *m)
	}

	// 4. Prepare prompt for LLM Sentiment & Narrative extraction
	var feedTextList []string
	for _, f := range feeds {
		dateStr := "未知时间"
		if f.PubAt != nil {
			dateStr = f.PubAt.Format("2006-01-02 15:04")
		}
		feedTextList = append(feedTextList, fmt.Sprintf("[%s, 点赞:%d, 评论:%d]: %s", dateStr, f.LikeCount, f.CommCount, f.Body))
	}

	cfg := GetActiveLLMConfig(ctx, pool)
	systemPrompt := `你是一名专业的开源情报（OSINT）与心理学叙事研判专家。
请根据目标用户的历史空间说说动态，提取其【时序情感波动曲线 (Sentiment Flow)】、【人生/心态关键转折点 (Narrative Milestones)】与【社交密友圈层概述】。

输出规范：
1. sentiment_flow 按月份聚合（如 "2025-10"），给出 -1.0(极度负面/悲伤) 到 1.0(极度积极/喜悦) 的情感分值，并给出核心情绪关键词。
2. narrative_milestones 提炼出值得关注的心态变化或生活事件节点。
3. 严格输出标准 JSON，不要输出任何多余解释。`

	fewShotExample := `
【标准输出示例】
{
  "sentiment_flow": [
    {
      "month": "2025-10",
      "sentiment_score": 0.75,
      "dominant_emotion": "成就感与期待",
      "post_count": 3,
      "summary": "集中分享新项目上线与技术探索心得，整体积极活跃。"
    },
    {
      "month": "2026-01",
      "sentiment_score": -0.20,
      "dominant_emotion": "疲惫与调侃",
      "post_count": 2,
      "summary": "提及年底事务繁多，以吐槽调侃生活解压为主。"
    }
  ],
  "narrative_milestones": [
    {
      "date": "2025-10",
      "type": "milestone",
      "description": "技术突破与新阶段开启",
      "evidence": "发布项目成果展示说说"
    }
  ],
  "circle_summary": "该用户空间互动呈现明显的极速好友圈与深度讨论圈分层，少数核心联系人在动态发布初期提供高频反馈，其余为泛社交关系。"
}`

	userPrompt := fmt.Sprintf(`请分析以下用户的空间说说记录：
【目标用户】QQ: %s, 昵称: %s, 动态总数: %d

【历史动态列表】
%s

请严格参考上述标准格式输出 JSON：
`, qq, displayName, len(feeds), strings.Join(feedTextList, "\n"))

	if pool != nil {
		var dynamicPrompt string
		if qErr := pool.QueryRow(ctx, `SELECT value #>> '{}' FROM system_configs WHERE key='ai.feed_system_prompt'`).Scan(&dynamicPrompt); qErr == nil && strings.TrimSpace(dynamicPrompt) != "" {
			systemPrompt = dynamicPrompt
		}
	}

	messages := []ChatMessage{
		{Role: "system", Content: systemPrompt + "\n" + fewShotExample},
		{Role: "user", Content: userPrompt},
	}

	rawResp, err := CallLLM(ctx, cfg, messages, 0.1)
	if err != nil {
		return nil, fmt.Errorf("llm feed dynamics analysis: %w", err)
	}

	cleanJSON := CleanJSONResponse(rawResp)
	var parsed struct {
		SentimentFlow       []MonthlySentiment   `json:"sentiment_flow"`
		NarrativeMilestones []NarrativeMilestone `json:"narrative_milestones"`
		CircleSummary       string               `json:"circle_summary"`
	}

	if err := json.Unmarshal([]byte(cleanJSON), &parsed); err != nil {
		return nil, fmt.Errorf("unmarshal feed dynamics json: %w (raw: %s)", err, cleanJSON)
	}

	now := time.Now()
	res := &FeedDynamicsResult{
		PersonID:            personID,
		QQ:                  qq,
		DisplayName:         displayName,
		TotalPosts:          len(feeds),
		TotalInteractions:   len(tier1) + len(tier2) + len(tier3),
		SentimentFlow:       parsed.SentimentFlow,
		NarrativeMilestones: parsed.NarrativeMilestones,
		CircleHierarchy: CircleHierarchy{
			Tier1Speedy: tier1,
			Tier2Deep:   tier2,
			Tier3Casual: tier3,
		},
		CircleSummary: parsed.CircleSummary,
		AnalyzedAt:    now,
	}

	valBytes, _ := json.Marshal(res)
	var inferenceID string
	_ = pool.QueryRow(ctx, `
		INSERT INTO inferences(
			subject_id, attribute_type, value, confidence, method_version,
			evidence_event_ids, review_status, created_at
		) VALUES (
			$1, 'feed_dynamics', $2::jsonb, 0.95, 'vlm-feeds-v1.0',
			$3::uuid[], 'approved', $4
		) RETURNING id::text
	`, personID, string(valBytes), evidenceIDs[:min(len(evidenceIDs), 30)], now).Scan(&inferenceID)
	res.InferenceID = inferenceID

	return res, nil
}
