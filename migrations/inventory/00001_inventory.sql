-- +goose Up
CREATE TABLE IF NOT EXISTS inventory_items (
    product_id BIGINT NOT NULL,
    available_quantity BIGINT NOT NULL,
    reserved_quantity BIGINT NOT NULL DEFAULT 0,
    version BIGINT NOT NULL DEFAULT 0,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (product_id),
    CONSTRAINT chk_inventory_available CHECK (available_quantity >= 0),
    CONSTRAINT chk_inventory_reserved CHECK (reserved_quantity >= 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

CREATE TABLE IF NOT EXISTS inventory_reservations (
    id VARCHAR(36) NOT NULL,
    order_id BIGINT NOT NULL,
    status VARCHAR(20) NOT NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    UNIQUE KEY uk_inventory_reservation_order (order_id),
    KEY idx_inventory_reservations_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

CREATE TABLE IF NOT EXISTS inventory_reservation_items (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    reservation_id VARCHAR(36) NOT NULL,
    product_id BIGINT NOT NULL,
    quantity BIGINT NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY uk_reservation_product (reservation_id, product_id),
    KEY idx_inventory_reservation_items_reservation_id (reservation_id),
    CONSTRAINT fk_inventory_reservation_items_reservation
        FOREIGN KEY (reservation_id) REFERENCES inventory_reservations (id) ON DELETE CASCADE,
    CONSTRAINT chk_inventory_reservation_quantity CHECK (quantity > 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

CREATE TABLE IF NOT EXISTS inventory_stock_logs (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    product_id BIGINT NOT NULL,
    change_type VARCHAR(32) NOT NULL,
    quantity BIGINT NOT NULL,
    reference_id VARCHAR(64) NOT NULL DEFAULT '',
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    KEY idx_inventory_stock_logs_product_id (product_id),
    KEY idx_inventory_stock_logs_change_type (change_type),
    KEY idx_inventory_stock_logs_reference_id (reference_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

-- +goose Down
DROP TABLE IF EXISTS inventory_stock_logs;
DROP TABLE IF EXISTS inventory_reservation_items;
DROP TABLE IF EXISTS inventory_reservations;
DROP TABLE IF EXISTS inventory_items;
