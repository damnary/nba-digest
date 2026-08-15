package telegram

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/damnary/nba-digest/internal/core"
)

const secretHeader = "X-Telegram-Bot-Api-Secret-Token"

type Webhook struct {
	client  *Client
	handle  core.CommandHandler
	secret  string
	url     string
	log     *slog.Logger
	replies *replier
}

type WebhookOption func(*Webhook)

func WithWebhookLogger(l *slog.Logger) WebhookOption {
	return func(w *Webhook) {
		w.log = l
		w.replies.log = l
	}
}

func NewWebhook(client *Client, handle core.CommandHandler, url, secret string, opts ...WebhookOption) *Webhook {
	w := &Webhook{
		client:  client,
		handle:  handle,
		secret:  secret,
		url:     url,
		log:     slog.Default(),
		replies: &replier{client: client, log: slog.Default()},
	}
	for _, opt := range opts {
		opt(w)
	}
	return w
}

func (w *Webhook) Run(ctx context.Context) error {
	if w.url != "" {
		if err := w.client.SetWebhook(ctx, w.url, w.secret); err != nil {
			return err
		}
		w.log.Info("webhook registered", "url", w.url)
	}

	<-ctx.Done()
	return nil
}

func (w *Webhook) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		rw.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	got := r.Header.Get(secretHeader)
	if subtle.ConstantTimeCompare([]byte(got), []byte(w.secret)) != 1 {
		w.log.Warn("webhook called with a wrong secret", "remote", r.RemoteAddr)
		rw.WriteHeader(http.StatusUnauthorized)
		return
	}

	var update Update
	if err := json.NewDecoder(http.MaxBytesReader(rw, r.Body, 1<<20)).Decode(&update); err != nil {
		w.log.Warn("malformed update", "err", err)
		rw.WriteHeader(http.StatusBadRequest)
		return
	}

	w.replies.dispatch(r.Context(), w.handle, update)
	rw.WriteHeader(http.StatusOK)
}

type replier struct {
	client *Client
	log    *slog.Logger
}

func (r *replier) dispatch(ctx context.Context, handle core.CommandHandler, update Update) {
	in := parseUpdate(update)
	if !in.valid {
		return
	}

	if in.callbackID != "" {
		if err := r.client.AnswerCallback(ctx, in.callbackID); err != nil {
			r.log.Warn("answer callback failed", "err", err)
		}
	}

	reply, err := handle(ctx, in.command)
	if err != nil {
		r.log.Error("command failed", "kind", in.command.Kind, "chat", in.command.ChatID, "err", err)
		return
	}

	text, kb := render(reply)

	if in.edit {
		if err := r.client.EditMessage(ctx, in.command.ChatID, in.messageID, text, kb); err == nil {
			return
		}
		r.log.Warn("edit failed, sending a new message", "chat", in.command.ChatID)
	}

	if err := r.client.SendMessage(ctx, in.command.ChatID, text, kb); err != nil {
		r.log.Error("send failed", "chat", in.command.ChatID, "err", err)
	}
}
