-- +goose Up
CREATE TABLE IF NOT EXISTS security_policy_rules (
    id CHAR(36) NOT NULL PRIMARY KEY,
    tenant_id VARCHAR(36) NOT NULL,
    name VARCHAR(128) NOT NULL,
    description TEXT NULL,
    priority INT NOT NULL DEFAULT 100,
    effect VARCHAR(16) NOT NULL DEFAULT 'allow',
    enabled TINYINT(1) NOT NULL DEFAULT 1,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    KEY idx_security_policy_rules_tenant (tenant_id),
    KEY idx_security_policy_rules_priority (tenant_id, priority, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS security_policy_matches (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    rule_id CHAR(36) NOT NULL,
    tenant_id VARCHAR(36) NOT NULL,
    match_type VARCHAR(32) NOT NULL,
    match_value VARCHAR(255) NOT NULL,
    negate TINYINT(1) NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL,
    FOREIGN KEY (rule_id) REFERENCES security_policy_rules(id) ON DELETE CASCADE,
    KEY idx_security_policy_matches_rule (rule_id),
    KEY idx_security_policy_matches_tenant_type (tenant_id, match_type)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- +goose Down
DROP TABLE IF EXISTS security_policy_matches;
DROP TABLE IF EXISTS security_policy_rules;
