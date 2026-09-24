package bot

import (
	"context"
	"encoding/json"
	"math/rand"
	"strings"
)

// nrand returns a random int in [0, n).
func (s *Service) nrand(n int) int {
	if n <= 0 {
		return 0
	}
	return rand.Intn(n)
}

// entertain rolls a small probability (reply_probability%) to reply with a
// random line from the configurable entertainment corpus. Empty = no reply.
func (s *Service) entertain(ctx context.Context, inst Instance, msg message) string {
	if inst.ReplyProbability <= 0 {
		return ""
	}
	if s.nrand(100) >= inst.ReplyProbability {
		return ""
	}
	return s.entertainLine(inst)
}

// entertainLine picks a random line from the instance corpus, falling back to
// a persona-flavored default.
func (s *Service) entertainLine(inst Instance) string {
	var corpus []string
	if len(inst.Entertainment) > 0 {
		if err := json.Unmarshal(inst.Entertainment, &corpus); err != nil || len(corpus) == 0 {
			corpus = nil
		}
	}
	if len(corpus) == 0 {
		corpus = []string{
			inst.MeowSound + " 喵喵！",
			"谁在叫老吴？" + inst.MeowSound,
			"蹭蹭你，今天也要开心喵~",
			"老吴老吴，代码敲累了吗？摸鱼喵！",
			"喵呜~ " + inst.MeowSound,
		}
	}
	return strings.TrimSpace(corpus[s.nrand(len(corpus))])
}
