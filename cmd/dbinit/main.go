package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"modern-dhcp/internal/config"
	"modern-dhcp/internal/db"
)

type migration struct {
	name       string
	statements []string
}

func main() {
	configPath := flag.String("config", "configs/config.yaml", "Path to configuration file")
	driver := flag.String("driver", "mysql", "Database driver (mysql|postgres)")
	dsn := flag.String("dsn", "", "Override DSN from config")
	dir := flag.String("dir", "migrations", "Directory containing .sql migrations")
	until := flag.String("until", "", "Run migrations up to and including this file (optional)")
	dryRun := flag.Bool("dry-run", false, "Print statements without executing them")
	compat := flag.String("compat", "", "Run migration compatibility check for target dialect (mysql|sqlite)")
	checkOnly := flag.Bool("check-only", false, "Only run migration compatibility check and exit")
	start := flag.String("start", "", "Skip migrations lexicographically before this file")
	leaseConverge := flag.Bool("lease-converge", false, "Run one-time lease convergence: migrate leases_v4 to lease_history_v4 and purge legacy rows")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	migrations, err := loadMigrations(*dir, *until)
	if err != nil {
		fatal(err)
	}
	if len(migrations) == 0 {
		fmt.Println("no migrations found; nothing to do")
		return
	}
	if target := strings.ToLower(strings.TrimSpace(*compat)); target != "" {
		if err := checkMigrationCompatibility(migrations, target); err != nil {
			fatal(err)
		}
		fmt.Printf("compatibility check passed for %s (%d migrations)\n", target, len(migrations))
		if *checkOnly {
			return
		}
	}

	sqlxDB, err := connectDatabase(ctx, *driver, *dsn, *configPath)
	if err != nil {
		fatal(err)
	}
	defer sqlxDB.Close()

	if err := ensureMigrationTable(ctx, sqlxDB); err != nil {
		fatal(err)
	}

	applied, err := loadAppliedMigrations(ctx, sqlxDB)
	if err != nil {
		fatal(err)
	}

	startName := strings.TrimSpace(*start)

	for _, mig := range migrations {
		if startName != "" && mig.name < startName {
			fmt.Printf("== %s == (skipped, before start target)\n", mig.name)
			continue
		}
		if _, already := applied[mig.name]; already {
			fmt.Printf("== %s == (skipped, already applied)\n", mig.name)
			continue
		}
		fmt.Printf("== %s ==\n", mig.name)
		if *dryRun {
			for _, stmt := range mig.statements {
				fmt.Println(stmt)
				fmt.Println("--")
			}
			continue
		}
		if err := applyMigration(ctx, sqlxDB, mig); err != nil {
			fatal(err)
		}
		if err := markMigrationApplied(ctx, sqlxDB, mig.name); err != nil {
			fatal(err)
		}
		fmt.Println("ok")
	}

	if *leaseConverge {
		if *dryRun {
			fmt.Println("lease convergence dry-run: skipped execution")
			return
		}
		if err := convergeLegacyLeases(ctx, sqlxDB); err != nil {
			fatal(err)
		}
	}
}

func convergeLegacyLeases(ctx context.Context, db *sqlx.DB) error {
	const migrateStmt = `
INSERT INTO lease_history_v4 (
	lease_id, tenant_id, pool_id, ip_address, hardware_addr, client_id, user_id,
	mobility_anchor_id, device_type, last_access_point_id, last_controller_id, last_geo_zone,
	mobility_location_hint, mdm_managed, mdm_source, mdm_tags, mdm_observed_at,
	relay_info, session_continuity, expires_at, final_state, security_state, cooldown_until,
	conflict_history, created_at, updated_at, archived_at
)
SELECT
	id,
	COALESCE(NULLIF(TRIM(tenant_id), ''), 'global') AS tenant_id,
	pool_id,
	ip_address,
	hardware_addr,
	NULLIF(TRIM(client_id), ''),
	NULLIF(TRIM(user_id), ''),
	COALESCE(mobility_anchor_id, ''),
	NULLIF(TRIM(device_type), ''),
	COALESCE(last_access_point_id, ''),
	COALESCE(last_controller_id, ''),
	COALESCE(last_geo_zone, ''),
	COALESCE(mobility_location_hint, ''),
	mdm_managed,
	NULLIF(TRIM(mdm_source), ''),
	mdm_tags,
	mdm_observed_at,
	relay_info,
	session_continuity,
	expires_at,
	state,
	COALESCE(NULLIF(TRIM(security_state), ''), 'OK'),
	cooldown_until,
	conflict_history,
	created_at,
	updated_at,
	UTC_TIMESTAMP()
FROM leases_v4
ON DUPLICATE KEY UPDATE
	final_state=VALUES(final_state),
	security_state=VALUES(security_state),
	updated_at=VALUES(updated_at),
	archived_at=VALUES(archived_at)`

	const purgeStmt = `DELETE FROM leases_v4`

	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin lease convergence tx: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	res, err := tx.ExecContext(ctx, migrateStmt)
	if err != nil {
		return fmt.Errorf("migrate leases_v4 to lease_history_v4: %w", err)
	}
	migrated, _ := res.RowsAffected()

	res, err = tx.ExecContext(ctx, purgeStmt)
	if err != nil {
		return fmt.Errorf("purge leases_v4: %w", err)
	}
	purged, _ := res.RowsAffected()

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit lease convergence tx: %w", err)
	}

	fmt.Printf("lease convergence completed: migrated=%d purged=%d\n", migrated, purged)
	return nil
}

