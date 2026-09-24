package napcat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
)

type WSEvent struct {
	Raw         json.RawMessage
	EventID     string
	PostType    string
	MessageType string
	Time        int64
	Sequence    int64
}
type WSClient struct {
	URL      string
	Token    string
	Logger   *slog.Logger
	OnEvent  func(WSEvent) error
	OnStatus func(string, error)
	mu       sync.Mutex
}

func (c *WSClient) Run(ctx context.Context) error {
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
	backoff := time.Second
	for {
		if err := c.runOnce(ctx); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if c.OnStatus != nil {
				c.OnStatus("disconnected", err)
			}
			c.Logger.Warn("NapCat websocket disconnected", "error", err)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
			if backoff < 30*time.Second {
				backoff *= 2
			}
			continue
		}
		backoff = time.Second
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
}
func (c *WSClient) runOnce(ctx context.Context) error {
	h := http.Header{}
	if c.Token != "" {
		h.Set("Authorization", "Bearer "+c.Token)
		h.Set("X-Token", c.Token)
	}
	conn, _, err := websocket.Dial(ctx, c.URL, &websocket.DialOptions{HTTPHeader: h})
	if err != nil {
		return err
	}
	defer conn.Close(websocket.StatusNormalClosure, "")
	if c.OnStatus != nil {
		c.OnStatus("connected", nil)
	}
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return err
		}
		for _, event := range parseEvents(data) {
			if c.OnEvent != nil {
				if err := c.OnEvent(event); err != nil {
					c.Logger.Warn("NapCat event handler failed", "error", err)
				}
			}
		}
	}
}
func parseEvents(raw []byte) []WSEvent {
	var batch []json.RawMessage
	if len(bytes.TrimSpace(raw)) > 0 && bytes.TrimSpace(raw)[0] == '[' {
		if err := json.Unmarshal(raw, &batch); err == nil {
			result := make([]WSEvent, 0, len(batch))
			for _, item := range batch {
				result = append(result, parseEvent(item))
			}
			return result
		}
	}
	return []WSEvent{parseEvent(raw)}
}

func parseEvent(raw []byte) WSEvent {
	var v map[string]any
	_ = json.Unmarshal(raw, &v)
	e := WSEvent{Raw: append([]byte(nil), raw...)}
	if s, ok := v["post_type"].(string); ok {
		e.PostType = s
	}
	if s, ok := v["message_type"].(string); ok {
		e.MessageType = s
	}
	if value, ok := v["self_id"]; ok {
		e.EventID = fmt.Sprint(value)
	}
	if value, ok := v["message_id"]; ok && e.EventID == "" {
		e.EventID = fmt.Sprint(value)
	}
	if n, ok := v["time"].(float64); ok {
		e.Time = int64(n)
	}
	if n, ok := v["message_seq"].(float64); ok {
		e.Sequence = int64(n)
	}
	return e
}
func (c *WSClient) String() string { return fmt.Sprintf("NapCatWS(%s)", strings.TrimSpace(c.URL)) }
