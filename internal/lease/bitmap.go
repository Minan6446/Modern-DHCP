package lease

import (
	"context"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"go.uber.org/zap"

	"modern-dhcp/pkg/models"
)

func (s *Service) bitmapEnabled() bool {
	return s.redisClient != nil
}

func (s *Service) bitmapKey(poolID string) string {
	return fmt.Sprintf("dhcpv4:bitmap:pool:%s", strings.TrimSpace(poolID))
}

func (s *Service) bitmapLockKey(poolID string) string {
	return fmt.Sprintf("dhcpv4:lock:pool:%s", strings.TrimSpace(poolID))
}

func (s *Service) bitmapInitKey(poolID string) string {
	return fmt.Sprintf("dhcpv4:bitmap:init:%s", strings.TrimSpace(poolID))
}

func (s *Service) acquirePoolLock(ctx context.Context, poolID string) (*lockRelease, error) {
	if s.allocLock == nil || strings.TrimSpace(poolID) == "" {
		return &lockRelease{}, nil
	}
	guard, err := s.allocLock.Acquire(ctx, s.bitmapLockKey(poolID), 5*time.Second)
	if err != nil {
		return nil, err
	}
	return &lockRelease{guard: guard}, nil
}

type lockRelease struct {
	guard interface{ Unlock(context.Context) error }
}

func (r *lockRelease) Unlock(ctx context.Context) {
	if r == nil || r.guard == nil {
		return
	}
	_ = r.guard.Unlock(ctx)
}

func (s *Service) ensurePoolBitmapInitialized(ctx context.Context, poolObj *models.AddressPool, start, end netip.Addr) error {
	if !s.bitmapEnabled() || poolObj == nil || !start.IsValid() || !end.IsValid() {
		return nil
	}
	initKey := s.bitmapInitKey(poolObj.ID)
	ok, err := s.redisClient.SetNX(ctx, initKey, "1", 24*time.Hour).Result()
	if err != nil || !ok {
		return err
	}
	count := int(addrToUint32(end)-addrToUint32(start)) + 1
	if count <= 0 {
		return nil
	}
	bytesLen := (count + 7) / 8
	payload := make([]byte, bytesLen)
	for i := range payload {
		payload[i] = 0xff
	}
	if count%8 != 0 {
		mask := byte(0xff << uint(8-(count%8)))
		payload[bytesLen-1] = mask
	}
	return s.redisClient.Set(ctx, s.bitmapKey(poolObj.ID), payload, 0).Err()
}

func (s *Service) bitmapSetAvailability(ctx context.Context, poolID string, idx uint32, available bool) error {
	if !s.bitmapEnabled() {
		return nil
	}
	bit := 0
	if available {
		bit = 1
	}
	return s.redisClient.SetBit(ctx, s.bitmapKey(poolID), int64(idx), bit).Err()
}

func (s *Service) bitmapSetByIP(ctx context.Context, poolObj *models.AddressPool, ip string, available bool) error {
	if !s.bitmapEnabled() || poolObj == nil {
		return nil
	}
	prefix, err := netip.ParsePrefix(poolObj.CIDR)
	if err != nil || !prefix.Addr().Is4() {
		return nil
	}
	start, end := s.resolvePoolBounds(poolObj, prefix)
	if !start.IsValid() || !end.IsValid() {
		return nil
	}
	addr, err := netip.ParseAddr(strings.TrimSpace(ip))
	if err != nil || !addr.Is4() {
		return nil
	}
	v := addrToUint32(addr)
	start32 := addrToUint32(start)
	end32 := addrToUint32(end)
	if v < start32 || v > end32 {
		return nil
	}
	idx := v - start32
	return s.bitmapSetAvailability(ctx, poolObj.ID, idx, available)
}

