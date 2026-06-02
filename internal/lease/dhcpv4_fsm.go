package lease

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"modern-dhcp/internal/dhcpv4/leasefsm"
	"modern-dhcp/pkg/models"
)

const dhcpv4LeaseFSMStateKey = "dhcpv4LeaseState"

// UpdateDHCPv4LeaseFSMByLeaseID validates and persists a DHCPv4 FSM transition for a lease ID.
func (s *Service) UpdateDHCPv4LeaseFSMByLeaseID(ctx context.Context, scope ResourceScope, leaseID string, target leasefsm.State) error {
	if _, err := s.tenantFromScope(scope); err != nil {
		return err
	}
	if err := s.ensurePrimaryWrite("lease.fsm.update_by_lease_id"); err != nil {
		return err
	}
	leaseRecord, err := s.repo.GetLeaseByID(ctx, scope.AccessScope(), strings.TrimSpace(leaseID))
	if err != nil {
		return err
	}
	updated, err := s.dhcpv4TransitionLeaseState(leaseRecord, target)
	if err != nil {
		return err
	}
	if !updated {
		return nil
	}
	return s.repo.UpdateLease(ctx, leaseRecord)
}

// UpdateDHCPv4LeaseFSMByIdentifier validates and persists a DHCPv4 FSM transition for the active lease matched by identifier.
func (s *Service) UpdateDHCPv4LeaseFSMByIdentifier(ctx context.Context, scope ResourceScope, identifier string, target leasefsm.State) error {
	if _, err := s.tenantFromScope(scope); err != nil {
		return err
	}
	if err := s.ensurePrimaryWrite("lease.fsm.update_by_identifier"); err != nil {
		return err
	}
	leaseRecord, err := s.repo.GetActiveLease(ctx, scope.AccessScope(), strings.TrimSpace(identifier))
	if err != nil {
		if err == ErrNotFound {
			return nil
		}
		return err
	}
	updated, err := s.dhcpv4TransitionLeaseState(leaseRecord, target)
	if err != nil {
		return err
	}
	if !updated {
		return nil
	}
	return s.repo.UpdateLease(ctx, leaseRecord)
}

// ReleaseByIdentifier releases an active lease immediately so its IP can be re-allocated.
func (s *Service) ReleaseByIdentifier(ctx context.Context, scope ResourceScope, identifier string) (*models.Lease, bool, error) {
	if _, err := s.tenantFromScope(scope); err != nil {
		return nil, false, err
	}
	if err := s.ensurePrimaryWrite("lease.release_by_identifier"); err != nil {
		return nil, false, err
	}
	leaseRecord, err := s.repo.GetActiveLease(ctx, scope.AccessScope(), strings.TrimSpace(identifier))
	if err != nil {
		if err == ErrNotFound {
			return nil, false, nil
		}
		return nil, false, err
	}
	changed, err := s.dhcpv4TransitionLeaseState(leaseRecord, leasefsm.StateReleased)
	if err != nil {
		return nil, false, err
	}
	if !changed {
		return leaseRecord, false, nil
	}
	if err := s.repo.UpdateLease(ctx, leaseRecord); err != nil {
		return nil, false, err
	}
	if poolObj, pErr := s.ensurePool(ctx, scope, leaseRecord.PoolID); pErr == nil {
		_ = s.bitmapSetByIP(ctx, poolObj, leaseRecord.IPAddress, true)
	}
	return leaseRecord, true, nil
}

