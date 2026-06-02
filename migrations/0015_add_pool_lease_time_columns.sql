ALTER TABLE address_pools
    ADD COLUMN min_lease_time INT NOT NULL DEFAULT 3600 AFTER reserve_percent,
    ADD COLUMN max_lease_time INT NOT NULL DEFAULT 7200 AFTER min_lease_time;

UPDATE address_pools ap
LEFT JOIN lease_profiles lp ON lp.id = ap.lease_profile_id
SET
    ap.min_lease_time = CASE
        WHEN COALESCE(lp.default_duration, 0) > 0 THEN lp.default_duration
        ELSE ap.min_lease_time
    END,
    ap.max_lease_time = CASE
        WHEN COALESCE(lp.max_duration, 0) > 0 THEN lp.max_duration
        ELSE ap.max_lease_time
    END;
