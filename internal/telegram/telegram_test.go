package telegram

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/damnary/nba-digest/internal/core"
)

func quiet() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

type apiCall struct {
	Method string
	Body   map[string]any
}

type fakeAPI struct {
	*httptest.Server
	mu    sync.Mutex
	calls []apiCall
}

func newFakeAPI(t *testing.T) *fakeAPI {
	t.Helper()
	api := &fakeAPI{}

	api.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		method := parts[len(parts)-1]

		var body map[string]any
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)

		api.mu.Lock()
		api.calls = append(api.calls, apiCall{Method: method, Body: body})
		api.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{}}`))
	}))
	t.Cleanup(api.Close)
	return api
}

func (a *fakeAPI) snapshot() []apiCall {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]apiCall(nil), a.calls...)
}

func (a *fakeAPI) methods() []string {
	var out []string
	for _, c := range a.snapshot() {
		out = append(out, c.Method)
	}
	return out
}

func (a *fakeAPI) lastOf(method string) (apiCall, bool) {
	calls := a.snapshot()
	for i := len(calls) - 1; i >= 0; i-- {
		if calls[i].Method == method {
			return calls[i], true
		}
	}
	return apiCall{}, false
}

func newTestWebhook(t *testing.T, api *fakeAPI, handle core.CommandHandler) *Webhook {
	t.Helper()
	client := NewClient("test-token", WithBaseURL(api.URL))
	return NewWebhook(client, handle, "", "s3cret", WithWebhookLogger(quiet()))
}

func post(t *testing.T, w *Webhook, secret string, update any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(update)
	if err != nil {
		t.Fatalf("marshal update: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(string(raw)))
	if secret != "" {
		req.Header.Set(secretHeader, secret)
	}
	rec := httptest.NewRecorder()
	w.ServeHTTP(rec, req)
	return rec
}

func TestParseText(t *testing.T) {
	tests := []struct {
		text string
		want core.Command
	}{
		{"/start", core.Command{Kind: core.CommandStart}},
		{"/start@DamnaryNbaDigestBot", core.Command{Kind: core.CommandStart}},
		{"/teams", core.Command{Kind: core.CommandTeams}},
		{"/team", core.Command{Kind: core.CommandTeams}},
		{"/team nyl", core.Command{Kind: core.CommandToggleTeam, Team: "NYL"}},
		{"/team add NYL", core.Command{Kind: core.CommandToggleTeam, Team: "NYL"}},
		{"/alerts on", core.Command{Kind: core.CommandAlerts, Enable: true}},
		{"/alerts off", core.Command{Kind: core.CommandAlerts}},
		{"/alerts maybe", core.Command{Kind: core.CommandUnknown}},
		{"/alerts", core.Command{Kind: core.CommandUnknown}},
		{"/stop", core.Command{Kind: core.CommandStop}},
		{"/help", core.Command{Kind: core.CommandHelp}},
		{"привет", core.Command{Kind: core.CommandUnknown}},
		{"   ", core.Command{Kind: core.CommandUnknown}},
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			got := parseText(tt.text)
			if got != tt.want {
				t.Errorf("parseText(%q) = %+v, want %+v", tt.text, got, tt.want)
			}
		})
	}
}

func TestParseCallback(t *testing.T) {
	if got := parseCallback("t:NYL"); got.Kind != core.CommandToggleTeam || got.Team != "NYL" {
		t.Errorf("callback t:NYL = %+v", got)
	}
	if got := parseCallback("garbage"); got.Kind != core.CommandUnknown {
		t.Errorf("garbage callback = %+v", got)
	}
	if got := parseCallback("t:"); got.Kind != core.CommandUnknown {
		t.Errorf("empty team = %+v", got)
	}
}

func TestWebhookRejectsWrongSecret(t *testing.T) {
	api := newFakeAPI(t)
	var called bool
	hook := newTestWebhook(t, api, func(context.Context, core.Command) (core.Reply, error) {
		called = true
		return core.Reply{}, nil
	})

	update := Update{UpdateID: 1}
	update.Message = &struct {
		MessageID int64  `json:"message_id"`
		Text      string `json:"text"`
		Chat      struct {
			ID int64 `json:"id"`
		} `json:"chat"`
	}{MessageID: 10, Text: "/start"}
	update.Message.Chat.ID = 42

	if rec := post(t, hook, "wrong", update); rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
	if rec := post(t, hook, "", update); rec.Code != http.StatusUnauthorized {
		t.Errorf("missing secret: status = %d, want 401", rec.Code)
	}
	if called {
		t.Error("handler must not run without a valid secret")
	}
	if len(api.snapshot()) != 0 {
		t.Errorf("nothing should have been sent: %v", api.methods())
	}
}

func TestWebhookAnswersATextCommand(t *testing.T) {
	api := newFakeAPI(t)

	var got core.Command
	hook := newTestWebhook(t, api, func(_ context.Context, cmd core.Command) (core.Reply, error) {
		got = cmd
		return core.Reply{
			Kind: core.ReplyWelcome,
			Teams: []core.TeamOption{
				{Team: core.Team{Code: "NYL", Name: "New York Liberty"}, Selected: true},
				{Team: core.Team{Code: "LVA", Name: "Las Vegas Aces"}},
			},
		}, nil
	})

	update := Update{UpdateID: 1}
	update.Message = &struct {
		MessageID int64  `json:"message_id"`
		Text      string `json:"text"`
		Chat      struct {
			ID int64 `json:"id"`
		} `json:"chat"`
	}{MessageID: 10, Text: "/start"}
	update.Message.Chat.ID = 42

	if rec := post(t, hook, "s3cret", update); rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}

	if got.Kind != core.CommandStart || got.ChatID != 42 {
		t.Errorf("handler received %+v", got)
	}

	call, ok := api.lastOf("sendMessage")
	if !ok {
		t.Fatalf("sendMessage was not called: %v", api.methods())
	}
	if call.Body["chat_id"].(float64) != 42 {
		t.Errorf("chat_id = %v", call.Body["chat_id"])
	}

	markup, ok := call.Body["reply_markup"].(map[string]any)
	if !ok {
		t.Fatalf("no keyboard in the reply: %+v", call.Body)
	}
	rows := markup["inline_keyboard"].([]any)
	if len(rows) != 1 {
		t.Fatalf("want a single row of two teams, got %d rows", len(rows))
	}
	first := rows[0].([]any)[0].(map[string]any)
	if first["text"] != "✓ New York Liberty" {
		t.Errorf("selected team should be ticked, got %q", first["text"])
	}
	if first["callback_data"] != "t:NYL" {
		t.Errorf("callback_data = %q", first["callback_data"])
	}
}

func TestWebhookEditsOnCallback(t *testing.T) {
	api := newFakeAPI(t)

	hook := newTestWebhook(t, api, func(_ context.Context, cmd core.Command) (core.Reply, error) {
		if cmd.Kind != core.CommandToggleTeam || cmd.Team != "SEA" {
			t.Errorf("handler received %+v", cmd)
		}
		return core.Reply{Kind: core.ReplyTeamAdded, Team: core.Team{Name: "Seattle Storm"}}, nil
	})

	update := Update{UpdateID: 2}
	update.CallbackQuery = &struct {
		ID      string `json:"id"`
		Data    string `json:"data"`
		Message *struct {
			MessageID int64 `json:"message_id"`
			Chat      struct {
				ID int64 `json:"id"`
			} `json:"chat"`
		} `json:"message"`
	}{ID: "cb-1", Data: "t:SEA"}
	update.CallbackQuery.Message = &struct {
		MessageID int64 `json:"message_id"`
		Chat      struct {
			ID int64 `json:"id"`
		} `json:"chat"`
	}{MessageID: 77}
	update.CallbackQuery.Message.Chat.ID = 42

	if rec := post(t, hook, "s3cret", update); rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}

	methods := api.methods()
	if len(methods) < 2 || methods[0] != "answerCallbackQuery" {
		t.Fatalf("callback must be acknowledged first, got %v", methods)
	}

	call, ok := api.lastOf("editMessageText")
	if !ok {
		t.Fatalf("a callback should edit the message, got %v", methods)
	}
	if call.Body["message_id"].(float64) != 77 {
		t.Errorf("message_id = %v", call.Body["message_id"])
	}
	if _, sent := api.lastOf("sendMessage"); sent {
		t.Error("a new message must not be sent when editing succeeded")
	}
}

func TestWebhookIgnoresEmptyUpdate(t *testing.T) {
	api := newFakeAPI(t)
	hook := newTestWebhook(t, api, func(context.Context, core.Command) (core.Reply, error) {
		t.Error("handler must not be called for an empty update")
		return core.Reply{}, nil
	})

	if rec := post(t, hook, "s3cret", Update{UpdateID: 3}); rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestPerChatRateLimit(t *testing.T) {
	api := newFakeAPI(t)
	client := NewClient("test-token", WithBaseURL(api.URL))

	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()

	if err := client.SendMessage(ctx, 1, "first", nil); err != nil {
		t.Fatalf("first send: %v", err)
	}

	err := client.SendMessage(ctx, 1, "second", nil)
	if err == nil {
		t.Error("a second message to the same chat should have been held back")
	}

	if err := client.SendMessage(t.Context(), 2, "other chat", nil); err != nil {
		t.Errorf("a different chat must not be blocked: %v", err)
	}
}

func TestRetryAfterIsHonoured(t *testing.T) {
	var attempts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		w.Header().Set("Content-Type", "application/json")
		if attempts == 1 {
			_, _ = w.Write([]byte(`{"ok":false,"error_code":429,"description":"Too Many Requests","parameters":{"retry_after":7}}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":{}}`))
	}))
	defer srv.Close()

	client := NewClient("test-token", WithBaseURL(srv.URL))

	var slept time.Duration
	client.sleep = func(_ context.Context, d time.Duration) error {
		slept = d
		return nil
	}

	if err := client.SendMessage(t.Context(), 1, "hello", nil); err != nil {
		t.Fatalf("send: %v", err)
	}
	if attempts != 2 {
		t.Errorf("want 2 attempts, got %d", attempts)
	}
	if slept != 7*time.Second {
		t.Errorf("slept %v, want the 7s the server asked for", slept)
	}
}

