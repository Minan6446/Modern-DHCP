package db

import (
	"context"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

const (
	defaultMaxOpenConns    = 50
	defaultMaxIdleConns    = 10
	defaultConnMaxLifetime = 5 * time.Minute
	defaultConnMaxIdleTime = 1 * time.Minute
	defaultDialTimeout     = 5 * time.Second
	defaultPingAttempts    = 3
	defaultRetryBackoff    = 150 * time.Millisecond
	maxRetryBackoff        = time.Second
)

func applyPoolSettings(db *sqlx.DB, maxOpen, maxIdle int, maxLife, maxIdleTime time.Duration) {
	if db == nil {
		return
	}
	if maxOpen <= 0 {
		maxOpen = defaultMaxOpenConns
	}
	if maxIdle < 0 {
		maxIdle = 0
	}
	if maxIdle == 0 {
		maxIdle = defaultMaxIdleConns
	}
	if maxLife <= 0 {
		maxLife = defaultConnMaxLifetime
	}
	if maxIdleTime <= 0 {
		maxIdleTime = defaultConnMaxIdleTime
	}
	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(maxIdle)
	db.SetConnMaxLifetime(maxLife)
	db.SetConnMaxIdleTime(maxIdleTime)
}

func pingWithRetry(ctx context.Context, db *sqlx.DB) error {
	if db == nil {
		return fmt.Errorf("db: nil handle")
	}
	attempts := defaultPingAttempts
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		pingCtx, cancel := context.WithTimeout(ctx, defaultDialTimeout)
		err := db.PingContext(pingCtx)
		cancel()
		if err == nil {
			return nil
		}
		lastErr = err
		select {
		case <-ctx.Done():
			return fmt.Errorf("db: ping cancelled: %w", ctx.Err())
		case <-time.After(backoffDelay(attempt)):
		}
	}
	return fmt.Errorf("db: ping failed after %d attempts: %w", attempts, lastErr)
}

func backoffDelay(attempt int) time.Duration {
	delay := time.Duration(attempt*attempt) * defaultRetryBackoff
	if delay > maxRetryBackoff {
		return maxRetryBackoff
	}
	return delay
}
