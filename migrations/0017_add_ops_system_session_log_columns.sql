ALTER TABLE ops_system_settings
    ADD COLUMN admin_session_timeout_minutes INT NOT NULL DEFAULT 30 AFTER announcement,
    ADD COLUMN auto_logout_enabled TINYINT(1) NOT NULL DEFAULT 1 AFTER admin_session_timeout_minutes,
    ADD COLUMN system_log_retention_days INT NOT NULL DEFAULT 30 AFTER auto_logout_enabled,
    ADD COLUMN audit_log_retention_days INT NOT NULL DEFAULT 180 AFTER system_log_retention_days,
    ADD COLUMN log_push_enabled TINYINT(1) NOT NULL DEFAULT 0 AFTER audit_log_retention_days,
    ADD COLUMN log_push_endpoint VARCHAR(512) NULL AFTER log_push_enabled,
    ADD COLUMN log_push_min_level VARCHAR(16) NOT NULL DEFAULT 'warning' AFTER log_push_endpoint;

UPDATE ops_system_settings
SET
    admin_session_timeout_minutes = CASE WHEN admin_session_timeout_minutes <= 0 THEN 30 ELSE admin_session_timeout_minutes END,
    auto_logout_enabled = CASE WHEN auto_logout_enabled IS NULL THEN 1 ELSE auto_logout_enabled END,
    system_log_retention_days = CASE WHEN system_log_retention_days <= 0 THEN 30 ELSE system_log_retention_days END,
    audit_log_retention_days = CASE WHEN audit_log_retention_days <= 0 THEN 180 ELSE audit_log_retention_days END,
    log_push_min_level = CASE WHEN log_push_min_level IS NULL OR TRIM(log_push_min_level) = '' THEN 'warning' ELSE LOWER(TRIM(log_push_min_level)) END
WHERE id = 1;