func TestPollingSurvivesANetworkTimeout(t *testing.T) {
	var updateCalls atomic.Int32
	dispatched := make(chan int64, 1)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		isUpdates := strings.HasSuffix(r.URL.Path, "/getUpdates")
		if isUpdates && updateCalls.Add(1) == 1 {
			time.Sleep(300 * time.Millisecond)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if isUpdates {
			_, _ = w.Write([]byte(`{"ok":true,"result":[{"update_id":7,"message":{"message_id":1,"text":"/start","chat":{"id":42}}}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":{}}`))
	}))
	defer srv.Close()

	client := NewClient("test-token",
		WithBaseURL(srv.URL),
		WithHTTPClient(&http.Client{Timeout: 80 * time.Millisecond}))

	poller := NewPoller(client, func(_ context.Context, cmd core.Command) (core.Reply, error) {
		select {
		case dispatched <- cmd.ChatID:
		default:
		}
		return core.Reply{Kind: core.ReplyHelp}, nil
	}, WithPollerLogger(quiet()))

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- poller.Run(ctx) }()

	select {
	case chat := <-dispatched:
		if chat != 42 {
			t.Errorf("dispatched chat %d, want 42", chat)
		}
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("polling stopped after a network timeout instead of retrying")
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run returned %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after cancel")
	}
}

