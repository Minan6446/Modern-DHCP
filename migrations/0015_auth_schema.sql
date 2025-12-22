-- Auth module baseline schema
CREATE TABLE IF NOT EXISTS auth_users (
    id CHAR(36) NOT NULL PRIMARY KEY,
    username VARCHAR(64) NOT NULL UNIQUE,
    display_name VARCHAR(128) NOT NULL,
    email VARCHAR(160) NULL,
    password_hash VARBINARY(255) NOT NULL,
    role VARCHAR(32) NOT NULL DEFAULT 'reader',
    tenant_id VARCHAR(64) NOT NULL DEFAULT 'default',
    status VARCHAR(16) NOT NULL DEFAULT 'active',
    must_change_password TINYINT(1) NOT NULL DEFAULT 0,
    last_login_at DATETIME NULL,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS auth_api_keys (
    id CHAR(36) NOT NULL PRIMARY KEY,
    name VARCHAR(128) NOT NULL,
    prefix VARCHAR(16) NOT NULL,
    token_hash CHAR(64) NOT NULL UNIQUE,
    role VARCHAR(32) NOT NULL,
    principal_id VARCHAR(128) NOT NULL,
    tenant_scope VARCHAR(64) NULL,
    kind VARCHAR(16) NOT NULL DEFAULT 'service',
    owner_user_id CHAR(36) NULL,
    description TEXT NULL,
    created_by VARCHAR(128) NOT NULL,
    created_at DATETIME NOT NULL,
    expires_at DATETIME NULL,
    last_used_at DATETIME NULL,
    revoked_at DATETIME NULL,
    revoked_by VARCHAR(128) NULL,
    metadata JSON NULL,
    INDEX idx_auth_api_keys_prefix (prefix),
    INDEX idx_auth_api_keys_owner (owner_user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
