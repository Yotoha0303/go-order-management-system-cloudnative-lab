-- +goose Up
CREATE TABLE IF NOT EXISTS roles (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    role_name VARCHAR(25) NOT NULL,
    description VARCHAR(255) NOT NULL DEFAULT '',
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    UNIQUE KEY uk_role_name (role_name)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_general_ci;

INSERT IGNORE
    roles (role_name, description)
VALUES ('admin', 'This is the admin.');

INSERT IGNORE
    roles (role_name, description)
VALUES (
        'user',
        'This is the user.'
    );

-- +goose Down
DROP TABLE IF EXISTS roles;
