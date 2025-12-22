package relay

import (
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"modern-dhcp/internal/config"
)

var (
	// ErrRelayNotAllowed indicates the relay metadata did not match any trusted identity.
	ErrRelayNotAllowed = errors.New("relay: identity not allowed")
	// ErrRelaySecretMismatch indicates PSK validation failed.
	ErrRelaySecretMismatch = errors.New("relay: shared secret mismatch")
)

// Authenticator enforces relay allow-lists and shared-secret validation.
type Authenticator struct {
	cfg     config.RelayAuthenticationConfig
	agents  []compiledAgent
	logger  *zap.Logger
	seenPSK sync.Map
}

type compiledAgent struct {
	config.RelayAgentIdentity
	vlanRanges [][2]int
}

// NewAuthenticator builds an authenticator if authentication is configured.
func NewAuthenticator(cfg config.RelayAuthenticationConfig, logger *zap.Logger) *Authenticator {
	if !cfg.Required && len(cfg.AllowedAgents) == 0 && len(cfg.SharedSecrets) == 0 {
		return nil
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	auth := &Authenticator{cfg: cfg, logger: logger}
	for _, agent := range cfg.AllowedAgents {
		auth.agents = append(auth.agents, compiledAgent{
			RelayAgentIdentity: agent,
			vlanRanges:         parseVLANRanges(agent.VLANRanges),
		})
	}
	return auth
}

// Authorize validates the relay metadata based on configuration.
func (a *Authenticator) Authorize(meta Metadata) error {
	if a == nil {
		return nil
	}
	if err := a.verifySecret(meta); err != nil {
		return err
	}
	if len(a.agents) == 0 {
		if a.cfg.Required {
			return ErrRelayNotAllowed
		}
		return nil
	}
	for _, agent := range a.agents {
		if agent.matches(meta) {
			return nil
		}
	}
	return ErrRelayNotAllowed
}

func (a *Authenticator) verifySecret(meta Metadata) error {
	if len(a.cfg.SharedSecrets) == 0 {
		return nil
	}
	secret := a.cfg.SharedSecrets[meta.RelayID]
	if strings.TrimSpace(secret) == "" {
		return nil
	}
	token := meta.Attribute("auth-token", "auth", "signature")
	if token == "" || token != secret {
		return ErrRelaySecretMismatch
	}
	if a.cfg.ReplayWindow > 0 {
		now := time.Now().UTC().Truncate(time.Second)
		key := meta.RelayID + token
		if last, ok := a.seenPSK.Load(key); ok {
			if ts, ok := last.(time.Time); ok {
				if now.Sub(ts) < a.cfg.ReplayWindow {
					return ErrRelaySecretMismatch
				}
			}
		}
		a.seenPSK.Store(key, now)
	}
	return nil
}

func (agent compiledAgent) matches(meta Metadata) bool {
	if strings.TrimSpace(agent.RelayID) != "" && !strings.EqualFold(agent.RelayID, meta.RelayID) {
		return false
	}
	if strings.TrimSpace(agent.GIAddr) != "" && !strings.EqualFold(agent.GIAddr, meta.GIAddr) {
		return false
	}
	if strings.TrimSpace(agent.RemoteID) != "" && !strings.EqualFold(agent.RemoteID, meta.RemoteID) {
		return false
	}
	if strings.TrimSpace(agent.CircuitID) != "" && !strings.EqualFold(agent.CircuitID, meta.CircuitID) {
		return false
	}
	if agent.VLANRanges != nil && meta.VLANID > 0 && !inVLANRanges(meta.VLANID, agent.vlanRanges) {
		return false
	}
	if strings.TrimSpace(agent.VPNID) != "" && !strings.EqualFold(agent.VPNID, meta.VPNID) {
		return false
	}
	if strings.TrimSpace(agent.Region) != "" && !strings.EqualFold(agent.Region, meta.Region) {
		return false
	}
	return true
}

func parseVLANRanges(raw []string) [][2]int {
	if len(raw) == 0 {
		return nil
	}
	var ranges [][2]int
	for _, token := range raw {
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
			ranges = append(ranges, [2]int{start, end})
			continue
		}
		if v, err := strconv.Atoi(token); err == nil {
			ranges = append(ranges, [2]int{v, v})
		}
	}
	return ranges
}

func inVLANRanges(vlan int, ranges [][2]int) bool {
	if vlan <= 0 {
		return false
	}
	for _, r := range ranges {
		if vlan >= r[0] && vlan <= r[1] {
			return true
		}
	}
	return false
}
