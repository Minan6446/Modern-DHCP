-- +goose Up
CREATE TABLE IF NOT EXISTS rbac_roles (
    name            VARCHAR(64) PRIMARY KEY,
    inherits_from   VARCHAR(64) NULL,
    description     TEXT NULL,
    capabilities    JSON NULL,
    created_at      DATETIME NOT NULL,
    updated_at      DATETIME NOT NULL,
    CONSTRAINT fk_rbac_roles_inherits FOREIGN KEY (inherits_from) REFERENCES rbac_roles(name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS rbac_org_units (
    id              VARCHAR(64) PRIMARY KEY,
    tenant_id       VARCHAR(36) NOT NULL,
    parent_id       VARCHAR(64) NULL,
    name            VARCHAR(128) NOT NULL,
    path            VARCHAR(512) NULL,
    created_at      DATETIME NOT NULL,
    updated_at      DATETIME NOT NULL,
    INDEX idx_rbac_org_units_parent (parent_id),
    INDEX idx_rbac_org_units_tenant (tenant_id),
    CONSTRAINT fk_rbac_org_units_parent FOREIGN KEY (parent_id) REFERENCES rbac_org_units(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS rbac_assignments (
    id              VARCHAR(64) PRIMARY KEY,
    principal_id    VARCHAR(128) NOT NULL,
    role_name       VARCHAR(64) NOT NULL,
    tenant_id       VARCHAR(36) NULL,
    org_unit_id     VARCHAR(64) NULL,
    resource_type   VARCHAR(64) NULL,
    resource_id     VARCHAR(64) NULL,
    created_by      VARCHAR(128) NOT NULL,
    created_at      DATETIME NOT NULL,
    expires_at      DATETIME NULL,
    attributes      JSON NULL,
    tenant_scope    VARCHAR(36) AS (COALESCE(tenant_id, '')) STORED,
    org_scope       VARCHAR(64) AS (COALESCE(org_unit_id, '')) STORED,
    resource_scope  VARCHAR(64) AS (COALESCE(resource_type, '')) STORED,
    resource_key    VARCHAR(64) AS (COALESCE(resource_id, '')) STORED,
    UNIQUE KEY uniq_assignment_scope (principal_id, role_name, tenant_scope, org_scope, resource_scope, resource_key),
    INDEX idx_assignment_principal (principal_id),
    INDEX idx_assignment_tenant (tenant_id),
    INDEX idx_assignment_org_unit (org_unit_id),
    CONSTRAINT fk_rbac_assignments_role FOREIGN KEY (role_name) REFERENCES rbac_roles(name),
    CONSTRAINT fk_rbac_assignments_org_unit FOREIGN KEY (org_unit_id) REFERENCES rbac_org_units(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS rbac_temp_grants (
    id              VARCHAR(64) PRIMARY KEY,
    assignment_id   VARCHAR(64) NOT NULL,
    requested_by    VARCHAR(128) NOT NULL,
    reason          TEXT NULL,
    status          VARCHAR(16) NOT NULL DEFAULT 'pending',
    expires_at      DATETIME NOT NULL,
    approved_by     VARCHAR(128) NULL,
    approved_at     DATETIME NULL,
    created_at      DATETIME NOT NULL,
    updated_at      DATETIME NOT NULL,
    INDEX idx_temp_grant_assignment (assignment_id),
    INDEX idx_temp_grant_status (status),
    INDEX idx_temp_grant_expiry (expires_at),
    CONSTRAINT fk_rbac_temp_grants_assignment FOREIGN KEY (assignment_id) REFERENCES rbac_assignments(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS rbac_approval_rules (
    id              VARCHAR(64) PRIMARY KEY,
    role_name       VARCHAR(64) NOT NULL,
    scope           VARCHAR(32) NOT NULL,
    min_approvers   INT NOT NULL DEFAULT 1,
    approver_role   VARCHAR(64) NOT NULL,
    created_at      DATETIME NOT NULL,
    updated_at      DATETIME NOT NULL,
    INDEX idx_approval_rules_role (role_name, scope),
    CONSTRAINT fk_rbac_approval_rules_role FOREIGN KEY (role_name) REFERENCES rbac_roles(name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- +goose Down
DROP TABLE IF EXISTS rbac_approval_rules;
DROP TABLE IF EXISTS rbac_temp_grants;
DROP TABLE IF EXISTS rbac_assignments;
DROP TABLE IF EXISTS rbac_org_units;
DROP TABLE IF EXISTS rbac_roles;
