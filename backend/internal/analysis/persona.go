package analysis

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PersonaProfile struct {
	PersonID              string                `json:"person_id"`
	QQ                    string                `json:"qq"`
	DisplayName           string                `json:"display_name"`
	BiographicalAnchors   []BiographicalAnchor  `json:"biographical_anchors"`
	PsychologicalDefense  PsychologicalDefense  `json:"psychological_defense"`
	VerbatimAnchorQuotes  []VerbatimQuote       `json:"verbatim_anchor_quotes"`
	LinguisticFingerprint LinguisticFingerprint `json:"linguistic_fingerprint"`
	InterestSpectrum      []InterestDomain      `json:"interest_spectrum"`
	SocialArchetype       SocialArchetype       `json:"social_archetype"`
	EvidenceCount         int                   `json:"evidence_count"`
	EvidenceMessageIDs    []string              `json:"evidence_message_ids"`
	InferenceID           string                `json:"inference_id"`
	AnalyzedAt            time.Time             `json:"analyzed_at"`
}

type BiographicalAnchor struct {
	Aspect           string `json:"aspect"`
	Detail           string `json:"detail"`
	VerbatimEvidence string `json:"verbatim_evidence"`
}

type PsychologicalDefense struct {
	CoreWoundAndInsecurity string `json:"core_wound_and_insecurity"`
	DefenseMechanism       string `json:"defense_mechanism"`
	EmpathyAndAttachment   string `json:"empathy_and_attachment"`
}

type VerbatimQuote struct {
	Quote   string `json:"quote"`
	Context string `json:"context"`
}

type LinguisticFingerprint struct {
	Catchphrases       []string `json:"catchphrases"`
	SlangAndSubculture []string `json:"slang_and_subculture"`
	SentenceStyle      string   `json:"sentence_style"`
	ToneBaseline       string   `json:"tone_baseline"`
}

type InterestDomain struct {
	Domain             string   `json:"domain"`
	Score              float64  `json:"score"`
	SpecificEntities   []string `json:"specific_entities"`
	ContextDescription string   `json:"context_description"`
}

type SocialArchetype struct {
	PrimaryRole       string `json:"primary_role"`
	CommunityFunction string `json:"community_function"`
	InfluenceScore    int    `json:"influence_score"` // 1-100
	DeepAnalysis      string `json:"deep_analysis"`
}

func decodeStringList(raw json.RawMessage) ([]string, error) {
	var list []string
	if err := json.Unmarshal(raw, &list); err == nil {
		return list, nil
	}
	var one string
	if err := json.Unmarshal(raw, &one); err == nil {
		if strings.TrimSpace(one) == "" {
			return []string{}, nil
		}
		return []string{one}, nil
	}
	return nil, fmt.Errorf("expected string or string array")
}