func connectDatabase(ctx context.Context, driver, overrideDSN, configPath string) (*sqlx.DB, error) {
	driver = strings.ToLower(strings.TrimSpace(driver))
	if driver == "" {
		driver = "mysql"
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	switch driver {
	case "mysql":
		mysqlCfg := cfg.MySQL
		if overrideDSN != "" {
			mysqlCfg.DSN = overrideDSN
		}
		if mysqlCfg.DSN == "" {
			return nil, errors.New("mysql dsn is empty")
		}
		return db.NewMySQL(ctx, mysqlCfg)
	case "postgres", "pg", "postgresql":
		pgCfg := cfg.Postgres
		if overrideDSN != "" {
			pgCfg.DSN = overrideDSN
		}
		if pgCfg.DSN == "" {
			return nil, errors.New("postgres dsn is empty")
		}
		return db.NewPostgres(ctx, pgCfg)
	default:
		return nil, fmt.Errorf("unsupported driver: %s", driver)
	}
}

func loadMigrations(dir, until string) ([]migration, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read migrations dir: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	var migrations []migration
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(strings.ToLower(entry.Name()), ".sql") {
			continue
		}
		name := entry.Name()
		if until != "" && name > until {
			break
		}
		mig, err := parseMigration(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		migrations = append(migrations, mig)
	}
	return migrations, nil
}

func parseMigration(path string) (migration, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return migration{}, fmt.Errorf("read migration %s: %w", path, err)
	}
	script := extractUpSection(string(data))
	statements := splitSQLStatements(script)
	if len(statements) == 0 {
		return migration{}, fmt.Errorf("migration %s: no executable statements found", path)
	}
	return migration{name: filepath.Base(path), statements: statements}, nil
}

func extractUpSection(script string) string {
	marker := "-- +goose down"
	lower := strings.ToLower(script)
	if idx := strings.Index(lower, marker); idx >= 0 {
		return script[:idx]
	}
	return script
}

func ensureMigrationTable(ctx context.Context, db *sqlx.DB) error {
	var query string
	switch strings.ToLower(db.DriverName()) {
	case "pgx", "postgres", "pq":
		query = `CREATE TABLE IF NOT EXISTS schema_migrations (
			name TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL
		)`
	default:
		query = `CREATE TABLE IF NOT EXISTS schema_migrations (
			name VARCHAR(255) PRIMARY KEY,
			applied_at DATETIME NOT NULL
		)`
	}
	if _, err := db.ExecContext(ctx, query); err != nil {
		return fmt.Errorf("ensure schema_migrations: %w", err)
	}
	return nil
}

func loadAppliedMigrations(ctx context.Context, db *sqlx.DB) (map[string]struct{}, error) {
	result := make(map[string]struct{})
	rows, err := db.QueryxContext(ctx, "SELECT name FROM schema_migrations")
	if err != nil {
		return nil, fmt.Errorf("select applied migrations: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scan migration name: %w", err)
		}
		result[name] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate migrations: %w", err)
	}
	return result, nil
}

func markMigrationApplied(ctx context.Context, db *sqlx.DB, name string) error {
	query := db.Rebind("INSERT INTO schema_migrations (name, applied_at) VALUES (?, ?)")
	if _, err := db.ExecContext(ctx, query, name, time.Now().UTC()); err != nil {
		return fmt.Errorf("record migration %s: %w", name, err)
	}
	return nil
}

