-- +goose Up
CREATE TABLE teams (
    league text NOT NULL,
    code   text NOT NULL,
    name   text NOT NULL,
    PRIMARY KEY (league, code)
);

CREATE TABLE team_external_ids (
    provider    text NOT NULL,
    external_id text NOT NULL,
    league      text NOT NULL,
    code        text NOT NULL,
    PRIMARY KEY (provider, external_id),
    FOREIGN KEY (league, code) REFERENCES teams (league, code) ON DELETE CASCADE
);

CREATE TABLE subscribers (
    id         bigserial PRIMARY KEY,
    chat_id    bigint NOT NULL UNIQUE,
    timezone   text NOT NULL DEFAULT 'Europe/Moscow',
    digest_at  time NOT NULL DEFAULT '08:00',
    alerts_on  boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE subscriptions (
    subscriber_id bigint NOT NULL REFERENCES subscribers (id) ON DELETE CASCADE,
    league        text NOT NULL,
    team_code     text NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (subscriber_id, league, team_code),
    FOREIGN KEY (league, team_code) REFERENCES teams (league, code)
);

CREATE INDEX subscriptions_team_idx ON subscriptions (league, team_code);

CREATE TABLE games (
    id            text PRIMARY KEY,
    league        text NOT NULL,
    starts_at     timestamptz NOT NULL,
    status        text NOT NULL CHECK (status IN ('scheduled', 'live', 'final', 'postponed')),
    home_code     text NOT NULL,
    away_code     text NOT NULL,
    home_score    int NOT NULL DEFAULT 0,
    away_score    int NOT NULL DEFAULT 0,
    period        int NOT NULL DEFAULT 0,
    clock         text NOT NULL DEFAULT '',
    clutch_margin int,
    stats_at      timestamptz,
    observed_at   timestamptz NOT NULL,
    FOREIGN KEY (league, home_code) REFERENCES teams (league, code),
    FOREIGN KEY (league, away_code) REFERENCES teams (league, code)
);

CREATE INDEX games_league_starts_at_idx ON games (league, starts_at);
CREATE INDEX games_active_idx ON games (starts_at) WHERE status IN ('scheduled', 'live');

CREATE TABLE game_cursors (
    game_id      text PRIMARY KEY REFERENCES games (id) ON DELETE CASCADE,
    provider     text NOT NULL,
    cursor_token text NOT NULL,
    updated_at   timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE game_events (
    id          text PRIMARY KEY,
    game_id     text NOT NULL REFERENCES games (id) ON DELETE CASCADE,
    league      text NOT NULL,
    kind        text NOT NULL,
    teams       text[] NOT NULL DEFAULT '{}',
    period      int NOT NULL,
    clock       text NOT NULL DEFAULT '',
    home_score  int NOT NULL,
    away_score  int NOT NULL,
    run_team    text NOT NULL DEFAULT '',
    run_points  int NOT NULL DEFAULT 0,
    run_against int NOT NULL DEFAULT 0,
    occurred_at timestamptz NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX game_events_game_idx ON game_events (game_id);
CREATE INDEX game_events_created_idx ON game_events (created_at);

CREATE TABLE game_player_stats (
    game_id       text NOT NULL REFERENCES games (id) ON DELETE CASCADE,
    player_id     text NOT NULL,
    player_name   text NOT NULL,
    team_code     text NOT NULL,
    points        int NOT NULL DEFAULT 0,
    rebounds      int NOT NULL DEFAULT 0,
    assists       int NOT NULL DEFAULT 0,
    clutch_points int NOT NULL DEFAULT 0,
    PRIMARY KEY (game_id, player_id)
);

CREATE TABLE alert_deliveries (
    subscriber_id bigint NOT NULL REFERENCES subscribers (id) ON DELETE CASCADE,
    event_id      text NOT NULL REFERENCES game_events (id) ON DELETE CASCADE,
    status        text NOT NULL CHECK (status IN ('pending', 'sent', 'skipped', 'failed')),
    attempts      int NOT NULL DEFAULT 0,
    sent_at       timestamptz,
    created_at    timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (subscriber_id, event_id)
);

CREATE INDEX alert_deliveries_pending_idx ON alert_deliveries (created_at) WHERE status = 'pending';

CREATE TABLE digest_runs (
    subscriber_id bigint NOT NULL REFERENCES subscribers (id) ON DELETE CASCADE,
    day           date NOT NULL,
    sent          boolean NOT NULL,
    processed_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (subscriber_id, day)
);

-- +goose Down
DROP TABLE digest_runs;
DROP TABLE alert_deliveries;
DROP TABLE game_player_stats;
DROP TABLE game_events;
DROP TABLE game_cursors;
DROP TABLE games;
DROP TABLE subscriptions;
DROP TABLE subscribers;
DROP TABLE team_external_ids;
DROP TABLE teams;
