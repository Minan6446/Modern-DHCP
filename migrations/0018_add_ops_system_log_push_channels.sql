ALTER TABLE ops_system_settings
    ADD COLUMN log_push_channels VARCHAR(256) NOT NULL DEFAULT '' AFTER log_push_min_level,
    ADD COLUMN log_push_phones VARCHAR(512) NOT NULL DEFAULT '' AFTER log_push_channels;

UPDATE ops_system_settings
SET
    log_push_channels = CASE WHEN log_push_channels IS NULL THEN '' ELSE LOWER(TRIM(log_push_channels)) END,
    log_push_phones = CASE WHEN log_push_phones IS NULL THEN '' ELSE TRIM(log_push_phones) END
WHERE id = 1;
