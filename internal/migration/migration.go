package migration

import (
	"context"
	"embed"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
)

//go:embed sql/*.sql
var migrationFiles embed.FS

type Migration struct {
	Version string
	File    string
}

var migrations = []Migration{
	{Version: "001_initial", File: "001_initial.sql"},
	{Version: "002_add_recurring_to_schedules", File: "002_add_recurring_to_schedules.sql"},
	{Version: "003_radio_programs_unique_key", File: "003_radio_programs_unique_key.sql"},
	{Version: "004_push_subscriptions", File: "004_push_subscriptions.sql"},
	{Version: "005_add_retry_count_to_recording_schedules", File: "005_add_retry_count_to_recording_schedules.sql"},
	{Version: "006_add_next_retry_at_to_recording_schedules", File: "006_add_next_retry_at_to_recording_schedules.sql"},
	{Version: "007_add_feed_token_to_users", File: "007_add_feed_token_to_users.sql"},
}

func Run(ctx context.Context, db *sqlx.DB) error {
	if err := ensureMigrationTable(ctx, db); err != nil {
		return err
	}
	if err := markBaseline(ctx, db); err != nil {
		return err
	}
	applied, err := appliedVersions(ctx, db)
	if err != nil {
		return err
	}
	for _, m := range migrations {
		if applied[m.Version] {
			continue
		}
		if skip, err := shouldSkip(ctx, db, m.Version); err != nil {
			return err
		} else if skip {
			if err := markApplied(ctx, db, m.Version); err != nil {
				return err
			}
			continue
		}
		if err := applyMigration(ctx, db, m); err != nil {
			return err
		}
	}
	return nil
}

func ensureMigrationTable(ctx context.Context, db *sqlx.DB) error {
	_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version VARCHAR(255) NOT NULL PRIMARY KEY,
		applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`)
	if err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}
	return nil
}

func markBaseline(ctx context.Context, db *sqlx.DB) error {
	if empty, err := schemaMigrationsEmpty(ctx, db); err != nil {
		return err
	} else if !empty {
		return nil
	}
	tableChecks := map[string]string{
		"001_initial":                   "users",
		"004_push_subscriptions":        "push_subscriptions",
		"003_radio_programs_unique_key": "",
	}
	if exists, err := tableExists(ctx, db, "users"); err != nil {
		return err
	} else if !exists {
		return nil
	}
	for version, table := range tableChecks {
		if table == "" {
			continue
		}
		exists, err := tableExists(ctx, db, table)
		if err != nil {
			return err
		}
		if exists {
			if err := markApplied(ctx, db, version); err != nil {
				return err
			}
		}
	}
	for _, version := range []string{"002_add_recurring_to_schedules", "003_radio_programs_unique_key"} {
		if skip, err := shouldSkip(ctx, db, version); err != nil {
			return err
		} else if skip {
			if err := markApplied(ctx, db, version); err != nil {
				return err
			}
		}
	}
	return nil
}

func schemaMigrationsEmpty(ctx context.Context, db *sqlx.DB) (bool, error) {
	var count int
	if err := db.GetContext(ctx, &count, `SELECT COUNT(*) FROM schema_migrations`); err != nil {
		return false, fmt.Errorf("count schema_migrations: %w", err)
	}
	return count == 0, nil
}

func appliedVersions(ctx context.Context, db *sqlx.DB) (map[string]bool, error) {
	var versions []string
	if err := db.SelectContext(ctx, &versions, `SELECT version FROM schema_migrations`); err != nil {
		return nil, fmt.Errorf("select schema_migrations: %w", err)
	}
	applied := make(map[string]bool, len(versions))
	for _, version := range versions {
		applied[version] = true
	}
	return applied, nil
}

func shouldSkip(ctx context.Context, db *sqlx.DB, version string) (bool, error) {
	switch version {
	case "002_add_recurring_to_schedules":
		return columnExists(ctx, db, "recording_schedules", "is_recurring")
	case "003_radio_programs_unique_key":
		return indexExists(ctx, db, "radio_programs", "uq_radio_programs_station_title_cast")
	case "005_add_retry_count_to_recording_schedules":
		return columnExists(ctx, db, "recording_schedules", "retry_count")
	case "006_add_next_retry_at_to_recording_schedules":
		return columnExists(ctx, db, "recording_schedules", "next_retry_at")
	case "007_add_feed_token_to_users":
		return columnExists(ctx, db, "users", "feed_token")
	default:
		return false, nil
	}
}

func applyMigration(ctx context.Context, db *sqlx.DB, m Migration) error {
	body, err := migrationFiles.ReadFile("sql/" + m.File)
	if err != nil {
		return fmt.Errorf("read migration %s: %w", m.File, err)
	}
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration %s: %w", m.Version, err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, stmt := range splitStatements(string(body)) {
		if strings.TrimSpace(stmt) == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("apply migration %s: %w", m.Version, err)
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (version) VALUES (?)`, m.Version); err != nil {
		return fmt.Errorf("record migration %s: %w", m.Version, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration %s: %w", m.Version, err)
	}
	return nil
}

func markApplied(ctx context.Context, db *sqlx.DB, version string) error {
	_, err := db.ExecContext(ctx, `INSERT IGNORE INTO schema_migrations (version) VALUES (?)`, version)
	if err != nil {
		return fmt.Errorf("mark migration %s applied: %w", version, err)
	}
	return nil
}

func tableExists(ctx context.Context, db *sqlx.DB, table string) (bool, error) {
	var count int
	if err := db.GetContext(ctx, &count,
		`SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = ?`,
		table,
	); err != nil {
		return false, fmt.Errorf("check table %s: %w", table, err)
	}
	return count > 0, nil
}

func columnExists(ctx context.Context, db *sqlx.DB, table, column string) (bool, error) {
	var count int
	if err := db.GetContext(ctx, &count,
		`SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ?`,
		table, column,
	); err != nil {
		return false, fmt.Errorf("check column %s.%s: %w", table, column, err)
	}
	return count > 0, nil
}

func indexExists(ctx context.Context, db *sqlx.DB, table, index string) (bool, error) {
	var count int
	if err := db.GetContext(ctx, &count,
		`SELECT COUNT(*) FROM information_schema.statistics WHERE table_schema = DATABASE() AND table_name = ? AND index_name = ?`,
		table, index,
	); err != nil {
		return false, fmt.Errorf("check index %s.%s: %w", table, index, err)
	}
	return count > 0, nil
}

func splitStatements(sql string) []string {
	lines := strings.Split(sql, "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "--") {
			continue
		}
		kept = append(kept, line)
	}
	parts := strings.Split(strings.Join(kept, "\n"), ";")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
