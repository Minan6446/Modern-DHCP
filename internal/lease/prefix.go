package lease

import (
	"context"
	"errors"
	"time"

	"modern-dhcp/pkg/models"
)

// ReleasePrefix releases a delegated prefix for a client.
func (s *Service) ReleasePrefix(ctx context.Context, scope ResourceScope, clientID string, iapdID uint32) error {
	if _, err := s.tenantFromScope(scope); err != nil {
		return err
	}
	if err := s.ensurePrimaryWrite("lease.prefix.release"); err != nil {
		return err
	}
	return s.updatePrefixState(ctx, scope, clientID, iapdID, leaseStateReleased)
}

// DeclinePrefix marks a delegated prefix as declined/conflicted.
func (s *Service) DeclinePrefix(ctx context.Context, scope ResourceScope, clientID string, iapdID uint32) error {
	if _, err := s.tenantFromScope(scope); err != nil {
		return err
	}
	if err := s.ensurePrimaryWrite("lease.prefix.decline"); err != nil {
		return err
	}
	return s.updatePrefixState(ctx, scope, clientID, iapdID, leaseStateDeclined)
}

func (s *Service) updatePrefixState(ctx context.Context, scope ResourceScope, clientID string, iapdID uint32, state string) error {
	if _, err := scope.TenantIDOrErr(); err != nil {
		return err
	}
	lease, err := s.repo.GetActivePrefixLease(ctx, scope.AccessScope(), clientID, iapdID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		return err
	}
	now := time.Now().UTC()
	lease.State = state
	lease.UpdatedAt = now
	lease.ExpiresAt = now
	return s.repo.UpdatePrefixLease(ctx, lease)
}

// ReleasePrefixByID releases a delegated prefix via its lease ID.
func (s *Service) ReleasePrefixByID(ctx context.Context, scope ResourceScope, prefixLeaseID string) (*models.PrefixLease, bool, error) {
	if _, err := s.tenantFromScope(scope); err != nil {
		return nil, false, err
	}
	return s.updatePrefixStateByID(ctx, scope, prefixLeaseID, leaseStateReleased)
}

// DeclinePrefixByID marks a delegated prefix as declined via its lease ID.
func (s *Service) DeclinePrefixByID(ctx context.Context, scope ResourceScope, prefixLeaseID string) (*models.PrefixLease, bool, error) {
	if _, err := s.tenantFromScope(scope); err != nil {
		return nil, false, err
	}
	return s.updatePrefixStateByID(ctx, scope, prefixLeaseID, leaseStateDeclined)
}

// ListPrefixLeases exposes prefix delegation search for HTTP handlers.
func (s *Service) ListPrefixLeases(ctx context.Context, scope ResourceScope, state string, limit, offset int) ([]models.PrefixLease, error) {
	if _, err := s.tenantFromScope(scope); err != nil {
		return nil, err
	}
	return s.repo.ListPrefixLeases(ctx, scope.AccessScope(), state, limit, offset)
}

func (s *Service) updatePrefixStateByID(ctx context.Context, scope ResourceScope, prefixLeaseID string, state string) (*models.PrefixLease, bool, error) {
	if _, err := scope.TenantIDOrErr(); err != nil {
		return nil, false, err
	}
	lease, err := s.repo.GetPrefixLeaseByID(ctx, scope.AccessScope(), prefixLeaseID)
	if err != nil {
		return nil, false, err
	}
	if lease.State == state {
		return lease, false, nil
	}
	now := time.Now().UTC()
	lease.State = state
	lease.UpdatedAt = now
	lease.ExpiresAt = now
	if err := s.repo.UpdatePrefixLease(ctx, lease); err != nil {
		return nil, false, err
	}
	return lease, true, nil
}
