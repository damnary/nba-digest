-- +goose Up
ALTER TABLE games
    ADD COLUMN home_record text NOT NULL DEFAULT '',
    ADD COLUMN away_record text NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE games DROP COLUMN home_record, DROP COLUMN away_record;
