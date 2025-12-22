package server

import (
	"net/http"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
	"golang.org/x/time/rate"
)

// rateLimiter enforces per-identity throttling.
type rateLimiter struct {
	opts    RateLimitOptions
	mu      sync.Mutex
	buckets map[string]*rate.Limiter
}

// newRateLimiter builds a reusable limiter helper.
func newRateLimiter(opts RateLimitOptions) *rateLimiter {
	if !opts.Enabled {
		return nil
	}
	if opts.Default.RequestsPerMinute <= 0 {
		opts.Default.RequestsPerMinute = 600
	}
	if opts.Default.Burst <= 0 {
		opts.Default.Burst = opts.Default.RequestsPerMinute
	}
	return &rateLimiter{
		opts:    opts,
		buckets: make(map[string]*rate.Limiter),
	}
}

func (r *rateLimiter) profile(credential, role string) RateLimitProfile {
	if credential != "" {
		if prof, ok := r.opts.PerAPIKey[credential]; ok {
			return sanitizeProfile(prof, r.opts.Default)
		}
	}
	if role != "" {
		if prof, ok := r.opts.PerRole[role]; ok {
			return sanitizeProfile(prof, r.opts.Default)
		}
	}
	return sanitizeProfile(r.opts.Default, r.opts.Default)
}

func sanitizeProfile(prof, fallback RateLimitProfile) RateLimitProfile {
	if prof.RequestsPerMinute <= 0 {
		prof.RequestsPerMinute = fallback.RequestsPerMinute
	}
	if prof.Burst <= 0 {
		prof.Burst = fallback.Burst
	}
	return prof
}

func (r *rateLimiter) limiterFor(bucketKey string, credential, role string) *rate.Limiter {
	prof := r.profile(credential, role)
	r.mu.Lock()
	defer r.mu.Unlock()
	if bucketKey == "" {
		bucketKey = role
		if bucketKey == "" {
			bucketKey = "__global__"
		}
	}
	limiter, ok := r.buckets[bucketKey]
	if !ok {
		limiter = rate.NewLimiter(rate.Limit(float64(prof.RequestsPerMinute)/60.0), prof.Burst)
		r.buckets[bucketKey] = limiter
	}
	limiter.SetLimit(rate.Limit(float64(prof.RequestsPerMinute) / 60.0))
	limiter.SetBurst(prof.Burst)
	return limiter
}

func (r *rateLimiter) Middleware() echo.MiddlewareFunc {
	if r == nil {
		return func(next echo.HandlerFunc) echo.HandlerFunc {
			return func(c echo.Context) error {
				return next(c)
			}
		}
	}
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			bucketKey, _ := c.Get(contextRateKey).(string)
			if bucketKey == "" && r.opts.FallbackToIP {
				bucketKey = c.RealIP()
			}
			credential, _ := c.Get(contextCredentialKey).(string)
			role, _ := c.Get(contextRoleKey).(string)
			limiter := r.limiterFor(bucketKey, credential, role)
			if limiter == nil {
				return next(c)
			}
			if !limiter.Allow() {
				reset := time.Now().Add(1 * time.Minute)
				c.Response().Header().Set("Retry-After", reset.Format(time.RFC1123))
				return echo.NewHTTPError(http.StatusTooManyRequests, "rate limit exceeded")
			}
			return next(c)
		}
	}
}