func (s *Service) bitmapFindAvailableIPv4(ctx context.Context, poolObj *models.AddressPool, start, end netip.Addr, usedSet, cooldownSet map[string]struct{}, reservePercent int, exclusions []ipv4Range) (string, bool) {
	if !s.bitmapEnabled() || poolObj == nil || !start.IsValid() || !end.IsValid() {
		return "", false
	}
	if err := s.ensurePoolBitmapInitialized(ctx, poolObj, start, end); err != nil {
		if s.logger != nil {
			s.logger.Debug("bitmap init failed", zap.String("pool", poolObj.ID), zap.Error(err))
		}
		return "", false
	}
	start32 := addrToUint32(start)
	end32 := addrToUint32(end)
	allocEnd, ok := applyReserveWindow(start32, end32, reservePercent)
	if !ok {
		return "", false
	}
	maxOffset := int64(allocEnd - start32)
	for offset := int64(0); offset <= maxOffset; {
		pos, err := s.redisClient.BitPos(ctx, s.bitmapKey(poolObj.ID), 1, offset).Result()
		if err != nil || pos < 0 {
			return "", false
		}
		if pos > maxOffset {
			return "", false
		}
		candidate32 := start32 + uint32(pos)
		candidate := uint32ToAddr(candidate32).String()
		if _, used := usedSet[candidate]; used {
			_ = s.bitmapSetAvailability(ctx, poolObj.ID, uint32(pos), false)
			offset = pos + 1
			continue
		}
		if _, blocked := cooldownSet[candidate]; blocked {
			_ = s.bitmapSetAvailability(ctx, poolObj.ID, uint32(pos), false)
			offset = pos + 1
			continue
		}
		addr := uint32ToAddr(candidate32)
		if isExcluded(addr, exclusions, nil) {
			_ = s.bitmapSetAvailability(ctx, poolObj.ID, uint32(pos), false)
			offset = pos + 1
			continue
		}
		if err := s.bitmapSetAvailability(ctx, poolObj.ID, uint32(pos), false); err != nil {
			if s.logger != nil {
				s.logger.Debug("bitmap reserve failed", zap.String("pool", poolObj.ID), zap.String("ip", candidate), zap.Error(err))
			}
			return "", false
		}
		return candidate, true
	}
	return "", false
}

// SyncPoolBitmapFromMySQL rebuilds one pool bitmap according to MySQL active/cooldown leases.
func (s *Service) SyncPoolBitmapFromMySQL(ctx context.Context, scope ResourceScope, poolObj models.AddressPool) (int, error) {
	if !s.bitmapEnabled() {
		return 0, nil
	}
	prefix, err := netip.ParsePrefix(poolObj.CIDR)
	if err != nil || !prefix.Addr().Is4() {
		return 0, nil
	}
	start, end := s.resolvePoolBounds(&poolObj, prefix)
	if !start.IsValid() || !end.IsValid() {
		return 0, nil
	}
	count := int(addrToUint32(end)-addrToUint32(start)) + 1
	if count <= 0 {
		return 0, nil
	}
	bytesLen := (count + 7) / 8
	payload := make([]byte, bytesLen)
	for i := range payload {
		payload[i] = 0xff
	}
	if count%8 != 0 {
		mask := byte(0xff << uint(8-(count%8)))
		payload[bytesLen-1] = mask
	}
	markUsed := func(ip string) {
		addr, err := netip.ParseAddr(strings.TrimSpace(ip))
		if err != nil || !addr.Is4() {
			return
		}
		value := addrToUint32(addr)
		if value < addrToUint32(start) || value > addrToUint32(end) {
			return
		}
		idx := value - addrToUint32(start)
		byteIdx := idx / 8
		bitMask := byte(1 << (7 - (idx % 8)))
		payload[byteIdx] &^= bitMask
	}
	activeIPs, err := s.repo.ListActiveIPs(ctx, scope.AccessScope(), poolObj.ID)
	if err != nil {
		return 0, err
	}
	for _, ip := range activeIPs {
		markUsed(ip)
	}
	cooldownIPs, err := s.repo.ListCooldownIPs(ctx, scope.AccessScope(), poolObj.ID, time.Now().UTC())
	if err == nil {
		for _, ip := range cooldownIPs {
			markUsed(ip)
		}
	}
	if err := s.redisClient.Set(ctx, s.bitmapKey(poolObj.ID), payload, 0).Err(); err != nil {
		return 0, err
	}
	return len(activeIPs) + len(cooldownIPs), nil
}
