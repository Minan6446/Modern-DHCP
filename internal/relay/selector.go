package relay

import (
	"strconv"
	"strings"

	"go.uber.org/zap"

	"modern-dhcp/internal/config"
)

// Selector evaluates relay metadata to choose pools without hitting policy rules.
type Selector struct {
	cfg             config.RelayProxyConfig
	vlanPools       map[int]string
	regionFallbacks map[string][]string
	logger          *zap.Logger
}

// NewSelector constructs a Selector from configuration.
func NewSelector(cfg config.RelayProxyConfig, logger *zap.Logger) *Selector {
	sel := &Selector{
		cfg:             cfg,
		vlanPools:       buildVLANPool(cfg.CrossDomain.MultiVLANPools),
		regionFallbacks: normalizeRegionFallbacks(cfg.CrossDomain.RegionFallbacks),
		logger:          logger,
	}
	if sel.logger == nil {
		sel.logger = zap.NewNop()
	}
	return sel
}

// Resolve returns the pool ID chosen for the metadata.
func (s *Selector) Resolve(meta Metadata) (string, bool) {
	if !s.cfg.Enabled {
		return "", false
	}
	if pool := s.matchGiaddr(meta); pool != "" {
		s.logger.Debug("relay selector matched giaddr", zap.String("poolId", pool), zap.String("giaddr", meta.GIAddr), zap.String("tenantId", meta.TenantID))
		return pool, true
	}
	if pool := s.matchCrossDomain(meta); pool != "" {
		s.logger.Debug("relay selector matched cross-domain", zap.String("poolId", pool), zap.String("relayId", meta.RelayID))
		return pool, true
	}
	if pool := s.matchRules(meta); pool != "" {
		s.logger.Debug("relay selector matched rule", zap.String("poolId", pool), zap.String("relayId", meta.RelayID))
		return pool, true
	}
	if pool := s.regionFallback(meta); pool != "" {
		s.logger.Debug("relay selector matched region fallback", zap.String("poolId", pool), zap.String("region", meta.Region))
		return pool, true
	}
	if s.cfg.PoolSelection.DefaultPool != "" {
		return s.cfg.PoolSelection.DefaultPool, true
	}
	return "", false
}

func (s *Selector) matchGiaddr(meta Metadata) string {
	if meta.GIAddr == "" {
		return ""
	}
	for _, rule := range s.cfg.PoolSelection.GiaddrRules {
		if strings.TrimSpace(rule.GIAddr) == "" || strings.TrimSpace(rule.PoolID) == "" {
			continue
		}
		if !strings.EqualFold(rule.GIAddr, meta.GIAddr) {
			continue
		}
		if strings.TrimSpace(rule.TenantID) != "" && !strings.EqualFold(rule.TenantID, meta.TenantID) {
			continue
		}
		return rule.PoolID
	}
	return ""
}

func (s *Selector) matchCrossDomain(meta Metadata) string {
	if meta.VRF != "" {
		for _, rule := range s.cfg.CrossDomain.VRFRules {
			if strings.TrimSpace(rule.PoolID) == "" {
				continue
			}
			if strings.EqualFold(rule.VRF, meta.VRF) {
				return rule.PoolID
			}
		}
	}
	vpnID := firstNonEmpty(meta.VPNID, meta.MPLSVPN)
	if vpnID != "" {
		for _, rule := range s.cfg.CrossDomain.MPLSVPNRules {
			if strings.TrimSpace(rule.PoolID) == "" {
				continue
			}
			if strings.EqualFold(rule.VPNID, vpnID) {
				return rule.PoolID
			}
		}
	}
	if meta.VLANID > 0 {
		if pool := s.vlanPools[meta.VLANID]; pool != "" {
			return pool
		}
	}
	return ""
}

func (s *Selector) matchRules(meta Metadata) string {
	for _, rule := range s.cfg.PoolSelection.Rules {
		if strings.TrimSpace(rule.PoolID) == "" {
			continue
		}
		if matchesCriteria(meta, rule.Match) {
			return rule.PoolID
		}
	}
	return ""
}

