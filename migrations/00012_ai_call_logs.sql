-- +goose Up
ALTER TABLE orders
    ADD KEY idx_orders_created_at_status (created_at, status);

CREATE TABLE ai_call_logs (
    id BIGINT NOT NULL PRIMARY KEY AUTO_INCREMENT,
    request_id VARCHAR(64) NOT NULL,
    user_id BIGINT NOT NULL,
    intent VARCHAR(64) NOT NULL DEFAULT '',
    tool_name VARCHAR(64) NOT NULL DEFAULT '',
    provider VARCHAR(64) NOT NULL DEFAULT '',
    model VARCHAR(128) NOT NULL DEFAULT '',
    prompt_tokens INT UNSIGNED NOT NULL DEFAULT 0,
    completion_tokens INT UNSIGNED NOT NULL DEFAULT 0,
    total_tokens INT UNSIGNED NOT NULL DEFAULT 0,
    latency_ms BIGINT UNSIGNED NOT NULL DEFAULT 0,
    status VARCHAR(16) NOT NULL,
    error_code VARCHAR(64) NOT NULL DEFAULT '',
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    UNIQUE KEY uk_ai_call_logs_request_id (request_id),
    KEY idx_ai_call_logs_user_created (user_id, created_at),
    KEY idx_ai_call_logs_status_created (status, created_at)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_general_ci;

-- +goose Down
DROP TABLE IF EXISTS ai_call_logs;

ALTER TABLE orders
    DROP INDEX idx_orders_created_at_status;
