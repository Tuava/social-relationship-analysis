package napcat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type HTTPClient struct {
	BaseURL string
	Token   string
	Client  *http.Client
}
type APIResponse struct {
	Status  string          `json:"status"`
	Retcode int             `json:"retcode"`
	Data    json.RawMessage `json:"data"`
	Message string          `json:"message"`
	Wording string          `json:"wording"`
}

func (c *HTTPClient) GetLoginInfo(ctx context.Context) (json.RawMessage, error) {
	return c.Call(ctx, "/get_login_info", map[string]any{})
}
func (c *HTTPClient) GetGroupList(ctx context.Context) (json.RawMessage, error) {
	return c.Call(ctx, "/get_group_list", map[string]any{"no_cache": true})
}
func (c *HTTPClient) GetFriendList(ctx context.Context) (json.RawMessage, error) {
	return c.Call(ctx, "/get_friend_list", map[string]any{"no_cache": true})
}
func (c *HTTPClient) GetStrangerInfo(ctx context.Context, userID any) (json.RawMessage, error) {
	return c.Call(ctx, "/get_stranger_info", map[string]any{"user_id": userID, "no_cache": true})
}
func (c *HTTPClient) GetGroupMemberList(ctx context.Context, groupID any) (json.RawMessage, error) {
	return c.Call(ctx, "/get_group_member_list", map[string]any{"group_id": groupID, "no_cache": true})
}
func (c *HTTPClient) GetGroupHistory(ctx context.Context, groupID any, count int, seq any) (json.RawMessage, error) {
	return c.Call(ctx, "/get_group_msg_history", historyParams("group_id", groupID, count, seq))
}

func NewHTTPClient(baseURL, token string) *HTTPClient {
	return &HTTPClient{BaseURL: strings.TrimRight(baseURL, "/"), Token: token, Client: &http.Client{Timeout: 30 * time.Second}}
}

func (c *HTTPClient) Call(ctx context.Context, endpoint string, params any) (json.RawMessage, error) {
	body, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("marshal API params: %w", err)
	}
	url := c.BaseURL + "/" + strings.TrimLeft(endpoint, "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	res, err := c.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 32<<20))
	if err != nil {
		return nil, err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("NapCat HTTP %s: %s", res.Status, string(raw))
	}
	var parsed APIResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("decode NapCat response: %w", err)
	}
	if parsed.Retcode != 0 && parsed.Status != "ok" {
		if strings.Contains(parsed.Message, `"result": 0`) || strings.Contains(parsed.Message, `"result":0`) {
			return json.RawMessage(`{"result":0}`), nil
		}
		return nil, fmt.Errorf("NapCat API %d: %s", parsed.Retcode, parsed.Message)
	}
	if parsed.Data == nil {
		return json.RawMessage("null"), nil
	}
	return parsed.Data, nil
}

func (c *HTTPClient) GetFriendMsgHistory(ctx context.Context, userID any, count int, seq any) (json.RawMessage, error) {
	return c.Call(ctx, "/get_friend_msg_history", historyParams("user_id", userID, count, seq))
}

func historyParams(identifierKey string, identifier any, count int, seq any) map[string]any {
	params := map[string]any{
		identifierKey:     fmt.Sprint(identifier),
		"count":           count,
		"reverse_order":   false,
		"disable_get_url": false,
		"parse_mult_msg":  true,
		"quick_reply":     false,
		"reverseOrder":    false,
	}
	if seq != nil && fmt.Sprint(seq) != "" {
		params["message_seq"] = fmt.Sprint(seq)
		// NapCat's short message ID is an anchor, not an ordered sequence.
		// With an anchor, reverse order walks toward older messages.
		params["reverse_order"] = true
		params["reverseOrder"] = true
	}
	return params
}

// GetGroupFiles fetches the root file listing for a group (files + folders).
func (c *HTTPClient) GetGroupFiles(ctx context.Context, groupID any) (json.RawMessage, error) {
	return c.Call(ctx, "/get_group_root_files", map[string]any{"group_id": groupID})
}

// GetGroupAlbums fetches the album list for a group. NapCat exposes this as the
// "get_qun_album_list" action (群相册列表).
func (c *HTTPClient) GetGroupAlbums(ctx context.Context, groupID any) (json.RawMessage, error) {
	return c.Call(ctx, "/get_qun_album_list", map[string]any{"group_id": groupID})
}

// GetGroupNotices fetches the announcement list for a group. The NapCat action
// is "_get_group_notice" (群公告).
func (c *HTTPClient) GetGroupNotices(ctx context.Context, groupID any) (json.RawMessage, error) {
	return c.Call(ctx, "/_get_group_notice", map[string]any{"group_id": groupID})
}

// GetGroupHonor fetches honor info for a group. honorType is one of:
// talkative, performer, legend, strong_newbie, emotion, all.
func (c *HTTPClient) GetGroupHonor(ctx context.Context, groupID any, honorType string) (json.RawMessage, error) {
	if honorType == "" {
		honorType = "all"
	}
	return c.Call(ctx, "/get_group_honor_info", map[string]any{"group_id": groupID, "type": honorType})
}

// GetRecentContacts fetches the most recent conversations for the logged-in account.
func (c *HTTPClient) GetRecentContacts(ctx context.Context) (json.RawMessage, error) {
	return c.Call(ctx, "/get_recent_contact", map[string]any{"count": 50})
}

// GetGroupMemberInfo fetches detail for a single group member (level, join_time,
// last_active_time, title, etc.).
func (c *HTTPClient) GetGroupMemberInfo(ctx context.Context, groupID any, userID any) (json.RawMessage, error) {
	return c.Call(ctx, "/get_group_member_info", map[string]any{"group_id": groupID, "user_id": userID, "no_cache": true})
}
