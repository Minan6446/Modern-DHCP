package models

import "time"

// PoolUsageDaily stores daily utilization snapshots per pool.
type PoolUsageDaily struct {
	TenantID  string    `db:"tenant_id" json:"tenantId"`
	PoolID    string    `db:"pool_id" json:"poolId"`
	Day       time.Time `db:"day" json:"day"`
	Used      int64     `db:"used" json:"used"`
	Capacity  int64     `db:"capacity" json:"capacity"`
	CreatedAt time.Time `db:"created_at" json:"createdAt"`
	UpdatedAt time.Time `db:"updated_at" json:"updatedAt"`
}
