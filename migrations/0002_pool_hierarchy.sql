-- +goose Up
ALTER TABLE address_pools
    ADD COLUMN scope VARCHAR(16) NOT NULL DEFAULT 'GLOBAL' AFTER tenant_id,
    ADD COLUMN network VARCHAR(64) NULL AFTER cidr,
    ADD COLUMN netmask VARCHAR(64) NULL AFTER network,
    ADD COLUMN range_start VARCHAR(64) NULL AFTER netmask,
    ADD COLUMN range_end VARCHAR(64) NULL AFTER range_start,
    ADD COLUMN exclusions JSON NULL AFTER range_end;

UPDATE address_pools SET scope = COALESCE(scope, 'GLOBAL');

-- +goose Down
ALTER TABLE address_pools
    DROP COLUMN exclusions,
    DROP COLUMN range_end,
    DROP COLUMN range_start,
    DROP COLUMN netmask,
    DROP COLUMN network,
    DROP COLUMN scope;
