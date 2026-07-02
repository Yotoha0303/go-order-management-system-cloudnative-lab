-- +goose Up
CREATE TABLE IF NOT EXISTS orders (
    id BIGINT NOT NULL AUTO_INCREMENT,
    order_no VARCHAR(255) NOT NULL,
    total_amount_fen BIGINT NOT NULL,
    status TINYINT NOT NULL DEFAULT 1,
    paid_at DATETIME(3) NULL DEFAULT NULL,
    completed_at DATETIME(3) NULL DEFAULT NULL,
    cancelled_at DATETIME(3) NULL DEFAULT NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    UNIQUE KEY uk_orders_order_no (order_no),
    KEY idx_orders_status (status),
    CONSTRAINT chk_orders_total_amount_fen_non_negative CHECK (total_amount_fen >= 0),
    CONSTRAINT chk_orders_status CHECK (status IN (1, 2, 3, 4))
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_general_ci;

-- +goose Down
DROP TABLE IF EXISTS orders;
