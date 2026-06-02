package lease

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"modern-dhcp/internal/pool"
	"modern-dhcp/pkg/models"
)

// AllocateOrReusePrefixes ensures each IA_PD request gets a persisted prefix lease.
func (s *Service) AllocateOrReusePrefixes(ctx context.Context, scope ResourceScope, clientID string, profile models.LeaseProfile, pool *models.AddressPool, requests []PrefixRequest) ([]PrefixDelegation, error) {
	if len(requests) == 0 || pool == nil {
		return nil, nil
	}
	tenantID, err := s.tenantFromScope(scope)
	if err != nil {
		return nil, err
	}
	poolPrefix, err := netip.ParsePrefix(pool.CIDR)
	if err != nil {
		return nil, ErrUnsupportedCIDR
	}
	if !poolPrefix.Addr().Is6() {
		return nil, ErrUnsupportedCIDR
	}
	access := scope.AccessScope()
	existing, err := s.repo.ListActivePrefixes(ctx, access, pool.ID)
	if err != nil {
		return nil, err
	}
	used := make(map[string]struct{}, len(existing))
	for _, lease := range existing {
		key := fmt.Sprintf("%s/%d", lease.Prefix, lease.PrefixLen)
		used[key] = struct{}{}
	}
	delegations := make([]PrefixDelegation, 0, len(requests))
	for _, req := range requests {
		pd, err := s.allocateOrReusePrefix(ctx, scope, tenantID, clientID, profile, pool, poolPrefix, req, used)
		if err != nil {
			return nil, err
		}
		if pd != nil {
			used[pd.Prefix] = struct{}{}
			delegations = append(delegations, *pd)
		}
	}
	return delegations, nil
}

func (s *Service) allocateOrReusePrefix(ctx context.Context, scope ResourceScope, tenantID, clientID string, profile models.LeaseProfile, pool *models.AddressPool, poolPrefix netip.Prefix, req PrefixRequest, used map[string]struct{}) (*PrefixDelegation, error) {
	if _, err := scope.TenantIDOrErr(); err != nil {
		return nil, err
	}
	existing, err := s.repo.GetActivePrefixLease(ctx, scope.AccessScope(), clientID, req.IAPDID)
	if err == nil {
		s.applyPrefixTiming(existing, profile)
		if err := s.repo.UpdatePrefixLease(ctx, existing); err != nil {
			return nil, err
		}
		pd := s.buildPrefixDelegation(existing, profile, req)
		if notification := s.computePrefixNotification(existing, profile); notification != nil {
			s.enqueueNotificationJob(ctx, NotificationJob{
				LeaseID:   existing.ID,
				TenantID:  existing.TenantID,
				PoolID:    existing.PoolID,
				SendAfter: notification.SendAfter,
				Lead:      notification.Lead,
				Kind:      "prefix",
				Metadata:  map[string]string{"prefix": pd.Prefix},
			})
		}
		return pd, nil
	}
	if errors.Is(err, ErrNotFound) {
		// proceed with new prefix allocation
	} else {
		return nil, err
	}
	length := determinePrefixLength(poolPrefix, req)
	candidate, ok := s.pickAvailablePrefix(poolPrefix, length, req, used)
	if !ok {
		return nil, ErrNoAvailablePrefix
	}
	now := time.Now().UTC()
	lease := &models.PrefixLease{
		ID:        uuid.NewString(),
		TenantID:  tenantID,
		PoolID:    pool.ID,
		ClientID:  clientID,
		IAPDID:    req.IAPDID,
		Prefix:    candidate.Masked().Addr().String(),
		PrefixLen: candidate.Bits(),
		State:     leaseStateActive,
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.applyPrefixTiming(lease, profile)
	if err := s.repo.CreatePrefixLease(ctx, lease); err != nil {
		return nil, err
	}
	pd := s.buildPrefixDelegation(lease, profile, req)
	notification := s.computePrefixNotification(lease, profile)
	if notification != nil {
		s.enqueueNotificationJob(ctx, NotificationJob{
			LeaseID:   lease.ID,
			TenantID:  lease.TenantID,
			PoolID:    lease.PoolID,
			SendAfter: notification.SendAfter,
			Lead:      notification.Lead,
			Kind:      "prefix",
			Metadata:  map[string]string{"prefix": pd.Prefix},
		})
	}
	return pd, nil
}

func determinePrefixLength(poolPrefix netip.Prefix, req PrefixRequest) int {
	length := int(req.PrefixLength)
	if length <= 0 {
		length = poolPrefix.Bits()
	}
	if length < poolPrefix.Bits() {
		length = poolPrefix.Bits()
	}
	if length > 128 {
		length = 128
	}
	return length
}

func (s *Service) pickAvailablePrefix(poolPrefix netip.Prefix, length int, req PrefixRequest, used map[string]struct{}) (netip.Prefix, bool) {
	allocator, err := pool.NewIPv6PrefixPool(poolPrefix, length, used)
	if err != nil {
		return netip.Prefix{}, false
	}
	if addr, ok := netip.AddrFromSlice(req.Prefix); ok {
		candidate := netip.PrefixFrom(addr, length).Masked()
		if allocated, ok := allocator.AllocateAvailablePrefix(candidate); ok {
			return allocated, true
		}
	}
	return allocator.AllocateAvailablePrefix(netip.Prefix{})
}

func (s *Service) applyPrefixTiming(lease *models.PrefixLease, profile models.LeaseProfile) {
	if lease == nil {
		return
	}
	now := time.Now().UTC()
	lease.UpdatedAt = now
	if profile.Infinite {
		lease.ExpiresAt = now.Add(24 * time.Hour * 365 * 100)
		return
	}
	lease.ExpiresAt = now.Add(profile.DefaultDuration)
}

func (s *Service) buildPrefixDelegation(lease *models.PrefixLease, profile models.LeaseProfile, req PrefixRequest) *PrefixDelegation {
	if lease == nil {
		return nil
	}
	prefixStr := fmt.Sprintf("%s/%d", lease.Prefix, lease.PrefixLen)
	preferred := profile.DefaultDuration
	if req.PreferredLifetime > 0 {
		preferred = time.Duration(req.PreferredLifetime) * time.Second
	}
	if preferred <= 0 {
		preferred = time.Hour
	}
	valid := profile.MaxDuration
	if req.ValidLifetime > 0 {
		valid = time.Duration(req.ValidLifetime) * time.Second
	}
	if valid < preferred {
		valid = preferred * 2
	}
	return &PrefixDelegation{
		IAPDID:            lease.IAPDID,
		Prefix:            prefixStr,
		PrefixLength:      byte(lease.PrefixLen),
		PreferredLifetime: preferred,
		ValidLifetime:     valid,
	}
}

func (s *Service) computePrefixNotification(lease *models.PrefixLease, profile models.LeaseProfile) *LeaseNotification {
	if lease == nil || profile.NotificationLead <= 0 {
		return nil
	}
	notifyAt := lease.ExpiresAt.Add(-profile.NotificationLead)
	now := time.Now().UTC()
	if notifyAt.Before(now) {
		notifyAt = now
	}
	s.logger.Debug("prefix notification scheduled", zap.String("prefix", fmt.Sprintf("%s/%d", lease.Prefix, lease.PrefixLen)), zap.Time("notifyAt", notifyAt))
	return &LeaseNotification{SendAfter: notifyAt, Lead: profile.NotificationLead}
}
