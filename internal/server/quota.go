package server

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
)

// quotaEnforcer tracks daily request limits.
type quotaEnforcer struct {
	opts     QuotaOptions
	mu       sync.Mutex
	counters map[string]*quotaCounter
}

type quotaCounter struct {
	count int
	reset time.Time
}

func newQuotaEnforcer(opts QuotaOptions) *quotaEnforcer {
	if !opts.Enabled {
		return nil
	}
	if opts.DefaultDaily <= 0 {
		opts.DefaultDaily = 20000
	}
	resetHour := opts.ResetHourUTC
	if resetHour < 0 || resetHour > 23 {
		resetHour = 0
	}
	opts.ResetHourUTC = resetHour
	return &quotaEnforcer{
		opts:     opts,
		counters: make(map[string]*quotaCounter),
	}
}

func (q *quotaEnforcer) limitFor(key, role string) int {
	if key != "" {
		if lim, ok := q.opts.PerAPIKey[key]; ok {
			return lim
		}
	}
	if role != "" {
		if lim, ok := q.opts.PerRole[role]; ok {
			return lim
		}
	}
	return q.opts.DefaultDaily
}

func (q *quotaEnforcer) nextReset(now time.Time) time.Time {
	reset := time.Date(now.UTC().Year(), now.UTC().Month(), now.UTC().Day(), q.opts.ResetHourUTC, 0, 0, 0, time.UTC)
	if !now.UTC().Before(reset) {
		reset = reset.Add(24 * time.Hour)
	}
	return reset
}

func (q *quotaEnforcer) check(counterKey string, limit int) (allowed bool, remaining int, reset time.Time) {
	if limit <= 0 {
		return true, -1, time.Time{}
	}
	now := time.Now().UTC()
	reset = q.nextReset(now)
	q.mu.Lock()
	defer q.mu.Unlock()
	counter, ok := q.counters[counterKey]
	if !ok || now.After(counter.reset) {
		counter = &quotaCounter{count: 0, reset: reset}
		q.counters[counterKey] = counter
	}
	if counter.count >= limit {
		return false, 0, counter.reset
	}
	counter.count++
	remaining = limit - counter.count
	return true, remaining, counter.reset
}

func (q *quotaEnforcer) Middleware() echo.MiddlewareFunc {
	if q == nil {
		return func(next echo.HandlerFunc) echo.HandlerFunc {
			return func(c echo.Context) error {
				return next(c)
			}
		}
	}
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			method := c.Request().Method
			if !q.opts.IncludeWriteOnly && (method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch || method == http.MethodDelete) {
				return next(c)
			}
			credential, _ := c.Get(contextCredentialKey).(string)
			rateKey, _ := c.Get(contextRateKey).(string)
			role, _ := c.Get(contextRoleKey).(string)
			if rateKey == "" {
				if credential != "" {
					rateKey = credential
				} else if role != "" {
					rateKey = role
				} else {
					rateKey = c.RealIP()
				}
			}
			limit := q.limitFor(credential, role)
			allowed, remaining, reset := q.check(rateKey, limit)
			if reset.IsZero() {
				return next(c)
			}
			c.Response().Header().Set("X-Quota-Reset", reset.Format(time.RFC3339))
			c.Response().Header().Set("X-Quota-Remaining", formatRemaining(remaining))
			c.Response().Header().Set("X-Quota-Limit", formatRemaining(limit))
			if !allowed {
				return echo.NewHTTPError(http.StatusTooManyRequests, "daily quota exceeded")
			}
			return next(c)
		}
	}
}

func formatRemaining(v int) string {
	if v < 0 {
		return "unlimited"
	}
	return strconv.Itoa(v)
}
