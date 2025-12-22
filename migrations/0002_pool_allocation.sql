-- +goose Up
ALTER TABLE address_pools
    ADD COLUMN allocation_mode VARCHAR(32) NOT NULL DEFAULT 'SEQUENTIAL' AFTER exclusions,
    ADD COLUMN priority_weight INT NOT NULL DEFAULT 50 AFTER allocation_mode;

-- ensure legacy rows get sane bounds
UPDATE address_pools SET priority_weight = 50 WHERE priority_weight < 1;

-- +goose Down
ALTER TABLE address_pools
    DROP COLUMN priority_weight,
    DROP COLUMN allocation_mode;
