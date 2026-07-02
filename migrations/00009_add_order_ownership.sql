-- +goose Up
-- Preserve pre-authentication orders under a disabled system account. New deployments
-- with no existing orders do not create this account.
INSERT INTO users (username, password_hash, nickname, status)
SELECT '__legacy_orders__', 'LOGIN_DISABLED', 'Legacy Orders', 2
FROM DUAL
WHERE EXISTS (SELECT 1 FROM orders)
   OR EXISTS (SELECT 1 FROM order_idempotency_keys)
ON DUPLICATE KEY UPDATE id = LAST_INSERT_ID(id);

SET @legacy_orders_user_id = (
    SELECT id FROM users WHERE username = '__legacy_orders__' LIMIT 1
);

ALTER TABLE orders
    ADD COLUMN user_id BIGINT NULL AFTER id;

UPDATE orders
SET user_id = @legacy_orders_user_id
WHERE user_id IS NULL;

ALTER TABLE orders
    MODIFY COLUMN user_id BIGINT NOT NULL,
    ADD INDEX idx_orders_user_id_created_at (user_id, created_at),
    ADD CONSTRAINT fk_orders_user FOREIGN KEY (user_id) REFERENCES users(id);

ALTER TABLE order_idempotency_keys
    DROP INDEX uk_idempotency_key,
    ADD COLUMN user_id BIGINT NULL AFTER id;

UPDATE order_idempotency_keys AS idempotency
LEFT JOIN orders AS owned_order ON owned_order.id = idempotency.order_id
SET idempotency.user_id = COALESCE(owned_order.user_id, @legacy_orders_user_id)
WHERE idempotency.user_id IS NULL;

ALTER TABLE order_idempotency_keys
    MODIFY COLUMN user_id BIGINT NOT NULL,
    ADD UNIQUE INDEX uk_user_id_idempotency_key (user_id, idempotency_key),
    ADD CONSTRAINT fk_order_idempotency_keys_user FOREIGN KEY (user_id) REFERENCES users(id);

-- +goose Down
ALTER TABLE order_idempotency_keys
    DROP FOREIGN KEY fk_order_idempotency_keys_user,
    DROP INDEX uk_user_id_idempotency_key,
    DROP COLUMN user_id,
    ADD UNIQUE INDEX uk_idempotency_key (idempotency_key);

ALTER TABLE orders
    DROP FOREIGN KEY fk_orders_user,
    DROP INDEX idx_orders_user_id_created_at,
    DROP COLUMN user_id;

DELETE FROM users WHERE username = '__legacy_orders__';
