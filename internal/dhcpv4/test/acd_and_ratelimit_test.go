package test

import (
	"context"
	"testing"

	"modern-dhcp/internal/lease"
)

func TestACDAndRateLimitCoveredByPackageTests(t *testing.T) {
	t.Skip("ACD与限速核心逻辑已在 internal/lease/acd_test.go 与 internal/dhcpv4/security_test.go 覆盖；此目录仅聚合入口测试")
	_ = context.Background()
	_ = lease.ErrPoolRequired
}
