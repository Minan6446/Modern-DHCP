-- +goose Up
CREATE TABLE IF NOT EXISTS collab_sessions (
    id             VARCHAR(64) PRIMARY KEY,
    tenant_id      VARCHAR(36) NOT NULL,
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
    UNIQUE KEY uniq_session_actor (tenant_id, resource_type, resource_id, user_id),
    INDEX idx_session_tenant_resource (tenant_id, resource_type, resource_id),
    INDEX idx_session_expires (expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS collab_locks (
    id             VARCHAR(64) PRIMARY KEY,
    tenant_id      VARCHAR(36) NOT NULL,
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
    UNIQUE KEY uniq_lock_resource (tenant_id, resource_type, resource_id),
    INDEX idx_lock_session (session_id),
    INDEX idx_lock_expires (expires_at),
    CONSTRAINT fk_collab_locks_session FOREIGN KEY (session_id) REFERENCES collab_sessions(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS collab_comments (
    id             VARCHAR(64) PRIMARY KEY,
    tenant_id      VARCHAR(36) NOT NULL,
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
    INDEX idx_comment_resource (tenant_id, resource_type, resource_id),
    INDEX idx_comment_parent (parent_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS collab_tasks (
    id             VARCHAR(64) PRIMARY KEY,
    tenant_id      VARCHAR(36) NOT NULL,
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
    INDEX idx_task_resource (tenant_id, resource_type, resource_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS collab_approvals (
    id             VARCHAR(64) PRIMARY KEY,
    tenant_id      VARCHAR(36) NOT NULL,
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
    tenant_id      VARCHAR(36) NOT NULL,
    session_id     VARCHAR(64) NULL,
    resource_type  VARCHAR(64) NULL,
    resource_id    VARCHAR(64) NULL,
    event_type     VARCHAR(64) NOT NULL,
    payload        JSON NULL,
    created_at     DATETIME NOT NULL,
    INDEX idx_event_tenant_time (tenant_id, created_at),
    INDEX idx_event_session (session_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- +goose Down
DROP TABLE IF EXISTS collab_events;
DROP TABLE IF EXISTS collab_approvals;
DROP TABLE IF EXISTS collab_tasks;
DROP TABLE IF EXISTS collab_comments;
DROP TABLE IF EXISTS collab_locks;
DROP TABLE IF EXISTS collab_sessions;
