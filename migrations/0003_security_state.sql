-- +goose Up
ALTER TABLE leases_v4
    ADD COLUMN security_state VARCHAR(16) NOT NULL DEFAULT 'OK' AFTER state;

ALTER TABLE leases_v6
    ADD COLUMN security_state VARCHAR(16) NOT NULL DEFAULT 'OK' AFTER state;

UPDATE leases_v4 SET security_state = 'OK' WHERE security_state = '';
UPDATE leases_v6 SET security_state = 'OK' WHERE security_state = '';

-- +goose Down
ALTER TABLE leases_v4
    DROP COLUMN security_state;

ALTER TABLE leases_v6
    DROP COLUMN security_state;
