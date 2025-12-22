-- +goose Up
ALTER TABLE lease_profiles
    ADD COLUMN renewal_time BIGINT NOT NULL DEFAULT 0 AFTER max_duration,
    ADD COLUMN rebinding_time BIGINT NOT NULL DEFAULT 0 AFTER renewal_time,
    ADD COLUMN infinite TINYINT(1) NOT NULL DEFAULT 0 AFTER notification_lead,
    ADD COLUMN sleepy_capable TINYINT(1) NOT NULL DEFAULT 0 AFTER device_type,
    ADD COLUMN sleepy_offline_window BIGINT NOT NULL DEFAULT 0 AFTER sleepy_capable,
    ADD COLUMN sleepy_hold_duration BIGINT NOT NULL DEFAULT 0 AFTER sleepy_offline_window;

-- +goose Down
ALTER TABLE lease_profiles
    DROP COLUMN sleepy_hold_duration,
    DROP COLUMN sleepy_offline_window,
    DROP COLUMN sleepy_capable,
    DROP COLUMN infinite,
    DROP COLUMN rebinding_time,
    DROP COLUMN renewal_time;
