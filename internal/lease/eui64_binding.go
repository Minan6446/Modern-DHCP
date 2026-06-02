package lease

import (
	"context"
	"errors"
	"strings"
	"time"
)

const eui64FixedLeaseDuration = 365 * 24 * time.Hour

// IPv6EUI64Binding stores dynamic MAC to IPv6 bindings derived from EUI-64.
type IPv6EUI64Binding struct {
	TenantID  string
	MAC       string
	Prefix    string
	IPv6Addr  string
	LeaseTime time.Duration
	CreatedAt time.Time
	UpdatedAt time.Time
}

// FindIPv6EUI64BindingByMAC returns a persisted EUI-64 binding for the MAC under the prefix.
func (s *Service) FindIPv6EUI64BindingByMAC(ctx context.Context, scope ResourceScope, mac, prefix string) (*IPv6EUI64Binding, error) {
	if s == nil || s.repo == nil {
		return nil, ErrNotFound
	}
	mac = strings.ToLower(strings.TrimSpace(mac))
	prefix = strings.TrimSpace(prefix)
	if mac == "" || prefix == "" {
		return nil, ErrNotFound
	}
	return s.repo.FindIPv6EUI64BindingByMAC(ctx, scope.AccessScope(), mac, prefix)
}

// FindIPv6EUI64BindingByIPv6Addr returns a persisted EUI-64 binding owner for an IPv6 address.
func (s *Service) FindIPv6EUI64BindingByIPv6Addr(ctx context.Context, scope ResourceScope, prefix, ipv6Addr string) (*IPv6EUI64Binding, error) {
	if s == nil || s.repo == nil {
		return nil, ErrNotFound
	}
	prefix = strings.TrimSpace(prefix)
	ipv6Addr = strings.ToLower(strings.TrimSpace(ipv6Addr))
	if prefix == "" || ipv6Addr == "" {
		return nil, ErrNotFound
	}
	return s.repo.FindIPv6EUI64BindingByIPv6Addr(ctx, scope.AccessScope(), prefix, ipv6Addr)
}

// CreateIPv6EUI64Binding persists a dynamic EUI-64 binding with a fixed long lease duration.
func (s *Service) CreateIPv6EUI64Binding(ctx context.Context, scope ResourceScope, binding IPv6EUI64Binding) (*IPv6EUI64Binding, error) {
	if s == nil || s.repo == nil {
		return nil, ErrPoolReaderMissing
	}
	tenantID, err := s.tenantFromScope(scope)
	if err != nil {
		return nil, err
	}
	binding.TenantID = tenantID
	binding.MAC = strings.ToLower(strings.TrimSpace(binding.MAC))
	binding.Prefix = strings.TrimSpace(binding.Prefix)
	binding.IPv6Addr = strings.ToLower(strings.TrimSpace(binding.IPv6Addr))
	if binding.MAC == "" || binding.Prefix == "" || binding.IPv6Addr == "" {
		return nil, ErrPoolRequired
	}
	if binding.LeaseTime <= 0 {
		binding.LeaseTime = eui64FixedLeaseDuration
	}
	now := time.Now().UTC()
	binding.CreatedAt = now
	binding.UpdatedAt = now
	if err := s.repo.CreateIPv6EUI64Binding(ctx, &binding); err != nil {
		return nil, err
	}
	stored, err := s.repo.FindIPv6EUI64BindingByMAC(ctx, scope.AccessScope(), binding.MAC, binding.Prefix)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return &binding, nil
		}
		return nil, err
	}
	return stored, nil
}
