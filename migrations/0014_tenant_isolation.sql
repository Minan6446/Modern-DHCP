-- +goose Up
ALTER TABLE tenants
    ADD COLUMN db_driver VARCHAR(16) NULL AFTER quota_leases,
    ADD COLUMN db_dsn TEXT NULL AFTER db_driver,
    ADD COLUMN db_schema VARCHAR(64) NULL AFTER db_dsn,
    ADD COLUMN logo_url VARCHAR(256) NULL AFTER db_schema,
    ADD COLUMN primary_color VARCHAR(32) NULL AFTER logo_url,
    ADD COLUMN accent_color VARCHAR(32) NULL AFTER primary_color,
    ADD COLUMN login_message TEXT NULL AFTER accent_color;

CREATE TABLE IF NOT EXISTS tenant_quotas (
    tenant_id     VARCHAR(36) PRIMARY KEY,
    pool_limit    INT NOT NULL DEFAULT 0,
    lease_limit   INT NOT NULL DEFAULT 0,
    client_limit  INT NOT NULL DEFAULT 0,
    updated_at    DATETIME NOT NULL,
    FOREIGN KEY (tenant_id) REFERENCES tenants(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- +goose Down
DROP TABLE IF EXISTS tenant_quotas;
ALTER TABLE tenants
    DROP COLUMN login_message,
    DROP COLUMN accent_color,
    DROP COLUMN primary_color,
    DROP COLUMN logo_url,
    DROP COLUMN db_schema,
    DROP COLUMN db_dsn,
    DROP COLUMN db_driver;
