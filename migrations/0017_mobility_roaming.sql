-- +goose Up
ALTER TABLE lease_profiles
    ADD COLUMN mobility_grace_period BIGINT NOT NULL DEFAULT 0 AFTER sleepy_hold_duration;

ALTER TABLE leases_v4
    ADD COLUMN mobility_anchor_id VARCHAR(128) NOT NULL DEFAULT '' AFTER user_id,
    ADD COLUMN last_access_point_id VARCHAR(128) NOT NULL DEFAULT '' AFTER device_type,
    ADD COLUMN last_controller_id VARCHAR(128) NOT NULL DEFAULT '' AFTER last_access_point_id,
    ADD COLUMN last_geo_zone VARCHAR(128) NOT NULL DEFAULT '' AFTER last_controller_id,
    ADD COLUMN mobility_location_hint VARCHAR(255) NOT NULL DEFAULT '' AFTER last_geo_zone,
    ADD COLUMN session_continuity JSON NULL AFTER conflict_history;

-- +goose Down
ALTER TABLE leases_v4
    DROP COLUMN session_continuity,
    DROP COLUMN mobility_location_hint,
    DROP COLUMN last_geo_zone,
    DROP COLUMN last_controller_id,
    DROP COLUMN last_access_point_id,
    DROP COLUMN mobility_anchor_id;

ALTER TABLE lease_profiles
    DROP COLUMN mobility_grace_period;
