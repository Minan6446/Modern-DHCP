-- +goose Up
CREATE TABLE IF NOT EXISTS iot_device_profiles (
    id               VARCHAR(36) PRIMARY KEY,
    tenant_id        VARCHAR(36) NOT NULL,
    name             VARCHAR(128) NOT NULL,
    description      TEXT NULL,
    sleep_class      VARCHAR(16) NOT NULL DEFAULT 'NORMAL',
    sleep_interval   BIGINT NOT NULL DEFAULT 0,
    offline_window   BIGINT NOT NULL DEFAULT 0,
    lease_profile_id VARCHAR(36) NULL,
    sleepy_capable   TINYINT(1) NOT NULL DEFAULT 0,
    metadata         JSON NULL,
    created_at       DATETIME NOT NULL,
    updated_at       DATETIME NOT NULL,
    UNIQUE KEY uniq_iot_profile_name (tenant_id, name),
    FOREIGN KEY (tenant_id) REFERENCES tenants(id),
    FOREIGN KEY (lease_profile_id) REFERENCES lease_profiles(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS iot_devices (
    id               VARCHAR(36) PRIMARY KEY,
    tenant_id        VARCHAR(36) NOT NULL,
    device_id        VARCHAR(128) NOT NULL,
    display_name     VARCHAR(128) NULL,
    hardware_addr    VARCHAR(64) NULL,
    profile_id       VARCHAR(36) NULL,
    lease_profile_id VARCHAR(36) NULL,
    sleep_class      VARCHAR(16) NOT NULL DEFAULT 'NORMAL',
    sleep_interval   BIGINT NOT NULL DEFAULT 0,
    offline_window   BIGINT NOT NULL DEFAULT 0,
    sleepy_hint      TINYINT(1) NOT NULL DEFAULT 0,
    status           VARCHAR(16) NOT NULL DEFAULT 'ACTIVE',
    firmware_version VARCHAR(64) NULL,
    labels           JSON NULL,
    metadata         JSON NULL,
    last_seen        DATETIME NULL,
    created_at       DATETIME NOT NULL,
    updated_at       DATETIME NOT NULL,
    UNIQUE KEY uniq_iot_device (tenant_id, device_id),
    INDEX idx_iot_device_profile (profile_id),
    INDEX idx_iot_device_status (tenant_id, status),
    FOREIGN KEY (tenant_id) REFERENCES tenants(id),
    FOREIGN KEY (profile_id) REFERENCES iot_device_profiles(id) ON DELETE SET NULL,
    FOREIGN KEY (lease_profile_id) REFERENCES lease_profiles(id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- +goose Down
DROP TABLE IF EXISTS iot_devices;
DROP TABLE IF EXISTS iot_device_profiles;
