package db

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jmoiron/sqlx"
)

// TxOption mutates sql.TxOptions before the transaction starts.
type TxOption func(*sql.TxOptions)

// TxWithIsolation sets the isolation level used for the transaction.
func TxWithIsolation(level sql.IsolationLevel) TxOption {
	return func(opts *sql.TxOptions) {
		opts.Isolation = level
	}
}

// TxReadOnly toggles the read-only flag for the transaction.
func TxReadOnly(readOnly bool) TxOption {
	return func(opts *sql.TxOptions) {
		opts.ReadOnly = readOnly
	}
}

// WithTx executes fn inside a SQL transaction, committing only if fn succeeds.
func WithTx(ctx context.Context, dbHandle *sqlx.DB, fn func(*sqlx.Tx) error, opts ...TxOption) error {
	if dbHandle == nil {
		return fmt.Errorf("db: nil handle")
	}
	if fn == nil {
		return fmt.Errorf("db: tx function is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	txOpts := &sql.TxOptions{}
	for _, apply := range opts {
		if apply != nil {
			apply(txOpts)
		}
	}

	tx, err := dbHandle.BeginTxx(ctx, txOpts)
	if err != nil {
		return fmt.Errorf("db: begin tx: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	if err := fn(tx); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("db: commit tx: %w", err)
	}
	committed = true
	return nil
}