func (s *Selector) regionFallback(meta Metadata) string {
	if pool := s.lookupRegion(meta.Region); pool != "" {
		return pool
	}
	if pool := s.lookupRegion(meta.DataCenter); pool != "" {
		return pool
	}
	return ""
}

func (s *Selector) lookupRegion(key string) string {
	if strings.TrimSpace(key) == "" {
		return ""
	}
	if pools, ok := s.regionFallbacks[strings.ToLower(strings.TrimSpace(key))]; ok {
		if len(pools) > 0 {
			return pools[0]
		}
	}
	return ""
}

func matchesCriteria(meta Metadata, criteria config.RelayMatchCriteria) bool {
	if strings.TrimSpace(criteria.TenantID) != "" && !strings.EqualFold(criteria.TenantID, meta.TenantID) {
		return false
	}
	if strings.TrimSpace(criteria.RelayID) != "" && !strings.EqualFold(criteria.RelayID, meta.RelayID) {
		return false
	}
	if strings.TrimSpace(criteria.GIAddr) != "" && !strings.EqualFold(criteria.GIAddr, meta.GIAddr) {
		return false
	}
	if strings.TrimSpace(criteria.CircuitID) != "" && !strings.EqualFold(criteria.CircuitID, meta.CircuitID) {
		return false
	}
	if strings.TrimSpace(criteria.RemoteID) != "" && !strings.EqualFold(criteria.RemoteID, meta.RemoteID) {
		return false
	}
	if criteria.VLANID > 0 && meta.VLANID != criteria.VLANID {
		return false
	}
	if strings.TrimSpace(criteria.VRF) != "" && !strings.EqualFold(criteria.VRF, meta.VRF) {
		return false
	}
	if strings.TrimSpace(criteria.VPNID) != "" {
		if !strings.EqualFold(criteria.VPNID, meta.VPNID) && !strings.EqualFold(criteria.VPNID, meta.MPLSVPN) {
			return false
		}
	}
	if strings.TrimSpace(criteria.Region) != "" && !strings.EqualFold(criteria.Region, meta.Region) {
		return false
	}
	if strings.TrimSpace(criteria.DataCenter) != "" && !strings.EqualFold(criteria.DataCenter, meta.DataCenter) {
		return false
	}
	if strings.TrimSpace(criteria.MPLSVPN) != "" && !strings.EqualFold(criteria.MPLSVPN, meta.MPLSVPN) {
		return false
	}
	if strings.TrimSpace(criteria.UserGroup) != "" && !containsIgnoreCase(meta.UserGroups, criteria.UserGroup) {
		return false
	}
	return true
}

func containsIgnoreCase(values []string, want string) bool {
	if len(values) == 0 || strings.TrimSpace(want) == "" {
		return false
	}
	for _, val := range values {
		if strings.EqualFold(val, want) {
			return true
		}
	}
	return false
}

func buildVLANPool(entries []config.RelayVLANPool) map[int]string {
	if len(entries) == 0 {
		return nil
	}
	lookup := make(map[int]string)
	for _, entry := range entries {
		for _, token := range entry.VLANs {
			token = strings.TrimSpace(token)
			if token == "" {
				continue
			}
			if strings.Contains(token, "-") {
				parts := strings.SplitN(token, "-", 2)
				start, errStart := strconv.Atoi(strings.TrimSpace(parts[0]))
				end, errEnd := strconv.Atoi(strings.TrimSpace(parts[1]))
				if errStart != nil || errEnd != nil || end < start {
					continue
				}
				for v := start; v <= end; v++ {
					lookup[v] = entry.PoolID
				}
				continue
			}
			if v, err := strconv.Atoi(token); err == nil {
				lookup[v] = entry.PoolID
			}
		}
	}
	if len(lookup) == 0 {
		return nil
	}
	return lookup
}

func normalizeRegionFallbacks(src map[string][]string) map[string][]string {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string][]string, len(src))
	for key, pools := range src {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		out[strings.ToLower(key)] = pools
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
