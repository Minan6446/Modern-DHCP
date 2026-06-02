package server

import (
	"sort"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
)

// RouteUsageRecorder tracks per-route hit counts and last-seen timestamps.
type RouteUsageRecorder struct {
	mu     sync.Mutex
	hits   map[string]int
	recent map[string]time.Time
}

func NewRouteUsageRecorder() *RouteUsageRecorder {
	return &RouteUsageRecorder{
		hits:   make(map[string]int),
		recent: make(map[string]time.Time),
	}
}

func (r *RouteUsageRecorder) Middleware() echo.MiddlewareFunc {
	if r == nil {
		return func(next echo.HandlerFunc) echo.HandlerFunc {
			return func(c echo.Context) error { return next(c) }
		}
	}
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			key := c.Request().Method + " " + c.Path()
			now := time.Now()
			r.mu.Lock()
			r.hits[key]++
			r.recent[key] = now
			r.mu.Unlock()
			return next(c)
		}
	}
}

// RouteUsageEntry is a snapshot view for reporting.
type RouteUsageEntry struct {
	Key      string
	Hits     int
	LastSeen time.Time
}

func (r *RouteUsageRecorder) Snapshot() []RouteUsageEntry {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]RouteUsageEntry, 0, len(r.hits))
	for key, hits := range r.hits {
		out = append(out, RouteUsageEntry{Key: key, Hits: hits, LastSeen: r.recent[key]})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}
