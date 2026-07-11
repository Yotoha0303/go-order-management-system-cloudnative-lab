-- +goose Up
CREATE TABLE IF NOT EXISTS orders_v2 (
    id BIGINT NOT NULL AUTO_INCREMENT,
    user_id BIGINT NOT NULL,
    status VARCHAR(32) NOT NULL,
    total_fen BIGINT NOT NULL,
    reservation_id VARCHAR(36) NOT NULL DEFAULT '',
    idempotency_key VARCHAR(64) NOT NULL,
    failure_reason VARCHAR(500) NOT NULL DEFAULT '',
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    UNIQUE KEY uk_order_user_idempotency (user_id, idempotency_key),
    KEY idx_orders_v2_user_id (user_id),
    KEY idx_orders_v2_status (status),
    KEY idx_orders_v2_reservation_id (reservation_id),
    CONSTRAINT chk_orders_v2_total CHECK (total_fen >= 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

CREATE TABLE IF NOT EXISTS order_items_v2 (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    order_id BIGINT NOT NULL,
    product_id BIGINT NOT NULL,
    product_name VARCHAR(100) NOT NULL,
    price_fen BIGINT NOT NULL,
    quantity BIGINT NOT NULL,
    PRIMARY KEY (id),
    KEY idx_order_items_v2_order_id (order_id),
    KEY idx_order_items_v2_product_id (product_id),
    CONSTRAINT fk_order_items_v2_order FOREIGN KEY (order_id) REFERENCES orders_v2 (id) ON DELETE CASCADE,
    CONSTRAINT chk_order_items_v2_price CHECK (price_fen > 0),
    CONSTRAINT chk_order_items_v2_quantity CHECK (quantity > 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

CREATE TABLE IF NOT EXISTS order_timeout_outbox_v2 (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    order_id BIGINT NOT NULL,
    due_at DATETIME(3) NOT NULL,
    status VARCHAR(20) NOT NULL,
    attempts INT NOT NULL DEFAULT 0,
    last_error VARCHAR(500) NOT NULL DEFAULT '',
    lease_owner VARCHAR(128) NOT NULL DEFAULT '',
    lease_until DATETIME(3) NULL,
    next_attempt_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    UNIQUE KEY uk_order_timeout_outbox_order (order_id),
    KEY idx_order_timeout_outbox_due_at (due_at),
    KEY idx_order_timeout_outbox_status (status),
    KEY idx_order_timeout_outbox_claim (status, next_attempt_at, lease_until),
    CONSTRAINT fk_order_timeout_outbox_order FOREIGN KEY (order_id) REFERENCES orders_v2 (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

-- +goose Down
DROP TABLE IF EXISTS order_timeout_outbox_v2;
DROP TABLE IF EXISTS order_items_v2;
DROP TABLE IF EXISTS orders_v2;
