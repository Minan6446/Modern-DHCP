CREATE TABLE IF NOT EXISTS ipv6_eui64_bindings (
    tenant_id     VARCHAR(64) NOT NULL,
    mac           VARCHAR(32) NOT NULL,
    prefix        VARCHAR(64) NOT NULL,
    ipv6_addr     VARCHAR(64) NOT NULL,
    lease_seconds BIGINT NOT NULL,
    created_at    DATETIME NOT NULL,
    updated_at    DATETIME NOT NULL,
    PRIMARY KEY (tenant_id, mac, prefix),
    UNIQUE KEY uniq_ipv6_eui64_addr (tenant_id, prefix, ipv6_addr),
    INDEX idx_ipv6_eui64_mac (tenant_id, mac)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
