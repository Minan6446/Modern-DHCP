-- migrations/0000_merged_no_tenant.sql
-- Consolidated no-tenant schema (multi-user, single database)
-- +goose Up

-- Auth
CREATE TABLE IF NOT EXISTS auth_users (
    id CHAR(36) PRIMARY KEY,
    username VARCHAR(64) NOT NULL UNIQUE,
    display_name VARCHAR(128) NOT NULL,
    email VARCHAR(160) NULL,
    password_hash VARBINARY(255) NOT NULL,
    role VARCHAR(32) NOT NULL DEFAULT 'reader',
    status VARCHAR(16) NOT NULL DEFAULT 'active',
    must_change_password TINYINT(1) NOT NULL DEFAULT 0,
    last_login_at DATETIME NULL,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS auth_user_roles (
    id CHAR(36) PRIMARY KEY,
    user_id CHAR(36) NOT NULL,
    role VARCHAR(64) NOT NULL,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    UNIQUE KEY uniq_user_role (user_id, role),
    INDEX idx_user_roles_user (user_id),
    CONSTRAINT fk_user_roles_user FOREIGN KEY (user_id) REFERENCES auth_users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS auth_api_keys (
    id CHAR(36) PRIMARY KEY,
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

CREATE TABLE IF NOT EXISTS auth_identity_providers (
    id VARCHAR(64) PRIMARY KEY,
    name VARCHAR(128) NOT NULL,
    type VARCHAR(32) NOT NULL,
    endpoint VARCHAR(512) NULL,
    config JSON NULL,
    enabled TINYINT(1) NOT NULL DEFAULT 1,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- RBAC (no tenant dimension)
CREATE TABLE IF NOT EXISTS rbac_roles (
    name          VARCHAR(64) PRIMARY KEY,
    inherits_from VARCHAR(64) NULL,
    description   TEXT NULL,
    capabilities  JSON NULL,
    created_at    DATETIME NOT NULL,
    updated_at    DATETIME NOT NULL,
    CONSTRAINT fk_rbac_roles_inherits FOREIGN KEY (inherits_from) REFERENCES rbac_roles(name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS rbac_org_units (
    id         VARCHAR(64) PRIMARY KEY,
    parent_id  VARCHAR(64) NULL,
    name       VARCHAR(128) NOT NULL,
    path       VARCHAR(512) NULL,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    INDEX idx_rbac_org_units_parent (parent_id),
    CONSTRAINT fk_rbac_org_units_parent FOREIGN KEY (parent_id) REFERENCES rbac_org_units(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS rbac_assignments (
    id             VARCHAR(64) PRIMARY KEY,
    principal_id   VARCHAR(128) NOT NULL,
    role_name      VARCHAR(64) NOT NULL,
    org_unit_id    VARCHAR(64) NULL,
    resource_type  VARCHAR(64) NULL,
    resource_id    VARCHAR(64) NULL,
    created_by     VARCHAR(128) NOT NULL,
    created_at     DATETIME NOT NULL,
    expires_at     DATETIME NULL,
    attributes     JSON NULL,
    group_ids      JSON NULL,
    labels         JSON NULL,
    org_scope      VARCHAR(64) AS (COALESCE(org_unit_id, '')) STORED,
    resource_scope VARCHAR(64) AS (COALESCE(resource_type, '')) STORED,
    resource_key   VARCHAR(64) AS (COALESCE(resource_id, '')) STORED,
    group_scope    VARCHAR(191) NOT NULL DEFAULT '',
    label_scope    VARCHAR(191) NOT NULL DEFAULT '',
    UNIQUE KEY uniq_assignment_scope (principal_id, role_name, org_scope, resource_scope, resource_key),
    INDEX idx_assignment_principal (principal_id),
    INDEX idx_assignment_org_unit (org_unit_id),
    INDEX idx_assignment_group_scope (group_scope),
    INDEX idx_assignment_label_scope (label_scope),
    CONSTRAINT fk_rbac_assignments_role FOREIGN KEY (role_name) REFERENCES rbac_roles(name),
    CONSTRAINT fk_rbac_assignments_org_unit FOREIGN KEY (org_unit_id) REFERENCES rbac_org_units(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- Normalize nullable RBAC JSON columns to avoid scan errors in mixed historical data.
UPDATE rbac_assignments
SET group_ids = COALESCE(group_ids, '[]'),
    labels = COALESCE(labels, '{}'),
    attributes = COALESCE(attributes, '{}')
WHERE group_ids IS NULL OR labels IS NULL OR attributes IS NULL;

CREATE TABLE IF NOT EXISTS rbac_temp_grants (
    id             VARCHAR(64) PRIMARY KEY,
    assignment_id  VARCHAR(64) NOT NULL,
    requested_by   VARCHAR(128) NOT NULL,
    reason         TEXT NULL,
    status         VARCHAR(16) NOT NULL DEFAULT 'pending',
    expires_at     DATETIME NOT NULL,
    approved_by    VARCHAR(128) NULL,
    approved_at    DATETIME NULL,
    created_at     DATETIME NOT NULL,
    updated_at     DATETIME NOT NULL,
    INDEX idx_temp_grant_assignment (assignment_id),
    INDEX idx_temp_grant_status (status),
    INDEX idx_temp_grant_expiry (expires_at),
    CONSTRAINT fk_rbac_temp_grants_assignment FOREIGN KEY (assignment_id) REFERENCES rbac_assignments(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS rbac_approval_rules (
    id            VARCHAR(64) PRIMARY KEY,
    role_name     VARCHAR(64) NOT NULL,
    scope         VARCHAR(32) NOT NULL,
    min_approvers INT NOT NULL DEFAULT 1,
    approver_role VARCHAR(64) NOT NULL,
    created_at    DATETIME NOT NULL,
    updated_at    DATETIME NOT NULL,
    INDEX idx_approval_rules_role (role_name, scope),
    CONSTRAINT fk_rbac_approval_rules_role FOREIGN KEY (role_name) REFERENCES rbac_roles(name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- DHCP core (no tenant)
CREATE TABLE IF NOT EXISTS lease_profiles (
    id                VARCHAR(36) PRIMARY KEY,
    name              VARCHAR(128) NOT NULL,
    default_duration  BIGINT NOT NULL,
    min_duration      BIGINT NOT NULL,
    max_duration      BIGINT NOT NULL,
    renewal_time      BIGINT NOT NULL DEFAULT 0,
    rebinding_time    BIGINT NOT NULL DEFAULT 0,
    notification_lead BIGINT NOT NULL,
    infinite          TINYINT(1) NOT NULL DEFAULT 0,
    device_type       VARCHAR(64) NULL,
    sleepy_capable    TINYINT(1) NOT NULL DEFAULT 0,
    sleepy_offline_window BIGINT NOT NULL DEFAULT 0,
    sleepy_hold_duration BIGINT NOT NULL DEFAULT 0,
    mobility_grace_period BIGINT NOT NULL DEFAULT 0,
    created_at        DATETIME NOT NULL,
    updated_at        DATETIME NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS address_pools (
    id               VARCHAR(36) PRIMARY KEY,
    parent_id        VARCHAR(36) NULL,
    scope            VARCHAR(16) NOT NULL DEFAULT 'GLOBAL',
    name             VARCHAR(128) NOT NULL,
    cidr             VARCHAR(64) NOT NULL,
    network          VARCHAR(64) NULL,
    netmask          VARCHAR(64) NULL,
    range_start      VARCHAR(64) NULL,
    range_end        VARCHAR(64) NULL,
    gateway          VARCHAR(64) NULL,
    option_43        VARCHAR(255) NOT NULL DEFAULT '',
    dns              JSON NULL,
    exclusions       JSON NULL,
    vlan_id          INT NULL,
    interface_id     VARCHAR(64) NULL,
    ssid             VARCHAR(64) NULL,
    location         VARCHAR(128) NULL,
    geo_code         VARCHAR(128) NULL,
    device_profile   VARCHAR(64) NULL,
    tag_fingerprint  CHAR(40) AS (sha1(json_unquote(json_extract(tags, '$')))) STORED,
    reserve_percent  INT DEFAULT 10,
    min_lease_time   INT NOT NULL DEFAULT 3600,
    max_lease_time   INT NOT NULL DEFAULT 7200,
    lease_profile_id VARCHAR(36) NOT NULL,
    tags             JSON NULL,
    allocation_mode  VARCHAR(32) NOT NULL DEFAULT 'ROUND_ROBIN',
    priority_weight  INT NOT NULL DEFAULT 1,
    status           VARCHAR(16) NOT NULL DEFAULT 'active',
    created_at       DATETIME NOT NULL,
    updated_at       DATETIME NOT NULL,
    FOREIGN KEY (lease_profile_id) REFERENCES lease_profiles(id),
    FOREIGN KEY (parent_id) REFERENCES address_pools(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS pool_usage_daily (
    tenant_id VARCHAR(64) NOT NULL,
    pool_id VARCHAR(64) NOT NULL,
    day DATE NOT NULL,
    used BIGINT NOT NULL,
    capacity BIGINT NOT NULL,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    PRIMARY KEY (tenant_id, pool_id, day),
    INDEX idx_pool_usage_day (tenant_id, day),
    INDEX idx_pool_usage_pool (tenant_id, pool_id, day)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS leases_v4 (
    id                    VARCHAR(36) PRIMARY KEY,
    pool_id               VARCHAR(36) NOT NULL,
    ip_address            VARBINARY(16) NOT NULL,
    hardware_addr         VARCHAR(64) NOT NULL,
    client_id             VARCHAR(128) NULL,
    user_id               VARCHAR(64) NULL,
    mobility_anchor_id    VARCHAR(128) NOT NULL DEFAULT '',
    device_type           VARCHAR(64) NULL,
    last_access_point_id  VARCHAR(128) NOT NULL DEFAULT '',
    last_controller_id    VARCHAR(128) NOT NULL DEFAULT '',
    last_geo_zone         VARCHAR(128) NOT NULL DEFAULT '',
    mobility_location_hint VARCHAR(255) NOT NULL DEFAULT '',
    relay_info            JSON NULL,
    session_continuity    JSON NULL,
    expires_at            DATETIME NOT NULL,
    state                 VARCHAR(16) NOT NULL,
    security_state        VARCHAR(16) NOT NULL DEFAULT 'OK',
    cooldown_until        DATETIME NULL,
    conflict_history      JSON NULL,
    mdm_managed           TINYINT(1) NOT NULL DEFAULT 0,
    mdm_source            VARCHAR(64) NULL,
    mdm_tags              JSON NULL,
    mdm_observed_at       DATETIME NULL,
    created_at            DATETIME NOT NULL,
    updated_at            DATETIME NOT NULL,
    UNIQUE KEY uniq_lease_active (hardware_addr, state),
    FOREIGN KEY (pool_id) REFERENCES address_pools(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS leases_v6 LIKE leases_v4;

CREATE TABLE IF NOT EXISTS lease_history_v4 (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    lease_id VARCHAR(36) NOT NULL,
    tenant_id VARCHAR(64) NOT NULL DEFAULT 'global',
    pool_id VARCHAR(36) NOT NULL,
    ip_address VARCHAR(64) NOT NULL,
    hardware_addr VARCHAR(64) NOT NULL,
    client_id VARCHAR(128) NULL,
    user_id VARCHAR(64) NULL,
    mobility_anchor_id VARCHAR(128) NOT NULL DEFAULT '',
    device_type VARCHAR(64) NULL,
    last_access_point_id VARCHAR(128) NOT NULL DEFAULT '',
    last_controller_id VARCHAR(128) NOT NULL DEFAULT '',
    last_geo_zone VARCHAR(128) NOT NULL DEFAULT '',
    mobility_location_hint VARCHAR(255) NOT NULL DEFAULT '',
    mdm_managed TINYINT(1) NOT NULL DEFAULT 0,
    mdm_source VARCHAR(64) NULL,
    mdm_tags JSON NULL,
    mdm_observed_at DATETIME NULL,
    relay_info JSON NULL,
    session_continuity JSON NULL,
    expires_at DATETIME NOT NULL,
    final_state VARCHAR(16) NOT NULL,
    security_state VARCHAR(16) NOT NULL DEFAULT 'OK',
    cooldown_until DATETIME NULL,
    conflict_history JSON NULL,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    archived_at DATETIME NOT NULL,
    UNIQUE KEY uniq_lease_history_lease_id (lease_id),
    INDEX idx_lease_history_tenant_archived (tenant_id, archived_at DESC),
    INDEX idx_lease_history_pool_archived (pool_id, archived_at DESC),
    INDEX idx_lease_history_hw_archived (hardware_addr, archived_at DESC),
    INDEX idx_lease_history_ip_archived (ip_address, archived_at DESC)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS prefix_leases_v6 (
    id             VARCHAR(36) PRIMARY KEY,
    pool_id        VARCHAR(36) NOT NULL,
    client_id      VARCHAR(128) NOT NULL,
    iapd_id        INT UNSIGNED NOT NULL,
    prefix         VARCHAR(64) NOT NULL,
    prefix_length  INT NOT NULL,
    state          VARCHAR(16) NOT NULL,
    expires_at     DATETIME NOT NULL,
    created_at     DATETIME NOT NULL,
    updated_at     DATETIME NOT NULL,
    UNIQUE KEY uniq_prefix_active (client_id, iapd_id, state),
    FOREIGN KEY (pool_id) REFERENCES address_pools(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS static_bindings (
    id               VARCHAR(36) PRIMARY KEY,
    identifier       VARCHAR(128) NOT NULL,
    identifier_type  VARCHAR(32) NOT NULL,
    pool_id          VARCHAR(36) NOT NULL,
    ip_address       VARBINARY(16) NOT NULL,
    lease_profile_id VARCHAR(36) NOT NULL,
    metadata         JSON NULL,
    status           VARCHAR(16) NOT NULL DEFAULT 'offline',
    status_source    VARCHAR(32) NULL,
    last_seen_at     DATETIME NULL,
    status_updated_at DATETIME NULL,
    created_at       DATETIME NOT NULL,
    updated_at       DATETIME NOT NULL,
    UNIQUE KEY uniq_binding (identifier, identifier_type),
    FOREIGN KEY (pool_id) REFERENCES address_pools(id),
    FOREIGN KEY (lease_profile_id) REFERENCES lease_profiles(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS policy_rules (
    id         VARCHAR(36) PRIMARY KEY,
    priority   INT NOT NULL,
    conditions JSON NOT NULL,
    actions    JSON NOT NULL,
    enabled    TINYINT NOT NULL DEFAULT 1,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS audit_events (
    id             BIGINT AUTO_INCREMENT PRIMARY KEY,
    audit_id       VARCHAR(64) NOT NULL,
    actor          VARCHAR(64) NOT NULL,
    action         VARCHAR(64) NOT NULL,
    source         VARCHAR(64) NOT NULL DEFAULT 'api',
    resource       VARCHAR(128) NULL,
    correlation_id VARCHAR(64) NULL,
    payload        JSON,
    created_at     DATETIME NOT NULL,
    UNIQUE KEY uk_audit_audit_id (audit_id),
    INDEX idx_audit_created (created_at),
    INDEX idx_audit_correlation (correlation_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- Ops
CREATE TABLE IF NOT EXISTS ops_system_settings (
    id                  TINYINT UNSIGNED NOT NULL PRIMARY KEY DEFAULT 1,
    theme               VARCHAR(64) NOT NULL DEFAULT '',
    locale              VARCHAR(32) NOT NULL DEFAULT '',
    maintenance_mode    TINYINT(1) NOT NULL DEFAULT 0,
    maintenance_window  VARCHAR(128) NULL,
    announcement        TEXT NULL,
    updated_at          DATETIME NOT NULL,
    updated_by          VARCHAR(128) NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

INSERT INTO ops_system_settings (id, theme, locale, maintenance_mode, maintenance_window, announcement, updated_at, updated_by)
VALUES (1, '', '', 0, '', '', NOW(), 'system')
ON DUPLICATE KEY UPDATE updated_at = VALUES(updated_at);

-- Security policy + MAC list
CREATE TABLE IF NOT EXISTS security_policy_rules (
    id CHAR(36) NOT NULL PRIMARY KEY,
    name VARCHAR(128) NOT NULL,
    description TEXT NULL,
    priority INT NOT NULL DEFAULT 100,
    effect VARCHAR(16) NOT NULL DEFAULT 'allow',
    enabled TINYINT(1) NOT NULL DEFAULT 1,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    KEY idx_security_policy_rules_priority (priority, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS security_policy_matches (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    rule_id CHAR(36) NOT NULL,
    match_type VARCHAR(32) NOT NULL,
    match_value VARCHAR(255) NOT NULL,
    negate TINYINT(1) NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL,
    FOREIGN KEY (rule_id) REFERENCES security_policy_rules(id) ON DELETE CASCADE,
    KEY idx_security_policy_matches_rule (rule_id),
    KEY idx_security_policy_matches_type (match_type)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS security_mac_lists (
    id          VARCHAR(36) PRIMARY KEY,
    mac         VARCHAR(64) NOT NULL,
    list_type   VARCHAR(16) NOT NULL,
    action      VARCHAR(16) NOT NULL DEFAULT 'monitor',
    description VARCHAR(255) NULL,
    source      VARCHAR(64) NULL,
    priority    INT NOT NULL DEFAULT 100,
    enabled     TINYINT NOT NULL DEFAULT 1,
    valid_from  DATETIME NULL,
    valid_until DATETIME NULL,
    metadata    JSON NULL,
    created_at  DATETIME NOT NULL,
    updated_at  DATETIME NOT NULL,
    UNIQUE KEY uniq_mac_list (mac, list_type)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- Alerting
CREATE TABLE IF NOT EXISTS alert_rules (
    id VARCHAR(64) NOT NULL PRIMARY KEY,
    tenant_id VARCHAR(64) NOT NULL,
    name VARCHAR(255) NOT NULL,
    expression VARCHAR(512) NOT NULL,
    operator VARCHAR(8) NOT NULL,
    threshold DOUBLE NOT NULL,
    duration_sec INT NOT NULL,
    severity VARCHAR(32) NOT NULL,
    enabled TINYINT(1) NOT NULL DEFAULT 1,
    match_template TEXT NULL,
    created_at DATETIME(6) NOT NULL,
    updated_at DATETIME(6) NOT NULL,
    INDEX idx_alert_rules_tenant (tenant_id),
    INDEX idx_alert_rules_name (tenant_id, name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS alert_routes (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    name VARCHAR(128) NOT NULL UNIQUE,
    description VARCHAR(512) NULL,
    severities JSON NOT NULL,
    channels JSON NOT NULL,
    escalation_minutes INT NOT NULL DEFAULT 0,
    enabled TINYINT(1) NOT NULL DEFAULT 1,
    updated_at DATETIME(6) NOT NULL,
    updated_by VARCHAR(128) NOT NULL DEFAULT 'system'
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS alert_thresholds (
    tenant_id VARCHAR(64) NOT NULL PRIMARY KEY,
    resource_pool_usage INT NOT NULL DEFAULT 0,
    resource_lease_usage INT NOT NULL DEFAULT 0,
    resource_renew_fail INT NOT NULL DEFAULT 0,
    resource_lease_time_drift INT NOT NULL DEFAULT 0,
    resource_failed_request_ratio INT NOT NULL DEFAULT 0,
    resource_subnet_imbalance INT NOT NULL DEFAULT 0,
    resource_log_error_threshold INT NOT NULL DEFAULT 0,
    server_response_timeout INT NOT NULL DEFAULT 0,
    server_response_time_ms INT NOT NULL DEFAULT 0,
    server_cpu_usage INT NOT NULL DEFAULT 0,
    server_memory_usage INT NOT NULL DEFAULT 0,
    server_process_check TINYINT(1) NOT NULL DEFAULT 1,
    network_conflict_sensitivity VARCHAR(16) NOT NULL DEFAULT 'medium',
    network_abnormal_qps INT NOT NULL DEFAULT 0,
    network_duplicate_ip_detection TINYINT(1) NOT NULL DEFAULT 0,
    network_unauthorized_server_detection TINYINT(1) NOT NULL DEFAULT 0,
    updated_at DATETIME(6) NOT NULL,
    updated_by VARCHAR(128) NOT NULL DEFAULT 'system'
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS alert_notify_configs (
    tenant_id VARCHAR(64) NOT NULL PRIMARY KEY,
    channels_email TINYINT(1) NOT NULL DEFAULT 1,
    channels_sms TINYINT(1) NOT NULL DEFAULT 0,
    channels_webhook TINYINT(1) NOT NULL DEFAULT 0,
    webhook_url VARCHAR(512) NULL,
    policies_emergency JSON NOT NULL,
    policies_critical JSON NOT NULL,
    policies_info JSON NOT NULL,
    updated_at DATETIME(6) NOT NULL,
    updated_by VARCHAR(128) NOT NULL DEFAULT 'system'
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS alert_templates (
    id VARCHAR(64) NOT NULL PRIMARY KEY,
    tenant_id VARCHAR(64) NOT NULL,
    name VARCHAR(255) NOT NULL,
    lang VARCHAR(32) NOT NULL,
    channel VARCHAR(32) NOT NULL,
    subject VARCHAR(255) NULL,
    body TEXT NULL,
    variables JSON NULL,
    updated_at DATETIME(6) NOT NULL,
    updated_by VARCHAR(128) NOT NULL DEFAULT 'system',
    INDEX idx_alert_templates_tenant (tenant_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS alert_receivers (
    id VARCHAR(64) NOT NULL PRIMARY KEY,
    tenant_id VARCHAR(64) NOT NULL,
    name VARCHAR(128) NOT NULL,
    email VARCHAR(160) NOT NULL,
    phone VARCHAR(64) NULL,
    levels JSON NOT NULL,
    department VARCHAR(128) NULL,
    schedule VARCHAR(128) NULL,
    server_groups JSON NULL,
    created_at DATETIME(6) NOT NULL,
    updated_at DATETIME(6) NOT NULL,
    updated_by VARCHAR(128) NOT NULL DEFAULT 'system',
    INDEX idx_alert_receivers_tenant (tenant_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- Collaboration
CREATE TABLE IF NOT EXISTS collab_sessions (
    id             VARCHAR(64) PRIMARY KEY,
    resource_type  VARCHAR(64) NOT NULL,
    resource_id    VARCHAR(64) NOT NULL,
    user_id        VARCHAR(64) NOT NULL,
    status         VARCHAR(32) NOT NULL DEFAULT 'active',
    lock_version   INT NOT NULL DEFAULT 0,
    expires_at     DATETIME NOT NULL,
    metadata       JSON NULL,
    created_at     DATETIME NOT NULL,
    updated_at     DATETIME NOT NULL,
    deleted_at     DATETIME NULL,
    version        INT NOT NULL DEFAULT 0,
    UNIQUE KEY uniq_session_actor (resource_type, resource_id, user_id),
    INDEX idx_session_resource (resource_type, resource_id),
    INDEX idx_session_expires (expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS collab_locks (
    id             VARCHAR(64) PRIMARY KEY,
    resource_type  VARCHAR(64) NOT NULL,
    resource_id    VARCHAR(64) NOT NULL,
    session_id     VARCHAR(64) NOT NULL,
    status         VARCHAR(32) NOT NULL DEFAULT 'active',
    acquired_at    DATETIME NOT NULL,
    expires_at     DATETIME NOT NULL,
    created_at     DATETIME NOT NULL,
    updated_at     DATETIME NOT NULL,
    deleted_at     DATETIME NULL,
    version        INT NOT NULL DEFAULT 0,
    UNIQUE KEY uniq_lock_resource (resource_type, resource_id),
    INDEX idx_lock_session (session_id),
    INDEX idx_lock_expires (expires_at),
    CONSTRAINT fk_collab_locks_session FOREIGN KEY (session_id) REFERENCES collab_sessions(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS collab_comments (
    id             VARCHAR(64) PRIMARY KEY,
    resource_type  VARCHAR(64) NOT NULL,
    resource_id    VARCHAR(64) NOT NULL,
    author_id      VARCHAR(64) NOT NULL,
    parent_id      VARCHAR(64) NULL,
    body           TEXT NOT NULL,
    status         VARCHAR(32) NOT NULL DEFAULT 'open',
    created_at     DATETIME NOT NULL,
    updated_at     DATETIME NOT NULL,
    deleted_at     DATETIME NULL,
    version        INT NOT NULL DEFAULT 0,
    INDEX idx_comment_resource (resource_type, resource_id),
    INDEX idx_comment_parent (parent_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS collab_tasks (
    id             VARCHAR(64) PRIMARY KEY,
    title          VARCHAR(256) NOT NULL,
    assignee_id    VARCHAR(64) NULL,
    resource_type  VARCHAR(64) NOT NULL,
    resource_id    VARCHAR(64) NOT NULL,
    resource_ref   JSON NULL,
    state          VARCHAR(32) NOT NULL DEFAULT 'todo',
    priority       INT NOT NULL DEFAULT 3,
    due_at         DATETIME NULL,
    created_at     DATETIME NOT NULL,
    updated_at     DATETIME NOT NULL,
    deleted_at     DATETIME NULL,
    version        INT NOT NULL DEFAULT 0,
    INDEX idx_task_resource (resource_type, resource_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS collab_approvals (
    id             VARCHAR(64) PRIMARY KEY,
    workflow_id    VARCHAR(64) NOT NULL,
    stage          INT NOT NULL,
    approver_id    VARCHAR(64) NOT NULL,
    decision       VARCHAR(16) NOT NULL DEFAULT 'pending',
    decided_at     DATETIME NULL,
    comment        TEXT NULL,
    created_at     DATETIME NOT NULL,
    updated_at     DATETIME NOT NULL,
    deleted_at     DATETIME NULL,
    version        INT NOT NULL DEFAULT 0,
    INDEX idx_approval_workflow (workflow_id, stage)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS collab_events (
    id             BIGINT AUTO_INCREMENT PRIMARY KEY,
    session_id     VARCHAR(64) NULL,
    resource_type  VARCHAR(64) NULL,
    resource_id    VARCHAR(64) NULL,
    event_type     VARCHAR(64) NOT NULL,
    payload        JSON NULL,
    created_at     DATETIME NOT NULL,
    INDEX idx_event_time (created_at),
    INDEX idx_event_session (session_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- Automation & policy drafts
CREATE TABLE IF NOT EXISTS automation_jobs (
    id              VARCHAR(64) NOT NULL PRIMARY KEY,
    job_type        VARCHAR(64) NOT NULL,
    status          VARCHAR(32) NOT NULL DEFAULT 'pending',
    source          VARCHAR(32) NOT NULL DEFAULT '',
    triggered_by    VARCHAR(128) NOT NULL DEFAULT '',
    priority        INT NOT NULL DEFAULT 0,
    attempts        INT NOT NULL DEFAULT 0,
    payload_hash    CHAR(64) NOT NULL DEFAULT '',
    labels          JSON NULL,
    payload         JSON NULL,
    result_summary  TEXT NULL,
    error_message   TEXT NULL,
    not_before      DATETIME NULL,
    queued_at       DATETIME NOT NULL,
    started_at      DATETIME NULL,
    completed_at    DATETIME NULL,
    updated_at      DATETIME NOT NULL,
    INDEX idx_automation_jobs_type_status (job_type, status),
    INDEX idx_automation_jobs_updated (updated_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS automation_change_requests (
    id               VARCHAR(64) NOT NULL PRIMARY KEY,
    job_type         VARCHAR(64) NOT NULL,
    request_type     VARCHAR(32) NOT NULL,
    status           VARCHAR(32) NOT NULL DEFAULT 'pending',
    requested_by     VARCHAR(128) NOT NULL,
    requested_at     DATETIME NOT NULL,
    approver_id      VARCHAR(128) NULL,
    decided_at       DATETIME NULL,
    decision_note    TEXT NULL,
    original_config  JSON NULL,
    proposed_config  JSON NULL,
    payload          JSON NULL,
    auto_applied     TINYINT(1) NOT NULL DEFAULT 0,
    applied_at       DATETIME NULL,
    created_at       DATETIME NOT NULL,
    updated_at       DATETIME NOT NULL,
    version          INT NOT NULL DEFAULT 1,
    INDEX idx_auto_change_status (status),
    INDEX idx_auto_change_job_status (job_type, status),
    INDEX idx_auto_change_requested (requested_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS policy_drafts (
    id            VARCHAR(64) NOT NULL PRIMARY KEY,
    name          VARCHAR(128) NOT NULL,
    description   TEXT NULL,
    status        VARCHAR(32) NOT NULL DEFAULT 'draft',
    rules         JSON NOT NULL,
    metadata      JSON NULL,
    created_by    VARCHAR(128) NOT NULL,
    updated_by    VARCHAR(128) NOT NULL,
    created_at    DATETIME NOT NULL,
    updated_at    DATETIME NOT NULL,
    published_at  DATETIME NULL,
    UNIQUE KEY uniq_policy_drafts_name (name),
    INDEX idx_policy_drafts_updated (updated_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS policy_versions (
    id            VARCHAR(64) NOT NULL PRIMARY KEY,
    version       INT NOT NULL,
    derived_from  VARCHAR(64) NULL,
    changelog     TEXT NULL,
    rules         JSON NOT NULL,
    metadata      JSON NULL,
    published_by  VARCHAR(128) NOT NULL,
    published_at  DATETIME NOT NULL,
    rollback_of   VARCHAR(64) NULL,
    created_at    DATETIME NOT NULL,
    UNIQUE KEY uniq_policy_versions_version (version),
    INDEX idx_policy_versions_published (published_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- IoT registry (no tenant)
CREATE TABLE IF NOT EXISTS iot_device_profiles (
    id               VARCHAR(36) PRIMARY KEY,
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
    UNIQUE KEY uniq_iot_profile_name (name),
    FOREIGN KEY (lease_profile_id) REFERENCES lease_profiles(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS iot_devices (
    id               VARCHAR(36) PRIMARY KEY,
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
    UNIQUE KEY uniq_iot_device (device_id),
    INDEX idx_iot_device_profile (profile_id),
    INDEX idx_iot_device_status (status),
    FOREIGN KEY (profile_id) REFERENCES iot_device_profiles(id) ON DELETE SET NULL,
    FOREIGN KEY (lease_profile_id) REFERENCES lease_profiles(id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- DHCP custom options (no tenant)
CREATE TABLE IF NOT EXISTS dhcp_custom_options (
  id VARCHAR(64) NOT NULL PRIMARY KEY,
  code INT NOT NULL,
  name VARCHAR(255) NOT NULL,
  scope VARCHAR(24) NOT NULL,
  format VARCHAR(64) NOT NULL,
  data_type VARCHAR(64) NOT NULL,
  value TEXT NOT NULL,
  value_example TEXT NULL,
  allowed_values TEXT NULL,
  sample_value TEXT NULL,
  description TEXT NULL,
  tags TEXT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS dhcp_option_templates (
    id VARCHAR(64) NOT NULL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    description TEXT NULL,
    icon VARCHAR(32) NULL,
    options LONGTEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_dhcp_option_templates_name (name),
    INDEX idx_dhcp_option_templates_updated_at (updated_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS dhcp_option_template_apply_history (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    template_id VARCHAR(64) NOT NULL,
    template_name VARCHAR(255) NOT NULL DEFAULT '',
    action VARCHAR(32) NOT NULL DEFAULT 'apply',
    operator VARCHAR(128) NOT NULL DEFAULT 'system',
    target_scope VARCHAR(64) NOT NULL DEFAULT 'global',
    applied_option_count INT NOT NULL DEFAULT 0,
    template_snapshot LONGTEXT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_dhcp_tpl_history_template_time (template_id, created_at),
    INDEX idx_dhcp_tpl_history_operator (operator)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS dhcp_option_scopes (
    id VARCHAR(64) NOT NULL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    subnet VARCHAR(64) NOT NULL DEFAULT '',
    range_value VARCHAR(128) NOT NULL DEFAULT '',
    gateway VARCHAR(64) NOT NULL DEFAULT '',
    status VARCHAR(16) NOT NULL DEFAULT 'active',
    scope_type VARCHAR(32) NOT NULL DEFAULT 'GLOBAL',
    target VARCHAR(128) NOT NULL DEFAULT 'global',
    template_id VARCHAR(64) NULL,
    option_ids LONGTEXT NOT NULL,
    description TEXT NULL,
    notes TEXT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_dhcp_option_scopes_name (name),
    INDEX idx_dhcp_option_scopes_template_id (template_id),
    INDEX idx_dhcp_option_scopes_updated_at (updated_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE INDEX idx_dhcp_custom_options_code ON dhcp_custom_options (code);

-- Indexes
ALTER TABLE address_pools
    ADD INDEX idx_address_pools_created (created_at DESC),
    ADD INDEX idx_address_pools_scope_parent (scope, parent_id),
    ADD INDEX idx_address_pools_interface (interface_id),
    ADD INDEX idx_address_pools_ssid (ssid),
    ADD INDEX idx_address_pools_location (location),
    ADD INDEX idx_address_pools_vlan (vlan_id),
    ADD INDEX idx_address_pools_geo (geo_code),
    ADD INDEX idx_address_pools_device (device_profile),
    ADD INDEX idx_address_pools_geo_location (geo_code, location),
    ADD INDEX idx_address_pools_tag_fingerprint (tag_fingerprint);

ALTER TABLE static_bindings
    ADD INDEX idx_static_bindings_pool (pool_id),
    ADD INDEX idx_static_bindings_ip (ip_address);

ALTER TABLE leases_v4
    ADD INDEX idx_leases_state_updated (state, updated_at DESC),
    ADD INDEX idx_leases_pool_state (pool_id, state, updated_at DESC),
    ADD INDEX idx_leases_expires_state (state, expires_at),
    ADD INDEX idx_leases_hw_active (hardware_addr, state),
    ADD INDEX idx_leases_client_active (client_id, state),
    ADD INDEX idx_leases_cooldown (pool_id, cooldown_until);

ALTER TABLE prefix_leases_v6
    ADD INDEX idx_prefix_leases_state_updated (state, updated_at DESC),
    ADD INDEX idx_prefix_leases_pool_state (pool_id, state, updated_at DESC),
    ADD INDEX idx_prefix_leases_client (client_id, iapd_id, state);


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