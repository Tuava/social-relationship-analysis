package qzone

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

type apiResponse struct {
	Status  string          `json:"status"`
	Retcode int             `json:"retcode"`
	Data    json.RawMessage `json:"data"`
	Message string          `json:"message"`
	Wording string          `json:"wording"`
}

type HTTPStatusError struct {
	StatusCode int
	Status     string
	Body       string
}

func (e *HTTPStatusError) Error() string {
	return fmt.Sprintf("QZone HTTP %s: %s", e.Status, e.Body)
}

func (e *HTTPStatusError) Temporary() bool {
	return e.StatusCode == http.StatusTooManyRequests || e.StatusCode >= 500
}

type APIError struct {
	Retcode int
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("QZone API %d: %s", e.Retcode, e.Message)
}

func (e *APIError) Temporary() bool {
	message := strings.ToLower(e.Message)
	// 1401 can mean "network busy" (rate limit, retryable) or "need login"
	// (auth expired, NOT retryable). Only retry on the rate-limit variant.
	if strings.Contains(message, "network busy") || strings.Contains(message, "网络繁忙") {
		return true
	}
	// "need login" / "需要登录" means the bridge token expired — not retryable.
	if strings.Contains(message, "need login") || strings.Contains(message, "需要登录") {
		return false
	}
	switch e.Retcode {
	case 1429, 1500:
		return true
	}
	return false
}

func NewHTTPClient(baseURL, token string) *HTTPClient {
	return &HTTPClient{BaseURL: strings.TrimRight(baseURL, "/"), Token: token, Client: &http.Client{Timeout: 45 * time.Second}}
}

func (c *HTTPClient) Call(ctx context.Context, action string, params any) (json.RawMessage, error) {
	body, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("marshal QZone params: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/"+strings.TrimLeft(action, "/"), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
		req.Header.Set("X-Access-Token", c.Token)
	}
	res, err := c.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 64<<20))
	if err != nil {
		return nil, err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, &HTTPStatusError{StatusCode: res.StatusCode, Status: res.Status, Body: strings.TrimSpace(string(raw))}
	}
	var parsed apiResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("decode QZone response: %w", err)
	}
	if parsed.Retcode != 0 || (parsed.Status != "" && parsed.Status != "ok") {
		message := parsed.Message
		if message == "" {
			message = parsed.Wording
		}
		return nil, &APIError{Retcode: parsed.Retcode, Message: message}
	}
	if parsed.Data == nil {
		return json.RawMessage("null"), nil
	}
	return parsed.Data, nil
}

// ProbeStatus sends a quick check to ensure the QZone bridge is responsive and online.
func (c *HTTPClient) ProbeStatus(ctx context.Context) (bool, error) {
	reqCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	raw, err := c.Call(reqCtx, "get_status", map[string]any{})
	if err != nil {
		return false, err
	}
	var res struct {
		Online bool `json:"online"`
		Good   bool `json:"good"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return true, nil
	}
	return res.Online || res.Good, nil
}
