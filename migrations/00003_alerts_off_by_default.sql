-- +goose Up
ALTER TABLE subscribers ALTER COLUMN alerts_on SET DEFAULT false;

UPDATE subscribers SET alerts_on = false;

-- +goose Down
ALTER TABLE subscribers ALTER COLUMN alerts_on SET DEFAULT true;
