-- +goose Up
CREATE TABLE IF NOT EXISTS stock_logs (
    id BIGINT NOT NULL AUTO_INCREMENT,
    product_id BIGINT NOT NULL,
    change_quantity BIGINT NOT NULL,
    before_quantity BIGINT NOT NULL,
    after_quantity BIGINT NOT NULL,
    biz_type TINYINT NOT NULL,
    biz_id BIGINT NULL DEFAULT NULL,
    remark VARCHAR(255) NOT NULL DEFAULT '',
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    KEY idx_stock_logs_product_id (product_id),
    KEY idx_stock_logs_biz_type (biz_type),
    KEY idx_stock_logs_biz_id (biz_id),
    CONSTRAINT chk_stock_logs_before_quantity_non_negative CHECK (before_quantity >= 0),
    CONSTRAINT chk_stock_logs_after_quantity_non_negative CHECK (after_quantity >= 0),
    CONSTRAINT chk_stock_logs_quantity_balance CHECK (after_quantity = before_quantity + change_quantity),
    CONSTRAINT chk_stock_logs_biz_type CHECK (biz_type IN (1, 2, 3, 4))
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_general_ci;

-- +goose Down
DROP TABLE IF EXISTS stock_logs;
