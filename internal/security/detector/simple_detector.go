package detector

import (
	"context"
	"strings"
	"sync"
	"time"
)

// Config conveys threshold settings for the simple detector.
type Config struct {
	DeclineSpikeThreshold     int
	StarvationWindow          time.Duration
	Option82MismatchTolerance int
	DiscoverBurstThreshold    int
	DiscoverToRequestRatio    float64
	BlockDuration             time.Duration
	RogueServerAllowlist      []string
	MACDriftThreshold         int
}

type activity struct {
	declines     int
	discovers    int
	requests     int
	lastSeen     time.Time
	blockedUntil time.Time
	lastReason   string
	state        SecurityState
	lastPort     string
	portChanges  int
	portSeen     map[string]struct{}
}

type simpleDetector struct {
	cfg            Config
	mu             sync.Mutex
	cache          map[string]*activity
	rogueAllowlist map[string]struct{}
}

// NewSimpleDetector builds a lightweight in-memory behavioral detector.
func NewSimpleDetector(cfg Config) Detector {
	if cfg.StarvationWindow <= 0 {
		cfg.StarvationWindow = 30 * time.Second
	}
	if cfg.DiscoverToRequestRatio <= 0 {
		cfg.DiscoverToRequestRatio = 0.25
	}
	if cfg.BlockDuration <= 0 {
		cfg.BlockDuration = 2 * time.Minute
	}
	if cfg.MACDriftThreshold < 0 {
		cfg.MACDriftThreshold = 0
	}
	allowlist := make(map[string]struct{}, len(cfg.RogueServerAllowlist))
	for _, addr := range cfg.RogueServerAllowlist {
		allowlist[strings.ToLower(strings.TrimSpace(addr))] = struct{}{}
	}
	return &simpleDetector{
		cfg:            cfg,
		cache:          make(map[string]*activity),
		rogueAllowlist: allowlist,
	}
}

func (d *simpleDetector) Observe(ctx context.Context, evt Event) Verdict {
	key := strings.ToLower(evt.TenantID + "|" + evt.MAC)
	now := time.Now()

	d.mu.Lock()
	defer d.mu.Unlock()

	bucket := d.touchBucket(key, now)
	if verdict, ok := d.evaluatePreconditions(bucket, evt, now); ok {
		return verdict
	}

	d.trackEvent(bucket, evt)
	if verdict, ok := d.inspectTopology(bucket, evt, now); ok {
		return verdict
	}
	return d.evaluatePatterns(bucket, evt, now)
}

func (d *simpleDetector) touchBucket(key string, now time.Time) *activity {
	bucket := d.cache[key]
	if bucket == nil || now.Sub(bucket.lastSeen) > d.cfg.StarvationWindow {
		bucket = &activity{lastSeen: now, state: SecurityStateOK}
		d.cache[key] = bucket
	} else {
		bucket.lastSeen = now
	}
	return bucket
}

func (d *simpleDetector) evaluatePreconditions(bucket *activity, evt Event, now time.Time) (Verdict, bool) {
	if bucket.state == "" {
		bucket.state = SecurityStateOK
	}
	if bucket.blockedUntil.After(now) {
		return Verdict{Block: true, Reason: bucket.lastReason, State: SecurityStateBlocked}, true
	}
	if d.isRogueServer(evt) {
		return d.block(bucket, "rogue_server", now), true
	}
	return Verdict{}, false
}

func (d *simpleDetector) trackEvent(bucket *activity, evt Event) {
	switch strings.ToLower(evt.MessageType) {
	case "decline":
		bucket.declines++
	case "discover":
		bucket.discovers++
	case "request":
		bucket.requests++
	}
}

func (d *simpleDetector) evaluatePatterns(bucket *activity, evt Event, now time.Time) Verdict {
	if d.cfg.DeclineSpikeThreshold > 0 && bucket.declines >= d.cfg.DeclineSpikeThreshold {
		return d.block(bucket, "decline_spike", now)
	}
	if d.cfg.DiscoverBurstThreshold > 0 && bucket.discovers >= d.cfg.DiscoverBurstThreshold {
		requests := bucket.requests
		ratio := float64(requests)
		if bucket.discovers > 0 {
			ratio = float64(requests) / float64(bucket.discovers)
		}
		if ratio < d.cfg.DiscoverToRequestRatio {
			return d.block(bucket, "starvation_pattern", now)
		}
		bucket.state = SecurityStateSuspect
		return Verdict{Block: false, Score: ratio, Reason: "discover_spike", State: bucket.state}
	}
	return Verdict{Block: false, Score: 0, Reason: "", State: bucket.state}
}

func (d *simpleDetector) block(bucket *activity, reason string, now time.Time) Verdict {
	bucket.blockedUntil = now.Add(d.cfg.BlockDuration)
	bucket.lastReason = reason
	bucket.state = SecurityStateBlocked
	return Verdict{Block: true, Reason: reason, State: SecurityStateBlocked}
}

func (d *simpleDetector) isRogueServer(evt Event) bool {
	if len(d.rogueAllowlist) == 0 {
		return false
	}
	addr := strings.ToLower(strings.TrimSpace(evt.GIAddr))
	if addr == "" {
		return false
	}
	_, ok := d.rogueAllowlist[addr]
	return !ok
}

func (d *simpleDetector) inspectTopology(bucket *activity, evt Event, now time.Time) (Verdict, bool) {
	port := strings.ToLower(strings.TrimSpace(evt.PortID))
	if port == "" {
		return Verdict{}, false
	}
	if bucket.portSeen == nil {
		bucket.portSeen = make(map[string]struct{})
	}
	if bucket.lastPort != "" && bucket.lastPort != port {
		bucket.portChanges++
		if d.cfg.Option82MismatchTolerance > 0 && bucket.portChanges > d.cfg.Option82MismatchTolerance {
			return d.block(bucket, "option82_mismatch", now), true
		}
	}
	bucket.lastPort = port
	if _, ok := bucket.portSeen[port]; !ok {
		bucket.portSeen[port] = struct{}{}
		if d.cfg.MACDriftThreshold > 0 && len(bucket.portSeen) > d.cfg.MACDriftThreshold {
			return d.block(bucket, "mac_drift", now), true
		}
	}
	return Verdict{}, false
}
