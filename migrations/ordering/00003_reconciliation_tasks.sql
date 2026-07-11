-- +goose Up
CREATE TABLE IF NOT EXISTS order_reconciliation_tasks (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    order_id BIGINT NOT NULL,
    action VARCHAR(64) NOT NULL,
    status VARCHAR(20) NOT NULL,
    attempts INT NOT NULL DEFAULT 0,
    next_attempt_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    lease_owner VARCHAR(128) NOT NULL DEFAULT '',
    lease_until DATETIME(3) NULL,
    last_error VARCHAR(500) NOT NULL DEFAULT '',
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    UNIQUE KEY uk_order_reconciliation_order_action (order_id, action),
    KEY idx_order_reconciliation_claim (status, next_attempt_at, lease_until),
    KEY idx_order_reconciliation_order (order_id),
    CONSTRAINT fk_order_reconciliation_order FOREIGN KEY (order_id) REFERENCES orders_v2 (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

-- +goose StatementBegin
CREATE TRIGGER trg_orders_v2_create_reconciliation_task
AFTER UPDATE ON orders_v2
FOR EACH ROW
BEGIN
    IF NEW.status = 'reconciliation_required' AND OLD.status <> NEW.status THEN
        INSERT INTO order_reconciliation_tasks (
            order_id,
            action,
            status,
            attempts,
            next_attempt_at,
            lease_owner,
            lease_until,
            last_error,
            created_at,
            updated_at
        ) VALUES (
            NEW.id,
            CASE OLD.status
                WHEN 'reserving' THEN 'release_inventory_and_fail'
                WHEN 'cancelling' THEN 'finalize_cancel'
                WHEN 'paying' THEN 'finalize_payment'
                ELSE CONCAT('unsupported_from_', OLD.status)
            END,
            CASE OLD.status
                WHEN 'reserving' THEN 'pending'
                WHEN 'cancelling' THEN 'pending'
                WHEN 'paying' THEN 'pending'
                ELSE 'unresolved'
            END,
            0,
            CURRENT_TIMESTAMP(3),
            '',
            NULL,
            CASE OLD.status
                WHEN 'reserving' THEN ''
                WHEN 'cancelling' THEN ''
                WHEN 'paying' THEN ''
                ELSE CONCAT('unsupported reconciliation transition from ', OLD.status)
            END,
            CURRENT_TIMESTAMP(3),
            CURRENT_TIMESTAMP(3)
        )
        ON DUPLICATE KEY UPDATE
            updated_at = CURRENT_TIMESTAMP(3);
    END IF;
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER IF EXISTS trg_orders_v2_create_reconciliation_task;
DROP TABLE IF EXISTS order_reconciliation_tasks;
