package normalization

import (
	"encoding/json"
	"fmt"
)

// ReplyTarget extracts the replied message ID from a "reply" segment, if present.
func ReplyTarget(segments json.RawMessage) string {
	var segs []map[string]any
	if json.Unmarshal(segments, &segs) != nil {
		return ""
	}
	for _, seg := range segs {
		if seg["type"] == "reply" {
			data, ok := seg["data"].(map[string]any)
			if !ok {
				continue
			}
			if id, ok := data["id"]; ok {
				return fmt.Sprint(id)
			}
		}
	}
	return ""
}

// MentionedQQs extracts all QQ numbers from "at" segments.
func MentionedQQs(segments json.RawMessage) []string {
	var segs []map[string]any
	if json.Unmarshal(segments, &segs) != nil {
		return nil
	}
	result := []string{}
	for _, seg := range segs {
		if seg["type"] != "at" {
			continue
		}
		data, ok := seg["data"].(map[string]any)
		if !ok {
			continue
		}
		qq := ""
		for _, key := range []string{"qq", "user_id", "user_uin", "uid"} {
			if v, ok := data[key]; ok && fmt.Sprint(v) != "" && fmt.Sprint(v) != "<nil>" {
				qq = fmt.Sprint(v)
				break
			}
		}
		if qq != "" && qq != "all" {
			result = append(result, qq)
		}
	}
	return result
}
