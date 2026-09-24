package qzone

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/coder/websocket"
)

type Event struct {
	Raw         json.RawMessage
	EventID     string
	PostType    string
	MessageType string
	NoticeType  string
	Time        int64
}

type WSClient struct {
	URL      string
	Token    string
	Logger   *slog.Logger
	OnEvent  func(Event) error
	OnStatus func(string, error)
}

func (c *WSClient) Run(ctx context.Context) error {
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
	backoff := time.Second
	for {
		err := c.runOnce(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if c.OnStatus != nil {
			c.OnStatus("disconnected", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}

func (c *WSClient) runOnce(ctx context.Context) error {
	headers := http.Header{}
	if c.Token != "" {
		headers.Set("Authorization", "Bearer "+c.Token)
		headers.Set("X-Access-Token", c.Token)
	}
	conn, _, err := websocket.Dial(ctx, c.URL, &websocket.DialOptions{HTTPHeader: headers})
	if err != nil {
		return err
	}
	defer conn.Close(websocket.StatusNormalClosure, "")
	if c.OnStatus != nil {
		c.OnStatus("connected", nil)
	}
	for {
		_, raw, err := conn.Read(ctx)
		if err != nil {
			return err
		}
		for _, event := range parseEvents(raw) {
			if c.OnEvent != nil {
				if err := c.OnEvent(event); err != nil {
					c.Logger.Warn("QZone event handler failed", "error", err)
				}
			}
		}
	}
}

func parseEvents(raw []byte) []Event {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) > 0 && trimmed[0] == '[' {
		var items []json.RawMessage
		if json.Unmarshal(trimmed, &items) == nil {
			result := make([]Event, 0, len(items))
			for _, item := range items {
				result = append(result, parseEvent(item))
			}
			return result
		}
	}
	return []Event{parseEvent(trimmed)}
}

func parseEvent(raw []byte) Event {
	var value map[string]any
	_ = json.Unmarshal(raw, &value)
	event := Event{Raw: append([]byte(nil), raw...)}
	event.PostType = textValue(value["post_type"])
	event.MessageType = textValue(value["message_type"])
	event.NoticeType = textValue(value["notice_type"])
	event.EventID = firstText(value, "_stable_post_key", "_tid", "comment_id", "message_id", "self_id")
	event.Time = int64Value(value["time"])
	return event
}

func eventType(event Event) string {
	if event.NoticeType != "" {
		return event.NoticeType
	}
	if event.PostType != "" {
		return event.PostType
	}
	return "unknown"
}

func (c *WSClient) String() string { return fmt.Sprintf("QZoneWS(%s)", c.URL) }
