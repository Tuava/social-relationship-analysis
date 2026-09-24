package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/seagull/social-relationship-analysis/backend/internal/collectors/napcat"
)

// sendReply sends one chat message through the account's NapCat HTTP API.
// The message is prefixed with a reply reference when possible so the bot
// answers in-thread.
func (s *Service) sendReply(ctx context.Context, accountID string, inst Instance, msg message, reply string) error {
	acc, err := s.accountForSend(ctx, accountID)
	if err != nil {
		return err
	}
	client := napcat.NewHTTPClient(acc.HTTPURL, acc.HTTPToken)

	var endpoint string
	params := make(map[string]any)
	segments := make([]map[string]any, 0, 3)

	// Attach in-thread quotation (reply segment) if message_id exists
	if msg.MessageID != "" && msg.MessageID != "<nil>" && msg.MessageID != "0" {
		if mid, err := strconv.ParseInt(msg.MessageID, 10, 64); err == nil {
			segments = append(segments, map[string]any{"type": "reply", "data": map[string]any{"id": mid}})
		} else {
			segments = append(segments, map[string]any{"type": "reply", "data": map[string]any{"id": msg.MessageID}})
		}
	}

	if msg.Private {
		endpoint = "/send_private_msg"
		if uid, err := strconv.ParseInt(msg.UserID, 10, 64); err == nil {
			params["user_id"] = uid
		} else {
			params["user_id"] = msg.UserID
		}
		segments = append(segments, map[string]any{"type": "text", "data": map[string]any{"text": reply}})
	} else {
		endpoint = "/send_group_msg"
		if gid, err := strconv.ParseInt(msg.GroupID, 10, 64); err == nil {
			params["group_id"] = gid
		} else {
			params["group_id"] = msg.GroupID
		}
		// In group chat, include explicit @ mention to the requester
		if msg.UserID != "" && msg.UserID != "<nil>" {
			if uid, err := strconv.ParseInt(msg.UserID, 10, 64); err == nil {
				segments = append(segments, map[string]any{"type": "at", "data": map[string]any{"qq": uid}})
			} else {
				segments = append(segments, map[string]any{"type": "at", "data": map[string]any{"qq": msg.UserID}})
			}
			segments = append(segments, map[string]any{"type": "text", "data": map[string]any{"text": " " + reply}})
		} else {
			segments = append(segments, map[string]any{"type": "text", "data": map[string]any{"text": reply}})
		}
	}

	params["message"] = segments

	raw, err := client.Call(ctx, endpoint, params)
	if err != nil {
		return fmt.Errorf("napcat send: %w", err)
	}

	var sendResp struct {
		MessageID any `json:"message_id"`
		Data      struct {
			MessageID any `json:"message_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &sendResp); err == nil {
		mid := formatID(sendResp.MessageID)
		if mid == "" || mid == "0" || mid == "<nil>" {
			mid = formatID(sendResp.Data.MessageID)
		}
		s.markBotSent(mid, reply)
	} else {
		s.markBotSent("", reply)
	}

	_, _ = s.DB.Exec(ctx, `INSERT INTO raw_records(account_id, source, endpoint_or_event_type, payload, payload_hash)
		VALUES($1, 'napcat_http', 'bot_send_msg', $2, md5($2::text))`, accountID, string(raw))
	return nil
}
