package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

const (
	defaultAPIBase  = "https://api.telegram.org"
	globalRateLimit = 25
	perChatRate     = 1
	maxRetryAfter   = 30 * time.Second
)

type Client struct {
	http    *http.Client
	base    string
	token   string
	global  *rate.Limiter
	now     func() time.Time
	sleep   func(context.Context, time.Duration) error
	mu      sync.Mutex
	perChat map[int64]*rate.Limiter
}

type ClientOption func(*Client)

func WithHTTPClient(c *http.Client) ClientOption {
	return func(cl *Client) { cl.http = c }
}

func WithBaseURL(base string) ClientOption {
	return func(cl *Client) { cl.base = base }
}

func NewClient(token string, opts ...ClientOption) *Client {
	c := &Client{
		http:    &http.Client{Timeout: 30 * time.Second},
		base:    defaultAPIBase,
		token:   token,
		global:  rate.NewLimiter(globalRateLimit, globalRateLimit),
		now:     time.Now,
		perChat: make(map[int64]*rate.Limiter),
	}
	c.sleep = func(ctx context.Context, d time.Duration) error {
		timer := time.NewTimer(d)
		defer timer.Stop()
		select {
		case <-timer.C:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

type response struct {
	OK          bool            `json:"ok"`
	Result      json.RawMessage `json:"result"`
	Description string          `json:"description"`
	ErrorCode   int             `json:"error_code"`
	Parameters  struct {
		RetryAfter int `json:"retry_after"`
	} `json:"parameters"`
}

type apiError struct {
	Method      string
	Code        int
	Description string
}

func (e *apiError) Error() string {
	return fmt.Sprintf("telegram %s: %d %s", e.Method, e.Code, e.Description)
}

type sendMessageRequest struct {
	ChatID      int64     `json:"chat_id"`
	Text        string    `json:"text"`
	ReplyMarkup *keyboard `json:"reply_markup,omitempty"`
}

type editMessageRequest struct {
	ChatID      int64     `json:"chat_id"`
	MessageID   int64     `json:"message_id"`
	Text        string    `json:"text"`
	ReplyMarkup *keyboard `json:"reply_markup,omitempty"`
}

func (c *Client) SendMessage(ctx context.Context, chatID int64, text string, kb *keyboard) error {
	if err := c.waitForChat(ctx, chatID); err != nil {
		return err
	}
	return c.call(ctx, "sendMessage", sendMessageRequest{ChatID: chatID, Text: text, ReplyMarkup: kb}, nil)
}

func (c *Client) EditMessage(ctx context.Context, chatID, messageID int64, text string, kb *keyboard) error {
	if err := c.waitForChat(ctx, chatID); err != nil {
		return err
	}
	req := editMessageRequest{ChatID: chatID, MessageID: messageID, Text: text, ReplyMarkup: kb}
	return c.call(ctx, "editMessageText", req, nil)
}

func (c *Client) AnswerCallback(ctx context.Context, id string) error {
	return c.call(ctx, "answerCallbackQuery", map[string]string{"callback_query_id": id}, nil)
}

func (c *Client) SetWebhook(ctx context.Context, url, secret string) error {
	body := map[string]any{
		"url":             url,
		"secret_token":    secret,
		"allowed_updates": []string{"message", "callback_query"},
	}
	return c.call(ctx, "setWebhook", body, nil)
}

func (c *Client) DeleteWebhook(ctx context.Context) error {
	return c.call(ctx, "deleteWebhook", map[string]any{}, nil)
}

func (c *Client) GetUpdates(ctx context.Context, offset int64, timeout time.Duration) ([]Update, error) {
	body := map[string]any{
		"offset":          offset,
		"timeout":         int(timeout.Seconds()),
		"allowed_updates": []string{"message", "callback_query"},
	}

	var updates []Update
	if err := c.call(ctx, "getUpdates", body, &updates); err != nil {
		return nil, err
	}
	return updates, nil
}

func (c *Client) call(ctx context.Context, method string, body any, out any) error {
	for attempt := range 3 {
		retryAfter, err := c.callOnce(ctx, method, body, out)
		if retryAfter <= 0 {
			return err
		}
		if attempt == 2 {
			return err
		}
		if retryAfter > maxRetryAfter {
			retryAfter = maxRetryAfter
		}
		if err := c.sleep(ctx, retryAfter); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) callOnce(ctx context.Context, method string, body, out any) (time.Duration, error) {
	if err := c.global.Wait(ctx); err != nil {
		return 0, err
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return 0, fmt.Errorf("encode %s: %w", method, err)
	}

	url := fmt.Sprintf("%s/bot%s/%s", c.base, c.token, method)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return 0, fmt.Errorf("build %s: %w", method, err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return 0, fmt.Errorf("call %s: %w", method, err)
	}
	defer resp.Body.Close()

	var parsed response
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return 0, fmt.Errorf("decode %s: %w", method, err)
	}

	if !parsed.OK {
		apiErr := &apiError{Method: method, Code: parsed.ErrorCode, Description: parsed.Description}
		if parsed.Parameters.RetryAfter > 0 {
			return time.Duration(parsed.Parameters.RetryAfter) * time.Second, apiErr
		}
		return 0, apiErr
	}

	if out != nil && len(parsed.Result) > 0 {
		if err := json.Unmarshal(parsed.Result, out); err != nil {
			return 0, fmt.Errorf("decode %s result: %w", method, err)
		}
	}
	return 0, nil
}

func (c *Client) waitForChat(ctx context.Context, chatID int64) error {
	c.mu.Lock()
	limiter, ok := c.perChat[chatID]
	if !ok {
		limiter = rate.NewLimiter(perChatRate, 1)
		c.perChat[chatID] = limiter
	}
	c.mu.Unlock()

	return limiter.Wait(ctx)
}
