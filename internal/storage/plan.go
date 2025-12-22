package storage

import (
	"fmt"
	"strings"

	"modern-dhcp/internal/config"
)

const (
	DriverMySQL    = "mysql"
	DriverPostgres = "postgres"
)

// RelationalPlan captures the resolved relational backend for the runtime.
type RelationalPlan struct {
	Driver       string
	Source       string
	DSN          string
	FailoverHint string
}

// DeriveRelationalPlan selects the active relational backend based on the storage matrix.
func DeriveRelationalPlan(cfg config.StorageMatrixConfig) (RelationalPlan, error) {
	plan := RelationalPlan{}
	primary := strings.ToLower(strings.TrimSpace(cfg.Relational.Primary))
	failover := strings.ToLower(strings.TrimSpace(cfg.Relational.Failover))
	if primary == "" {
		primary = DriverMySQL
	}
	plan.FailoverHint = failover

	switch primary {
	case DriverMySQL:
		plan.Driver = DriverMySQL
		plan.Source = "mysql"
		plan.DSN = strings.TrimSpace(cfg.Relational.MySQL.DSN)
	case "postgres", "postgresql":
		plan.Driver = DriverPostgres
		plan.Source = "postgres"
		plan.DSN = strings.TrimSpace(cfg.Relational.Postgres.DSN)
	case "rds", "aurora", "aws-rds":
		plan.Driver = DriverMySQL
		plan.Source = "rds"
		if len(cfg.Relational.RDS.Endpoints) > 0 {
			plan.DSN = cfg.Relational.RDS.Endpoints[0]
		}
	default:
		return plan, fmt.Errorf("storage: unsupported primary backend %q", primary)
	}

	if plan.DSN == "" {
		return plan, fmt.Errorf("storage: missing DSN for backend %s", plan.Source)
	}

	return plan, nil
}
