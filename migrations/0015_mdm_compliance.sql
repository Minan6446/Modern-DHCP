-- +goose Up
ALTER TABLE leases_v4
    ADD COLUMN mdm_managed TINYINT(1) NOT NULL DEFAULT 0 AFTER mobility_location_hint,
    ADD COLUMN mdm_source VARCHAR(64) NULL AFTER mdm_managed,
    ADD COLUMN mdm_tags JSON NULL AFTER mdm_source,
    ADD COLUMN mdm_observed_at DATETIME NULL AFTER mdm_tags;

ALTER TABLE leases_v6
    ADD COLUMN mdm_managed TINYINT(1) NOT NULL DEFAULT 0 AFTER mobility_location_hint,
    ADD COLUMN mdm_source VARCHAR(64) NULL AFTER mdm_managed,
    ADD COLUMN mdm_tags JSON NULL AFTER mdm_source,
    ADD COLUMN mdm_observed_at DATETIME NULL AFTER mdm_tags;

-- +goose Down
ALTER TABLE leases_v6
    DROP COLUMN mdm_observed_at,
    DROP COLUMN mdm_tags,
    DROP COLUMN mdm_source,
    DROP COLUMN mdm_managed;

ALTER TABLE leases_v4
    DROP COLUMN mdm_observed_at,
    DROP COLUMN mdm_tags,
    DROP COLUMN mdm_source,
    DROP COLUMN mdm_managed;