// ReconcileDHCPv4LeaseFSMOnStartup compares MySQL lease records with Redis cache and repairs inconsistent or invalid FSM state values.
func (s *Service) ReconcileDHCPv4LeaseFSMOnStartup(ctx context.Context, scope ResourceScope, batchSize int) (int, error) {
	if _, err := s.tenantFromScope(scope); err != nil {
		return 0, err
	}
	repo, ok := s.repo.(*MySQLRepository)
	if !ok || repo.cache == nil {
		return 0, nil
	}
	if batchSize <= 0 || batchSize > 1000 {
		batchSize = 200
	}
	repaired := 0
	offset := 0
	for {
		leases, err := repo.ListLeases(ctx, scope.AccessScope(), "", batchSize, offset)
		if err != nil {
			return repaired, err
		}
		for i := range leases {
			leaseRecord := leases[i]
			recordChanged, err := s.dhcpv4EnsureValidStateRecord(&leaseRecord)
			if err != nil {
				return repaired, err
			}
			if recordChanged {
				if err := repo.UpdateLease(ctx, &leaseRecord); err != nil {
					return repaired, err
				}
				repaired++
			}
			cacheChanged, err := dhcpv4ReconcileLeaseCache(ctx, repo, scope, &leaseRecord)
			if err != nil {
				return repaired, err
			}
			repaired += cacheChanged
		}
		if len(leases) < batchSize {
			break
		}
		offset += len(leases)
	}
	return repaired, nil
}

func (s *Service) dhcpv4TransitionLeaseState(leaseRecord *models.Lease, target leasefsm.State) (bool, error) {
	if leaseRecord == nil {
		return false, nil
	}
	if !leasefsm.IsKnownState(target) {
		return false, fmt.Errorf("dhcpv4 lease fsm: unknown target state %q", target)
	}
	current := dhcpv4LeaseFSMStateFromLease(leaseRecord)
	if target == leasefsm.StateOffered && current != leasefsm.StateInit {
		current = leasefsm.StateInit
	}
	if err := leasefsm.ValidateTransition(current, target); err != nil {
		return false, err
	}
	if current == target && strings.EqualFold(leaseRecord.State, leaseStateReleased) == (target == leasefsm.StateReleased) {
		return false, nil
	}
	now := time.Now().UTC()
	continuity, _ := dhcpv4LeaseContinuityMap(leaseRecord.SessionContinuity)
	continuity[dhcpv4LeaseFSMStateKey] = string(target)
	payload, err := json.Marshal(continuity)
	if err != nil {
		return false, err
	}
	leaseRecord.SessionContinuity = payload
	leaseRecord.UpdatedAt = now
	if target == leasefsm.StateReleased {
		leaseRecord.State = leaseStateReleased
		leaseRecord.ExpiresAt = now
		leaseRecord.CooldownUntil = nil
	} else if strings.EqualFold(leaseRecord.State, leaseStateReleased) {
		leaseRecord.State = leaseStateActive
	}
	return true, nil
}

func (s *Service) dhcpv4EnsureValidStateRecord(leaseRecord *models.Lease) (bool, error) {
	if leaseRecord == nil {
		return false, nil
	}
	continuity, original := dhcpv4LeaseContinuityMap(leaseRecord.SessionContinuity)
	raw, _ := continuity[dhcpv4LeaseFSMStateKey].(string)
	state := leasefsm.State(strings.TrimSpace(raw))
	if !leasefsm.IsKnownState(state) {
		state = dhcpv4DeriveFallbackState(leaseRecord)
		continuity[dhcpv4LeaseFSMStateKey] = string(state)
	}
	payload, err := json.Marshal(continuity)
	if err != nil {
		return false, err
	}
	if bytes.Equal(original, payload) {
		return false, nil
	}
	leaseRecord.SessionContinuity = payload
	leaseRecord.UpdatedAt = time.Now().UTC()
	return true, nil
}

