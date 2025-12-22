-- +goose Up
CREATE TABLE IF NOT EXISTS tenants (
    id            VARCHAR(36) PRIMARY KEY,
    name          VARCHAR(128) NOT NULL,
    brand_theme   JSON NULL,
    quota_pools   INT DEFAULT 0,
    quota_leases  INT DEFAULT 0,
    created_at    DATETIME NOT NULL,
    updated_at    DATETIME NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS lease_profiles (
    id                VARCHAR(36) PRIMARY KEY,
    tenant_id         VARCHAR(36) NOT NULL,
    name              VARCHAR(128) NOT NULL,
    default_duration  BIGINT NOT NULL,
    min_duration      BIGINT NOT NULL,
    max_duration      BIGINT NOT NULL,
    notification_lead BIGINT NOT NULL,
    device_type       VARCHAR(64) NULL,
    created_at        DATETIME NOT NULL,
    updated_at        DATETIME NOT NULL,
    FOREIGN KEY (tenant_id) REFERENCES tenants(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS address_pools (
    id               VARCHAR(36) PRIMARY KEY,
    tenant_id        VARCHAR(36) NOT NULL,
    parent_id        VARCHAR(36) NULL,
    name             VARCHAR(128) NOT NULL,
    cidr             VARCHAR(64) NOT NULL,
    vlan_id          INT NULL,
    interface_id     VARCHAR(64) NULL,
    ssid             VARCHAR(64) NULL,
    location         VARCHAR(128) NULL,
    reserve_percent  INT DEFAULT 10,
    lease_profile_id VARCHAR(36) NOT NULL,
    tags             JSON NULL,
    created_at       DATETIME NOT NULL,
    updated_at       DATETIME NOT NULL,
    FOREIGN KEY (tenant_id) REFERENCES tenants(id),
    FOREIGN KEY (lease_profile_id) REFERENCES lease_profiles(id),
    FOREIGN KEY (parent_id) REFERENCES address_pools(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS leases_v4 (
    id               VARCHAR(36) PRIMARY KEY,
    tenant_id        VARCHAR(36) NOT NULL,
    pool_id          VARCHAR(36) NOT NULL,
    ip_address       VARBINARY(16) NOT NULL,
    hardware_addr    VARCHAR(64) NOT NULL,
    client_id        VARCHAR(128) NULL,
    user_id          VARCHAR(64) NULL,
    device_type      VARCHAR(64) NULL,
    relay_info       JSON NULL,
    expires_at       DATETIME NOT NULL,
    state            VARCHAR(16) NOT NULL,
    cooldown_until   DATETIME NULL,
    conflict_history JSON NULL,
    created_at       DATETIME NOT NULL,
    updated_at       DATETIME NOT NULL,
    UNIQUE KEY uniq_lease_active (tenant_id, hardware_addr, state),
    FOREIGN KEY (tenant_id) REFERENCES tenants(id),
    FOREIGN KEY (pool_id) REFERENCES address_pools(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS leases_v6 LIKE leases_v4;

CREATE TABLE IF NOT EXISTS prefix_leases_v6 (
    id             VARCHAR(36) PRIMARY KEY,
    tenant_id     VARCHAR(36) NOT NULL,
    pool_id       VARCHAR(36) NOT NULL,
    client_id     VARCHAR(128) NOT NULL,
    iapd_id       INT UNSIGNED NOT NULL,
    prefix        VARCHAR(64) NOT NULL,
    prefix_length INT NOT NULL,
    state         VARCHAR(16) NOT NULL,
    expires_at    DATETIME NOT NULL,
    created_at    DATETIME NOT NULL,
    updated_at    DATETIME NOT NULL,
    UNIQUE KEY uniq_prefix_active (tenant_id, client_id, iapd_id, state),
    FOREIGN KEY (tenant_id) REFERENCES tenants(id),
    FOREIGN KEY (pool_id) REFERENCES address_pools(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS static_bindings (
    id               VARCHAR(36) PRIMARY KEY,
    tenant_id        VARCHAR(36) NOT NULL,
    identifier       VARCHAR(128) NOT NULL,
    identifier_type  VARCHAR(32) NOT NULL,
    pool_id          VARCHAR(36) NOT NULL,
    ip_address       VARBINARY(16) NOT NULL,
    lease_profile_id VARCHAR(36) NOT NULL,
    metadata         JSON NULL,
    created_at       DATETIME NOT NULL,
    updated_at       DATETIME NOT NULL,
    UNIQUE KEY uniq_binding (tenant_id, identifier, identifier_type),
    FOREIGN KEY (tenant_id) REFERENCES tenants(id),
    FOREIGN KEY (pool_id) REFERENCES address_pools(id),
    FOREIGN KEY (lease_profile_id) REFERENCES lease_profiles(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS policy_rules (
    id         VARCHAR(36) PRIMARY KEY,
    tenant_id  VARCHAR(36) NOT NULL,
    priority   INT NOT NULL,
    conditions JSON NOT NULL,
    actions    JSON NOT NULL,
    enabled    TINYINT NOT NULL DEFAULT 1,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    FOREIGN KEY (tenant_id) REFERENCES tenants(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS audit_events (
    id              BIGINT AUTO_INCREMENT PRIMARY KEY,
    audit_id        VARCHAR(64) NOT NULL,
    tenant_id       VARCHAR(36) NOT NULL,
    actor           VARCHAR(64) NOT NULL,
    action          VARCHAR(64) NOT NULL,
    source          VARCHAR(64) NOT NULL DEFAULT 'api',
    resource        VARCHAR(128) NULL,
    correlation_id  VARCHAR(64) NULL,
    payload         JSON,
    created_at      DATETIME NOT NULL,
    UNIQUE KEY uk_audit_audit_id (audit_id),
    INDEX idx_audit_tenant_created (tenant_id, created_at),
    INDEX idx_audit_tenant_correlation (tenant_id, correlation_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- +goose Down
DROP TABLE IF EXISTS audit_events;
DROP TABLE IF EXISTS policy_rules;
DROP TABLE IF EXISTS static_bindings;
DROP TABLE IF EXISTS prefix_leases_v6;
DROP TABLE IF EXISTS leases_v6;
DROP TABLE IF EXISTS leases_v4;
DROP TABLE IF EXISTS address_pools;
DROP TABLE IF EXISTS lease_profiles;
DROP TABLE IF EXISTS tenants;