func (l *LinguisticFingerprint) UnmarshalJSON(data []byte) error {
	var raw struct {
		Catchphrases  json.RawMessage `json:"catchphrases"`
		Slang         json.RawMessage `json:"slang_and_subculture"`
		SentenceStyle string          `json:"sentence_style"`
		ToneBaseline  string          `json:"tone_baseline"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	var err error
	l.Catchphrases, err = decodeStringList(raw.Catchphrases)
	if err != nil {
		return fmt.Errorf("catchphrases: %w", err)
	}
	l.SlangAndSubculture, err = decodeStringList(raw.Slang)
	if err != nil {
		return fmt.Errorf("slang_and_subculture: %w", err)
	}
	l.SentenceStyle, l.ToneBaseline = raw.SentenceStyle, raw.ToneBaseline
	return nil
}

func (d *InterestDomain) UnmarshalJSON(data []byte) error {
	var raw struct {
		Domain   string          `json:"domain"`
		Score    float64         `json:"score"`
		Entities json.RawMessage `json:"specific_entities"`
		Context  string          `json:"context_description"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	entities, err := decodeStringList(raw.Entities)
	if err != nil {
		return fmt.Errorf("specific_entities: %w", err)
	}
	d.Domain, d.Score, d.SpecificEntities, d.ContextDescription = raw.Domain, raw.Score, entities, raw.Context
	return nil
}

func sanitizeMessageText(text string) string {
	text = strings.TrimSpace(text)
	// Truncate excessively long message lines
	if len([]rune(text)) > 120 {
		text = string([]rune(text)[:120]) + "..."
	}
	return text
}

func fitPersonaContextBudget(messages, evidenceIDs []string, runeBudget int) ([]string, []string) {
	if runeBudget <= 0 || len(messages) == 0 {
		return messages, evidenceIDs
	}
	totalRunes := 0
	for _, message := range messages {
		totalRunes += len([]rune(message)) + 1
	}
	if totalRunes <= runeBudget {
		return messages, evidenceIDs
	}
	averageRunes := max(1, totalRunes/len(messages))
	targetCount := max(1, min(len(messages), runeBudget/averageRunes))
	selectedMessages := make([]string, 0, targetCount)
	selectedIDs := make([]string, 0, targetCount)
	usedRunes := 0
	lastIndex := -1
	for i := 0; i < targetCount; i++ {
		index := len(messages) - 1
		if targetCount > 1 {
			index = i * (len(messages) - 1) / (targetCount - 1)
		}
		if index == lastIndex {
			continue
		}
		lineRunes := len([]rune(messages[index])) + 1
		if usedRunes+lineRunes > runeBudget {
			continue
		}
		selectedMessages = append(selectedMessages, messages[index])
		if index < len(evidenceIDs) {
			selectedIDs = append(selectedIDs, evidenceIDs[index])
		}
		usedRunes += lineRunes
		lastIndex = index
	}
	return selectedMessages, selectedIDs
}

// AnalyzePersonPersona performs deep, forensic-level persona profiling for a given person or QQ.
func AnalyzePersonPersona(ctx context.Context, pool *pgxpool.Pool, personIDOrQQ string) (*PersonaProfile, error) {
	now := time.Now()

	// 1. Resolve Person ID and QQ
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

	// 2. Fetch all meaningful historical message samples with reply/dialogue context
	contextDepth := 150
	if pool != nil {
		var depthVal int
		if err := pool.QueryRow(ctx, `SELECT (value #>> '{}')::int FROM system_configs WHERE key = 'ai.context_depth'`).Scan(&depthVal); err == nil && depthVal > 0 {
			contextDepth = depthVal
		}
	}

	msgRows, err := pool.Query(ctx, `
		WITH ranked_messages AS (
			SELECT
				m.id::text AS message_id,
				COALESCE(m.raw_text, '') AS raw_text,
				m.sent_at,
				COALESCE(parent_p.display_name, '') AS parent_name,
				COALESCE(parent.raw_text, '') AS parent_text,
				row_number() OVER (ORDER BY m.sent_at ASC) AS ascending_rank,
				row_number() OVER (ORDER BY m.sent_at DESC) AS descending_rank
		FROM messages m
		LEFT JOIN messages parent ON parent.source_message_id = m.reply_to_message_id
		LEFT JOIN persons parent_p ON parent_p.id = parent.sender_id
		WHERE m.sender_id = $1::uuid AND (m.raw_text IS NOT NULL AND m.raw_text NOT LIKE '[CQ:image%')
		)
		SELECT message_id, raw_text, sent_at, parent_name, parent_text
		FROM ranked_messages
		WHERE ascending_rank <= (($2 + 1) / 2) OR descending_rank <= ($2 / 2)
		ORDER BY sent_at ASC
	`, personID, contextDepth)
	if err != nil {
		return nil, fmt.Errorf("query sender messages: %w", err)
	}
	defer msgRows.Close()

	var sampleMessages []string
	var evidenceIDs []string
	for msgRows.Next() {
		var mid, txt, parentName, parentTxt string
		var sentAt time.Time
		if err := msgRows.Scan(&mid, &txt, &sentAt, &parentName, &parentTxt); err == nil {
			cleanTxt := sanitizeMessageText(txt)
			if cleanTxt != "" {
				evidenceIDs = append(evidenceIDs, mid)
				var line string
				if parentTxt != "" {
					cleanParent := sanitizeMessageText(parentTxt)
					if parentName == "" {
						parentName = "群友"
					}
					line = fmt.Sprintf("[%s] (回复了 %s: \"%s\"): %s", sentAt.Format("2006-01-02 15:04"), parentName, cleanParent, cleanTxt)
				} else {
					line = fmt.Sprintf("[%s]: %s", sentAt.Format("2006-01-02 15:04"), cleanTxt)
				}
				sampleMessages = append(sampleMessages, line)
			}
		}
	}
	msgRows.Close()
	contextRuneBudget := 12000
	if pool != nil {
		var configuredBudget int
		if err := pool.QueryRow(ctx, `SELECT (value #>> '{}')::int FROM system_configs WHERE key = 'ai.persona_input_char_budget'`).Scan(&configuredBudget); err == nil && configuredBudget > 0 {
			contextRuneBudget = configuredBudget
		}
	}
	sampleMessages, evidenceIDs = fitPersonaContextBudget(sampleMessages, evidenceIDs, contextRuneBudget)

	if len(sampleMessages) == 0 {
		return nil, fmt.Errorf("该用户在数据库中没有足够的文字发言记录用于深度画像分析")
	}

	cfg := GetActiveLLMConfig(ctx, pool)
	// Persona output is consumed as a persisted JSON record. Ask compatible
	// providers for JSON natively so parsing does not depend on prompt obedience.
	cfg.JSONMode = true
	if cfg.ThinkingMode == "" {
		cfg.ThinkingMode = "disabled"
	}

	systemPrompt := `你是一名极其严谨的计算语言学与社交行为刑侦级研判专家。
你的任务是对目标用户的聊天流水进行【高保真、去伪存真、杜绝一切张冠李戴】的微观全息画像。

【最高研判铁律 - 坚决杜绝把别人的事安在目标头上】：
1. 【严格区分第一人称自述与第三方评价/调侃/反问】：
   - 只有当目标用户使用明确的【第一人称自述】（如“我妈带大的”、“我初中毕业才有手机”、“我过敏”、“我家也管”、“我小时候”）时，才能作为其本人的生平事实。
   - 【严禁张冠李戴】：用户对群友的调侃、回复、建议、疑问或对社会热点的讨论，绝对不能算作本人特征！
     * 错误示例反面教材：用户说“mtf女同吗，有点意思”是他在评价/反问别人的标签或话题，绝非自称跨性别！
     * 错误示例反面教材：用户对群友说“挂壁减小开销呢”是给他人出主意或接梗调侃，绝非本人经济现状！
     * 错误示例反面教材：讨论公共技术、医学、心理学名词时，绝不能直接扣在用户头上，除非他明确自述“我患有...”。
2. 【生平事实与成长轨迹 (biographical_anchors)】：
   - 必须是 100% 确定为用户本人亲历的客观事实拼图（家庭身世、亲人变故、求学阶段、早年数码设备购置经历、生理过敏反应、真实作息习惯等）。
   - 每条必须附带无可辩驳的第一人称原话佐证。
3. 【心理防御机制与人格暗线 (psychological_defense)】：
   - 深入剖析其【第一人称表露】的自卑、自嘲、防御模式（如主动自贬“我就是个躺平废人”、“不像我一样”以降低他人预期；对弱小流浪动物的深度共情与保护欲等）。
4. 【典型原话金句库 (verbatim_anchor_quotes)】：
   - 精选 3-5 句最能体现其真实身世经历或性格反差的【第一人称自述原话】及心理透视。
5. 【微观语言特征与口癖 (linguistic_fingerprint)】：
   - 必须是真实的高频口语词汇（如“这倒不至于”、“怕挨揍”、“不像我一样”、“废了”、“强强”、“真怕被人给爱了”），严禁纯标点或 emoji。
   - 提取圈子暗语或反讽黑话（如“带专”、“被人给爱了”）。
6. 【具象化领域图谱 (interest_spectrum)】：
   - 必须精确到具体微观领域（如“流浪动物救助与反虐猫关注”、“低预算设备与童年数码代偿”、“逆向与爬虫技术求知”）。
7. 【社群生态定位 (social_archetype)】：
   - 深入剖析其在群聊中的深层心理需求与角色定位。
8. 【JSON 语法规范】：
   - 必须输出合法标准 JSON，不要输出任何思考过程或前置说明；
   - 字符串值内部若需引用词汇，必须使用单引号 'xxx'，绝对禁止使用未转义的英文字符双引号！
   - verbatim_evidence 必须是单个字符串（多条证明请用分号分隔，如 "原话1；原话2"），绝对禁止输出逗号分隔的多个散落字符串！

请直接以合法标准 JSON 格式输出：
{
  "biographical_anchors": [
    {"aspect": "维度名称", "detail": "极度具体的事实描述", "verbatim_evidence": "无可辩驳的第一人称原话证明"}
  ],
  "psychological_defense": {
    "core_wound_and_insecurity": "核心自卑点与敏感区深度剖析",
    "defense_mechanism": "防御模式剖析",
    "empathy_and_attachment": "共情与依恋特征"
  },
  "verbatim_anchor_quotes": [
    {"quote": "第一人称原话文本", "context": "心理透视与语境说明"}
  ],
  "linguistic_fingerprint": {
    "catchphrases": ["词1", "词2"],
    "slang_and_subculture": ["特定圈子用语或黑话"],
    "sentence_style": "句式与分段特征",
    "tone_baseline": "核心语调底色"
  },
  "interest_spectrum": [
    {"domain": "具象领域", "score": 0.90, "specific_entities": ["具体实体"], "context_description": "具体关注点"}
  ],
  "social_archetype": {
    "primary_role": "具象角色",
    "community_function": "在群内的心理定位与社交功能",
    "influence_score": 75,
    "deep_analysis": "深入且击中本质的行为学与心理学画像总结"
  }
}`

	userPrompt := fmt.Sprintf(`请深度研判以下目标用户的真实聊天流水（包含回复群友上下文）：
【目标用户】%s (QQ: %s)
【历史发言流水 (%d条)】
%s

请输出严格去伪存真、拒绝张冠李戴的深度全息画像 JSON：
`, displayName, qq, len(sampleMessages), strings.Join(sampleMessages, "\n"))

	temperature := 0.1
	if pool != nil {
		var dynamicPrompt string
		if qErr := pool.QueryRow(ctx, `SELECT value #>> '{}' FROM system_configs WHERE key='ai.persona_system_prompt'`).Scan(&dynamicPrompt); qErr == nil && strings.TrimSpace(dynamicPrompt) != "" {
			systemPrompt = dynamicPrompt
		}

		var tempVal float64
		if qErr := pool.QueryRow(ctx, `SELECT (value #>> '{}')::float FROM system_configs WHERE key='ai.temperature'`).Scan(&tempVal); qErr == nil && tempVal >= 0 && tempVal <= 1.0 {
			temperature = tempVal
		}

		var customPrompt string
		if qErr := pool.QueryRow(ctx, `SELECT value #>> '{}' FROM system_configs WHERE key='ai.custom_prompt'`).Scan(&customPrompt); qErr == nil && strings.TrimSpace(customPrompt) != "" {
			userPrompt += fmt.Sprintf("\n【附加系统指令】:\n%s\n", strings.TrimSpace(customPrompt))
		}

		var strictMode bool
		if qErr := pool.QueryRow(ctx, `SELECT (value #>> '{}')::boolean FROM system_configs WHERE key='ai.strict_mode'`).Scan(&strictMode); qErr == nil && !strictMode {
			userPrompt += "\n【注意】当前允许适当推断非第一人称上下文。\n"
		}
	}
	systemPrompt += `

【输出可靠性约束】
- 只输出 JSON，不要输出思考过程、Markdown、解释文字或代码围栏。
- 所有字段必须存在；没有证据时使用空数组或空字符串，不要编造内容。
- biographical_anchors 最多 5 条，verbatim_anchor_quotes 最多 5 条，interest_spectrum 最多 6 条。
- 每个 detail、context、deep_analysis 字符串尽量控制在 120 个汉字以内，避免输出被截断。
- influence_score 使用 0-100 的整数，interest_spectrum.score 使用 0-1 的数字。
`

	messages := []ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}

	rawResp, err := CallLLM(ctx, cfg, messages, temperature)
	if err != nil {
		return nil, fmt.Errorf("llm persona analysis: %w", err)
	}

	cleanedJSON := CleanJSONResponse(rawResp)

	var parsed struct {
		BiographicalAnchors   []BiographicalAnchor  `json:"biographical_anchors"`
		PsychologicalDefense  PsychologicalDefense  `json:"psychological_defense"`
		VerbatimAnchorQuotes  []VerbatimQuote       `json:"verbatim_anchor_quotes"`
		LinguisticFingerprint LinguisticFingerprint `json:"linguistic_fingerprint"`
		InterestSpectrum      []InterestDomain      `json:"interest_spectrum"`
		SocialArchetype       SocialArchetype       `json:"social_archetype"`
	}

	if err := json.Unmarshal([]byte(cleanedJSON), &parsed); err != nil {
		return nil, fmt.Errorf("unmarshal persona json: %w (raw: %s)", err, cleanedJSON)
	}

	if len(parsed.BiographicalAnchors) == 0 && len(parsed.VerbatimAnchorQuotes) == 0 && parsed.PsychologicalDefense.CoreWoundAndInsecurity == "" {
		return nil, fmt.Errorf("大模型未返回完整的画像结构（可能因思维链过长耗尽 Token），请调小上下文深度或重新研判")
	}

	valBytes, _ := json.Marshal(parsed)

	// Save to inferences table with bound evidence
	var inferenceID string
	_ = pool.QueryRow(ctx, `
		INSERT INTO inferences(
			subject_id, attribute_type, value, confidence, method_version,
			evidence_event_ids, review_status, created_at
		) VALUES (
			$1, 'persona_profile', $2::jsonb, 0.98, 'forensic-persona-v2.0',
			$3::uuid[], 'approved', $4
		) RETURNING id::text
	`, personID, string(valBytes), evidenceIDs[:min(len(evidenceIDs), 50)], now).Scan(&inferenceID)

	return &PersonaProfile{
		PersonID:              personID,
		QQ:                    qq,
		DisplayName:           displayName,
		BiographicalAnchors:   parsed.BiographicalAnchors,
		PsychologicalDefense:  parsed.PsychologicalDefense,
		VerbatimAnchorQuotes:  parsed.VerbatimAnchorQuotes,
		LinguisticFingerprint: parsed.LinguisticFingerprint,
		InterestSpectrum:      parsed.InterestSpectrum,
		SocialArchetype:       parsed.SocialArchetype,
		EvidenceCount:         len(sampleMessages),
		EvidenceMessageIDs:    evidenceIDs[:min(len(evidenceIDs), 50)],
		InferenceID:           inferenceID,
		AnalyzedAt:            now,
	}, nil
}