func TestErrorsNeverLeakTheToken(t *testing.T) {
	const token = "8715712861:AA-super-secret-value"

	client := NewClient(token,
		WithBaseURL("http://127.0.0.1:1"),
		WithHTTPClient(&http.Client{Timeout: time.Second}))

	err := client.SendMessage(t.Context(), 42, "hello", nil)
	if err == nil {
		t.Fatal("want a transport error")
	}
	if strings.Contains(err.Error(), token) {
		t.Fatalf("token leaked into the error: %v", err)
	}
	if !strings.Contains(err.Error(), "REDACTED") {
		t.Errorf("the url should be redacted, got: %v", err)
	}
}

func TestSplitMessageKeepsParagraphsIntact(t *testing.T) {
	short := "короткое сообщение"
	if got := splitMessage(short); len(got) != 1 || got[0] != short {
		t.Fatalf("short text must stay whole, got %d parts", len(got))
	}

	paragraph := "<b>Матч</b>\n" + strings.Repeat("строка со счётом и фамилиями игроков\n", 20)
	paragraph = strings.TrimRight(paragraph, "\n")
	var paragraphs []string
	for range 12 {
		paragraphs = append(paragraphs, paragraph)
	}
	long := strings.Join(paragraphs, "\n\n")

	parts := splitMessage(long)
	if len(parts) < 2 {
		t.Fatalf("a %d-byte digest must be split, got %d part(s)", len(long), len(parts))
	}
	for i, part := range parts {
		if len(part) > maxMessageBytes {
			t.Errorf("part %d is %d bytes, over the limit", i, len(part))
		}
		if strings.Count(part, "<b>") != strings.Count(part, "</b>") {
			t.Errorf("part %d has unbalanced tags", i)
		}
	}
	if strings.Join(parts, "\n\n") != long {
		t.Error("splitting lost content")
	}
}
