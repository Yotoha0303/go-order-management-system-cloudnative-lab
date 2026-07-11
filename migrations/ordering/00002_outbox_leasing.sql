-- +goose Up
ALTER TABLE order_timeout_outbox_v2
    ADD COLUMN lease_owner VARCHAR(128) NOT NULL DEFAULT '' AFTER last_error,
    ADD COLUMN lease_until DATETIME(3) NULL AFTER lease_owner,
    ADD COLUMN next_attempt_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) AFTER lease_until,
    ADD KEY idx_order_timeout_outbox_claim (status, next_attempt_at, lease_until);

-- +goose Down
ALTER TABLE order_timeout_outbox_v2
    DROP KEY idx_order_timeout_outbox_claim,
    DROP COLUMN next_attempt_at,
    DROP COLUMN lease_until,
    DROP COLUMN lease_owner;
