package storage

import (
	"database/sql"
	"embed"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// 轻量迁移 runner（无 CGO 依赖）。方向基线选型为 golang-migrate，
// 但其 SQLite driver 依赖 CGO；为保持纯 Go 构建，Phase 0 采用此 runner。
// 文件约定：<version>_<name>.up.sql / .down.sql，version 为整数前缀。

//go:embed migrations/*.sql
var migrationsFS embed.FS

type migration struct {
	version int
	name    string
	up      bool
}

func migrate(db *sql.DB) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version    INTEGER PRIMARY KEY,
		applied_at INTEGER NOT NULL
	)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	applied := map[int]bool{}
	rows, err := db.Query(`SELECT version FROM schema_migrations`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return err
		}
		applied[v] = true
	}
	if err := rows.Err(); err != nil {
		return err
	}

	migs, err := readMigrations()
	if err != nil {
		return err
	}
	for _, m := range migs {
		if !m.up || applied[m.version] {
			continue
		}
		text, err := migrationsFS.ReadFile("migrations/" + m.name)
		if err != nil {
			return err
		}
		if err := applyMigration(db, m.version, string(text)); err != nil {
			return fmt.Errorf("apply migration %d (%s): %w", m.version, m.name, err)
		}
	}
	return nil
}

func applyMigration(db *sql.DB, version int, text string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(text); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)`, version, now()); err != nil {
		return err
	}
	return tx.Commit()
}

func readMigrations() ([]migration, error) {
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return nil, err
	}
	var migs []migration
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".sql") {
			continue
		}
		stem := strings.TrimSuffix(name, ".sql")
		dot := strings.LastIndex(stem, ".")
		if dot < 0 {
			continue
		}
		verPart := stem[:dot]
		under := strings.Index(verPart, "_")
		if under < 0 {
			continue
		}
		v, err := strconv.Atoi(verPart[:under])
		if err != nil {
			continue
		}
		migs = append(migs, migration{version: v, name: name, up: stem[dot+1:] == "up"})
	}
	sort.Slice(migs, func(i, j int) bool { return migs[i].version < migs[j].version })
	return migs, nil
}
