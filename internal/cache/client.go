package cache

import (
	"context"
	"crypto/tls"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"modern-dhcp/internal/config"
)

// NewRedisClient builds a universal Redis client (single or cluster).
func NewRedisClient(cfg config.RedisConfig, logger *zap.Logger) (redis.UniversalClient, error) {
	if len(cfg.Addresses) == 0 {
		return nil, nil
	}
	connectTimeout := cfg.ConnectTimeout
	if connectTimeout <= 0 {
		connectTimeout = 5 * time.Second
	}
	var tlsConfig *tls.Config
	if cfg.UseTLS {
		tlsConfig = &tls.Config{InsecureSkipVerify: cfg.TLSSkipVerify}
	}
	opts := &redis.UniversalOptions{
		Addrs:        cfg.Addresses,
		Username:     cfg.Username,
		Password:     cfg.Password,
		DB:           cfg.Database,
		MaxRetries:   cfg.MaxRetries,
		DialTimeout:  cfg.DialTimeout,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		PoolSize:     cfg.PoolSize,
		MinIdleConns: cfg.MinIdleConns,
		TLSConfig:    tlsConfig,
	}
	client := redis.NewUniversalClient(opts)
	ctx, cancel := context.WithTimeout(context.Background(), connectTimeout)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		if logger != nil {
			logger.Warn("redis ping failed", zap.Error(err))
		}
		// Older Redis servers reject AUTH username password, so retry without username when that happens.
		if cfg.Username != "" && strings.Contains(strings.ToLower(err.Error()), "wrong number of arguments for 'auth'") {
			if logger != nil {
				logger.Info("retrying redis auth without username for legacy servers")
			}
			_ = client.Close()
			opts.Username = ""
			client = redis.NewUniversalClient(opts)
			retryCtx, retryCancel := context.WithTimeout(context.Background(), connectTimeout)
			defer retryCancel()
			if retryErr := client.Ping(retryCtx).Err(); retryErr == nil {
				return client, nil
			} else {
				if logger != nil {
					logger.Warn("redis fallback auth failed", zap.Error(retryErr))
				}
				return nil, retryErr
			}
		}
		return nil, err
	}
	return client, nil
}
