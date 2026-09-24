package analysis

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestParseRetryAfter(t *testing.T) {
	if got := parseRetryAfter("7"); got != 7*time.Second {
		t.Fatalf("expected 7 seconds, got %v", got)
	}
	if got := parseRetryAfter(""); got != 0 {
		t.Fatalf("expected empty Retry-After to return zero, got %v", got)
	}
}

func TestCallLLMPacesBackToBackRequests(t *testing.T) {
	var mu sync.Mutex
	var received []time.Time
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		received = append(received, time.Now())
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer server.Close()

	cfg := LLMConfig{
		APIBase:            server.URL,
		APIKey:             "test-key",
		Model:              "test-pacing",
		InternalAttempts:   1,
		RequestTimeout:     time.Second,
		MinRequestInterval: 35 * time.Millisecond,
	}
	if _, err := CallLLM(context.Background(), cfg, []ChatMessage{{Role: "user", Content: "one"}}, 0); err != nil {
		t.Fatalf("first call failed: %v", err)
	}
	if _, err := CallLLM(context.Background(), cfg, []ChatMessage{{Role: "user", Content: "two"}}, 0); err != nil {
		t.Fatalf("second call failed: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(received))
	}
	if gap := received[1].Sub(received[0]); gap < 25*time.Millisecond {
		t.Fatalf("back-to-back requests were not paced: gap=%v", gap)
	}
}

func TestCleanJSONResponseRepairsUnescapedInnerQuotes(t *testing.T) {
	raw := `{
		"biographical_anchors": [
			{"aspect": "家庭身世", "detail": "父亲在童年时期因癌症去世，由母亲独自抚养长大", "verbatim_evidence": "我是我妈带大的，我爸在我小时候就癌症走了"},
			{"aspect": "教育背景", "detail": "初中毕业才拥有第一部手机，后来在专科院校通过奖学金购买电脑", "verbatim_evidence": "我初中毕业才有的手机，电脑还是后来上带专拿奖学金买的"},
			{"aspect": "社交经历", "detail": "初中及以前人缘很差，曾因在同学家车上学习骂人行为而感到羞愧", "verbatim_evidence": "我初中及以前人缘很差，我记得小时候去同学家的路上在他家车上学阿Q 骂人，现在都还记得，真觉得丢人"},
			{"aspect": "生理特征", "detail": "有过敏问题，无法饲养宠物", "verbatim_evidence": "我过敏完全无法考虑"},
			{"aspect": "作息习惯", "detail": "经常失眠，深夜活动频繁，有自我行为限制", "verbatim_evidence": "睡不着，又睡不着，出来吃麦当劳了，今天次数多了，不能🫎了"}
		],
		"psychological_defense": {
			"core_wound_and_insecurity": "核心自卑源于童年社交挫折和父亲早逝的家庭创伤，表现为对过去社交行为的持续羞愧和对自我价值的否定",
			"defense_mechanism": "通过主动自贬'我就是个躺平废人'、'不像我一样'、'废了'等表达降低他人预期，形成防御性自我保护机制",
			"empathy_and_attachment": "对流浪动物表现出强烈的共情和保护欲，'真怕他被人给爱了'反映其对弱小生命的深度关怀，可能是对自身童年缺失的情感投射"
		},
		"verbatim_anchor_quotes": [
			{"quote": "我是我妈带大的，我爸在我小时候就癌症走了", "context": "家庭背景的核心陈述，揭示其成长环境中的重大缺失"},
			{"quote": "我初中毕业才有的手机，电脑还是后来上带专拿奖学金买的", "context": "成长轨迹的标志性事件，反映其物质条件限制和通过努力获取资源的经历"},
			{"quote": "我花了好长时间才意识到那种感觉不是性欲，而是我需要多巴胺，因为不快乐所以需要高潮来获得快乐", "context": "对自我心理状态的深刻洞察，显示其自我认知能力和对情绪机制的思考"},
			{"quote": "我是个躺平废人，没法给这方面的建议", "context": "典型的自我贬低表达，反映其防御性人格特质"},
			{"quote": "真怕他被人给爱了", "context": "对流浪动物的共情和保护欲，展现其温柔敏感的一面"}
		],
		"linguistic_fingerprint": {
			"catchphrases": ["这倒不至于", "怕挨揍", "不像我一样", "废了", "强强", "真怕被人给爱了", "可以的"],
			"slang_and_subculture": ["带专", "被人给爱了"],
			"sentence_style": "简短直接的回复，常以"我"开头表达个人观点，频繁使用反问句式如"哪搞来这么多人的"、"啥玩意"、"啥原理"",
			"tone_baseline": "自我贬低与防御性语调为主，对技术话题表现出好奇探索，对流浪动物话题转为温柔关怀"
		},
		"interest_spectrum": [
			{"domain": "逆向工程与爬虫技术", "score": 0.85, "specific_entities": ["爬虫", "逆向工程", "成分分析"], "context_description": "对技术领域有强烈好奇心，主动询问接触途径和技术原理"},
			{"domain": "流浪动物救助与反虐猫关注", "score": 0.90, "specific_entities": ["流浪蓝猫", "梨花猫", "被领养"], "context_description": "对流浪动物有强烈保护欲，担心它们被不当对待"},
			{"domain": "低预算设备与童年数码代偿", "score": 0.80, "specific_entities": ["初中手机", "带专电脑", "奖学金"], "context_description": "关注早期数码设备获取经历，反映成长过程中的物质条件限制"},
			{"domain": "ADHD相关话题", "score": 0.75, "specific_entities": ["ADHD•清醒生存指南", "记忆问题"], "context_description": "在ADHD相关群组活跃，对记忆问题有共鸣"}
		],
		"social_archetype": {
			"primary_role": "技术探索者与流浪动物保护者",
			"community_function": "在群内既是技术话题的积极参与者，也是情感支持的提供者，通过自我暴露和共情建立社群连接",
			"influence_score": 75,
			"deep_analysis": "mikoto是一个在成长过程中经历了父亲早逝、家庭严格管教、社交困难等挑战的年轻人。他/她通过技术探索和自我认知来寻找身份认同，同时表现出对弱小动物的强烈共情和保护欲。其自我贬低的表达方式可能是对过去社交挫折的防御性反应，而多巴胺依赖的洞察则显示其对自身心理状态有较深的理解。在群聊中，他/她既是技术话题的积极参与者，也是情感支持的提供者，通过自我暴露和共情建立社群连接。"
		}
	}`

	cleaned := CleanJSONResponse(raw)
	var parsed map[string]any
	if err := json.Unmarshal([]byte(cleaned), &parsed); err != nil {
		t.Fatalf("CleanJSONResponse failed to produce valid json: %v\nCleaned JSON:\n%s", err, cleaned)
	}

	lf, ok := parsed["linguistic_fingerprint"].(map[string]any)
	if !ok {
		t.Fatalf("missing linguistic_fingerprint")
	}
	style, _ := lf["sentence_style"].(string)
	t.Logf("Successfully repaired sentence_style: %s", style)
}

func TestCleanJSONResponseRepairsDanglingStrings(t *testing.T) {
	raw := `{
  "biographical_anchors": [
    {
      "aspect": "地理居住与成长轨迹",
      "detail": "长期居住在加拿大，早年曾在美国佛罗里达生活数年，曾前往西班牙巴塞罗那度假并探亲（妻姐一家居住在市郊富人区）。",
      "verbatim_evidence": "我是在佛罗里达呆过几年的", "我在巴塞罗那市郊", "加拿大"
    },
    {
      "aspect": "教育背景与早期经历",
      "detail": "曾作为高中生赴美进行为期一年的交换生项目（J1签证），童年时期在北京中关村曾遭遇电子产品骗局。",
      "verbatim_evidence": "我是高中去的一年交换生", "小时候让中关村骗过", "J1"
    }
  ]
}`

	cleaned := CleanJSONResponse(raw)
	var parsed struct {
		BiographicalAnchors []BiographicalAnchor `json:"biographical_anchors"`
	}
	if err := json.Unmarshal([]byte(cleaned), &parsed); err != nil {
		t.Fatalf("CleanJSONResponse failed to repair dangling strings: %v\nCleaned JSON:\n%s", err, cleaned)
	}

	if len(parsed.BiographicalAnchors) != 2 {
		t.Fatalf("expected 2 anchors, got %d", len(parsed.BiographicalAnchors))
	}

	if parsed.BiographicalAnchors[0].VerbatimEvidence != "我是在佛罗里达呆过几年的；我在巴塞罗那市郊；加拿大" {
		t.Fatalf("unexpected verbatim_evidence: %s", parsed.BiographicalAnchors[0].VerbatimEvidence)
	}
	t.Logf("Successfully repaired verbatim_evidence: %s", parsed.BiographicalAnchors[0].VerbatimEvidence)
}

func TestCleanJSONResponseRepairsMismatchedQuotesAndSingleQuotes(t *testing.T) {
	raw := `{
  "linguistic_fingerprint": {
    "catchphrases": [
      "刷史高",
      "牛马",
      "带英",
      "泡面钱",
      "没事儿干的"
    ],
    "slang_and_subculture": [
      "牛马' (形容打工人),
      '带英' (对英国的蔑称),
      '刷史高' (刷新历史最高价),
      '光芯存' (光存储与半导体芯片的简称)
    ],
    'sentence_style": "喜欢使用技术术语（均线、财报、扩产）夹杂口语吐槽，句式简短有力，情绪波动大（从“卧槽”到“知足常乐”）。",
    "tone_baseline": "愤世嫉俗、技术流、自嘲、焦虑。"
  }
}`

	cleaned := CleanJSONResponse(raw)
	var parsed struct {
		LinguisticFingerprint LinguisticFingerprint `json:"linguistic_fingerprint"`
	}
	if err := json.Unmarshal([]byte(cleaned), &parsed); err != nil {
		t.Fatalf("CleanJSONResponse failed to repair mismatched quotes: %v\nCleaned JSON:\n%s", err, cleaned)
	}

	if len(parsed.LinguisticFingerprint.SlangAndSubculture) != 4 {
		t.Fatalf("expected 4 slang items, got %d", len(parsed.LinguisticFingerprint.SlangAndSubculture))
	}

	if parsed.LinguisticFingerprint.SentenceStyle == "" {
		t.Fatalf("sentence_style was not parsed")
	}

	t.Logf("Successfully repaired slang: %v", parsed.LinguisticFingerprint.SlangAndSubculture)
	t.Logf("Successfully repaired style: %s", parsed.LinguisticFingerprint.SentenceStyle)
}
