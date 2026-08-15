package config

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/damnary/nba-digest/internal/core"
)

type Config struct {
	DatabaseURL   string
	TelegramToken string
	WebhookURL    string
	WebhookSecret string
	HTTPAddr      string
	League        core.League
	ReplayDir     string
	ReplaySpeed   float64
	DefaultTZ     string
	DigestAt      core.DailyTime
	ShutdownGrace time.Duration
	LogLevel      string
}

func (c Config) UsesWebhook() bool { return c.WebhookURL != "" }

func (c Config) UsesReplay() bool { return c.ReplayDir != "" }

func Load() (Config, error) {
	loadDotEnv(".env")

	cfg := Config{
		DatabaseURL:   os.Getenv("DATABASE_URL"),
		TelegramToken: os.Getenv("TELEGRAM_BOT_TOKEN"),
		WebhookURL:    os.Getenv("TELEGRAM_WEBHOOK_URL"),
		WebhookSecret: os.Getenv("TELEGRAM_WEBHOOK_SECRET"),
		HTTPAddr:      valueOr("HTTP_ADDR", ":8080"),
		ReplayDir:     os.Getenv("REPLAY_DIR"),
		DefaultTZ:     valueOr("DEFAULT_TIMEZONE", "Europe/Moscow"),
		ShutdownGrace: 30 * time.Second,
		LogLevel:      valueOr("LOG_LEVEL", "info"),
	}

	var problems []string

	league, err := core.ParseLeague(valueOr("LEAGUE", string(core.LeagueWNBA)))
	if err != nil {
		problems = append(problems, err.Error())
	}
	cfg.League = league

	digestAt, err := core.ParseDailyTime(valueOr("DIGEST_AT", "08:00"))
	if err != nil {
		problems = append(problems, err.Error())
	}
	cfg.DigestAt = digestAt

	speed, err := floatOr("REPLAY_SPEED", 60)
	if err != nil {
		problems = append(problems, err.Error())
	}
	cfg.ReplaySpeed = speed

	if _, err := time.LoadLocation(cfg.DefaultTZ); err != nil {
		problems = append(problems, fmt.Sprintf("DEFAULT_TIMEZONE %q is not a known IANA name", cfg.DefaultTZ))
	}

	if cfg.DatabaseURL == "" {
		problems = append(problems, "DATABASE_URL is not set")
	}
	if cfg.TelegramToken == "" {
		problems = append(problems, "TELEGRAM_BOT_TOKEN is not set")
	}
	if cfg.UsesWebhook() {
		if !strings.HasPrefix(cfg.WebhookURL, "https://") {
			problems = append(problems, "TELEGRAM_WEBHOOK_URL must be an https address")
		}
		if len(cfg.WebhookSecret) < 16 {
			problems = append(problems, "TELEGRAM_WEBHOOK_SECRET must be at least 16 characters")
		}
	}

	if len(problems) > 0 {
		return Config{}, fmt.Errorf("configuration is incomplete:\n  - %s", strings.Join(problems, "\n  - "))
	}
	return cfg, nil
}

func valueOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func floatOr(key string, fallback float64) (float64, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}
	var v float64
	if _, err := fmt.Sscanf(raw, "%g", &v); err != nil || v <= 0 {
		return 0, fmt.Errorf("%s must be a positive number, got %q", key, raw)
	}
	return v, nil
}

func loadDotEnv(path string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)
		_ = os.Setenv(key, value)
	}
}
