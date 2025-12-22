-- +goose Up
CREATE TABLE IF NOT EXISTS automation_jobs (
    id              VARCHAR(64) NOT NULL PRIMARY KEY,
    tenant_id       VARCHAR(64) NOT NULL,
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
    INDEX idx_automation_jobs_tenant (tenant_id),
    INDEX idx_automation_jobs_type_status (job_type, status),
    INDEX idx_automation_jobs_updated (updated_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- +goose Down
DROP TABLE IF EXISTS automation_jobs;