func checkMigrationCompatibility(migrations []migration, target string) error {
	target = strings.ToLower(strings.TrimSpace(target))
	type rule struct {
		needle  string
		message string
	}
	var rules []rule
	switch target {
	case "sqlite", "sqlite3":
		rules = []rule{
			{needle: "AUTO_INCREMENT", message: "SQLite 不支持 AUTO_INCREMENT（需改为 INTEGER PRIMARY KEY AUTOINCREMENT）"},
			{needle: "ENGINE=", message: "SQLite 不支持 ENGINE 选项"},
			{needle: "UNSIGNED", message: "SQLite 不支持 UNSIGNED 列类型"},
			{needle: "ON UPDATE CURRENT_TIMESTAMP", message: "SQLite 不支持 ON UPDATE CURRENT_TIMESTAMP 子句"},
			{needle: "DATETIME(6)", message: "SQLite 不支持 DATETIME(6) 精度语法"},
		}
	case "mysql":
		rules = []rule{
			{needle: "PRAGMA ", message: "MySQL 不支持 PRAGMA"},
			{needle: "WITHOUT ROWID", message: "MySQL 不支持 WITHOUT ROWID"},
			{needle: "AUTOINCREMENT", message: "MySQL 不使用 AUTOINCREMENT 关键字"},
		}
	default:
		return fmt.Errorf("unsupported compatibility target: %s", target)
	}

	var issues []string
	for _, mig := range migrations {
		for idx, stmt := range mig.statements {
			normalized := strings.ToUpper(stmt)
			for _, r := range rules {
				if strings.Contains(normalized, strings.ToUpper(r.needle)) {
					issues = append(issues, fmt.Sprintf("%s stmt#%d: %s", mig.name, idx+1, r.message))
				}
			}
		}
	}
	if len(issues) == 0 {
		return nil
	}
	for _, issue := range issues {
		fmt.Printf("compatibility issue: %s\n", issue)
	}
	return fmt.Errorf("found %d migration compatibility issue(s) for %s", len(issues), target)
}

func splitSQLStatements(script string) []string {
	var (
		stmts          []string
		sb             strings.Builder
		inSingleQuote  bool
		inDoubleQuote  bool
		inLineComment  bool
		inBlockComment bool
	)

	flush := func() {
		stmt := strings.TrimSpace(sb.String())
		if stmt != "" {
			stmt = strings.TrimSpace(strings.TrimSuffix(stmt, ";"))
			if stmt != "" {
				stmts = append(stmts, stmt)
			}
		}
		sb.Reset()
	}

	runes := []rune(script)
	for i := 0; i < len(runes); i++ {
		ch := runes[i]
		next := rune(0)
		if i+1 < len(runes) {
			next = runes[i+1]
		}

		if !inSingleQuote && !inDoubleQuote {
			if !inBlockComment && !inLineComment && ch == '-' && next == '-' {
				inLineComment = true
			} else if !inBlockComment && !inLineComment && ch == '/' && next == '*' {
				inBlockComment = true
			} else if inLineComment && (ch == '\n' || ch == '\r') {
				inLineComment = false
			} else if inBlockComment && ch == '*' && next == '/' {
				inBlockComment = false
				sb.WriteRune(ch)
				i++
				sb.WriteRune(runes[i])
				continue
			}
		}

		sb.WriteRune(ch)

		if inLineComment || inBlockComment {
			continue
		}

		switch ch {
		case '\'':
			if !inDoubleQuote {
				if inSingleQuote {
					if next == '\'' {
						sb.WriteRune(next)
						i++
						continue
					}
					inSingleQuote = false
				} else {
					inSingleQuote = true
				}
			}
		case '"':
			if !inSingleQuote {
				if inDoubleQuote {
					if next == '"' {
						sb.WriteRune(next)
						i++
						continue
					}
					inDoubleQuote = false
				} else {
					inDoubleQuote = true
				}
			}
		case ';':
			if !inSingleQuote && !inDoubleQuote {
				flush()
			}
		}
	}

	if sb.Len() > 0 {
		flush()
	}
	return stmts
}

func applyMigration(ctx context.Context, db *sqlx.DB, mig migration) error {
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("%s: begin tx: %w", mig.name, err)
	}
	for _, stmt := range mig.statements {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("%s: exec failed: %w\nstatement: %s", mig.name, err, stmt)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("%s: commit: %w", mig.name, err)
	}
	return nil
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "error: %v\n", err)
	os.Exit(1)
}
