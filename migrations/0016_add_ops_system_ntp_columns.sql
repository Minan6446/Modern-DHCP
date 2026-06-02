ALTER TABLE ops_system_settings
    ADD COLUMN ntp_enabled TINYINT(1) NOT NULL DEFAULT 1 AFTER announcement,
    ADD COLUMN ntp_servers TEXT NULL AFTER ntp_enabled,
    ADD COLUMN ntp_interval_minutes INT NOT NULL DEFAULT 30 AFTER ntp_servers,
    ADD COLUMN ntp_timeout_seconds INT NOT NULL DEFAULT 5 AFTER ntp_interval_minutes,
    ADD COLUMN timezone VARCHAR(64) NOT NULL DEFAULT 'Asia/Shanghai' AFTER ntp_timeout_seconds,
    ADD COLUMN ntp_sync_status VARCHAR(16) NOT NULL DEFAULT 'idle' AFTER timezone,
    ADD COLUMN ntp_last_sync_at DATETIME NULL AFTER ntp_sync_status;

UPDATE ops_system_settings
SET
    ntp_servers = CASE
        WHEN ntp_servers IS NULL OR TRIM(ntp_servers) = '' THEN 'pool.ntp.org\nntp.aliyun.com\ntime.cloudflare.com'
        ELSE ntp_servers
    END,
    ntp_interval_minutes = CASE WHEN ntp_interval_minutes <= 0 THEN 30 ELSE ntp_interval_minutes END,
    ntp_timeout_seconds = CASE WHEN ntp_timeout_seconds <= 0 THEN 5 ELSE ntp_timeout_seconds END,
    timezone = CASE WHEN timezone IS NULL OR TRIM(timezone) = '' THEN 'Asia/Shanghai' ELSE timezone END,
    ntp_sync_status = CASE WHEN ntp_sync_status IS NULL OR TRIM(ntp_sync_status) = '' THEN 'idle' ELSE ntp_sync_status END
WHERE id = 1;