func dhcpv4ReconcileLeaseCache(ctx context.Context, repo *MySQLRepository, scope ResourceScope, leaseRecord *models.Lease) (int, error) {
	if repo == nil || repo.cache == nil || leaseRecord == nil {
		return 0, nil
	}
	tenantID := strings.TrimSpace(leaseRecord.TenantID)
	if tenantID == "" {
		tenantID = strings.TrimSpace(scope.TenantOrDefault())
	}
	changed := 0
	cacheLeaseKey := repo.cacheKeyLease(tenantID, leaseRecord.ID)
	updated, err := dhcpv4ReconcileLeaseCacheKey(ctx, repo, cacheLeaseKey, leaseRecord)
	if err != nil {
		return changed, err
	}
	changed += updated
	identifiers := make([]string, 0, 2)
	if id := strings.TrimSpace(leaseRecord.HardwareAddr); id != "" {
		identifiers = append(identifiers, id)
	}
	if id := strings.TrimSpace(leaseRecord.ClientID); id != "" && id != strings.TrimSpace(leaseRecord.HardwareAddr) {
		identifiers = append(identifiers, id)
	}
	for _, identifier := range identifiers {
		activeKey := repo.cacheKeyActiveLease(tenantID, identifier)
		if strings.EqualFold(strings.TrimSpace(leaseRecord.State), leaseStateActive) {
			updated, err = dhcpv4ReconcileLeaseCacheKey(ctx, repo, activeKey, leaseRecord)
			if err != nil {
				return changed, err
			}
			changed += updated
			continue
		}
		if err := repo.cache.Delete(ctx, activeKey); err != nil {
			return changed, err
		}
	}
	return changed, nil
}

func dhcpv4ReconcileLeaseCacheKey(ctx context.Context, repo *MySQLRepository, key string, leaseRecord *models.Lease) (int, error) {
	if repo == nil || repo.cache == nil || strings.TrimSpace(key) == "" {
		return 0, nil
	}
	var cached models.Lease
	ok, err := repo.cache.GetJSON(ctx, key, &cached)
	if err != nil {
		return 0, err
	}
	if ok && dhcpv4LeaseCacheEquivalent(&cached, leaseRecord) {
		return 0, nil
	}
	if err := repo.cache.SetJSON(ctx, key, leaseRecord, repo.cacheTTL); err != nil {
		return 0, err
	}
	return 1, nil
}

func dhcpv4LeaseCacheEquivalent(cached, source *models.Lease) bool {
	if cached == nil || source == nil {
		return false
	}
	if cached.ID != source.ID {
		return false
	}
	if cached.State != source.State {
		return false
	}
	if !bytes.Equal(cached.SessionContinuity, source.SessionContinuity) {
		return false
	}
	return true
}

func dhcpv4LeaseFSMStateFromLease(leaseRecord *models.Lease) leasefsm.State {
	if leaseRecord == nil {
		return leasefsm.StateInit
	}
	continuity, _ := dhcpv4LeaseContinuityMap(leaseRecord.SessionContinuity)
	raw, _ := continuity[dhcpv4LeaseFSMStateKey].(string)
	state := leasefsm.State(strings.TrimSpace(raw))
	if leasefsm.IsKnownState(state) {
		return state
	}
	return dhcpv4DeriveFallbackState(leaseRecord)
}

func dhcpv4DeriveFallbackState(leaseRecord *models.Lease) leasefsm.State {
	if leaseRecord == nil {
		return leasefsm.StateInit
	}
	if strings.EqualFold(strings.TrimSpace(leaseRecord.State), leaseStateReleased) {
		return leasefsm.StateReleased
	}
	if !leaseRecord.ExpiresAt.IsZero() && leaseRecord.ExpiresAt.Before(time.Now().UTC()) {
		return leasefsm.StateExpired
	}
	return leasefsm.StateBound
}

func dhcpv4LeaseContinuityMap(payload []byte) (map[string]any, []byte) {
	original := append([]byte(nil), payload...)
	if len(payload) == 0 {
		return make(map[string]any), original
	}
	decoded := make(map[string]any)
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return make(map[string]any), original
	}
	return decoded, original
}

func (s *Service) dhcpv4LogFSMRepair(count int) {
	if count <= 0 || s.logger == nil {
		return
	}
	s.logger.Info("dhcpv4 lease fsm startup reconciliation completed", zap.Int("repaired", count))
}
