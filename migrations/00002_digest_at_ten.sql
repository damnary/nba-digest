-- +goose Up
ALTER TABLE subscribers ALTER COLUMN digest_at SET DEFAULT '10:00';

UPDATE subscribers SET digest_at = '10:00' WHERE digest_at = '08:00';

-- +goose Down
ALTER TABLE subscribers ALTER COLUMN digest_at SET DEFAULT '08:00';

UPDATE subscribers SET digest_at = '08:00' WHERE digest_at = '10:00';
