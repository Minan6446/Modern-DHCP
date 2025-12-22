-- +goose Up
ALTER TABLE tenant_quotas
    ADD COLUMN api_request_limit INT NOT NULL DEFAULT 0 AFTER client_limit,
    ADD COLUMN automation_job_limit INT NOT NULL DEFAULT 0 AFTER api_request_limit;

-- +goose Down
ALTER TABLE tenant_quotas
    DROP COLUMN automation_job_limit,
    DROP COLUMN api_request_limit;
