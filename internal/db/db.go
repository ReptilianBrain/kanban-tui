// Package db is the SQLite storage layer for KanbanTUI. The database lives at
// ~/.kanbantui/kanban.db unless KANBANTUI_DB_PATH overrides it (tests only).
package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

type DB struct {
	conn *sql.DB
}

// rowScanner is satisfied by both *sql.Row and *sql.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

func DefaultPath() (string, error) {
	if p := os.Getenv("KANBANTUI_DB_PATH"); p != "" {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("db: resolving home directory: %w", err)
	}
	return filepath.Join(home, ".kanbantui", "kanban.db"), nil
}

// Open initializes the database at DefaultPath.
func Open() (*DB, error) {
	path, err := DefaultPath()
	if err != nil {
		return nil, err
	}
	return InitDB(path)
}

// InitDB opens (creating if necessary) the database at path and runs the
// idempotent migration. Safe to call repeatedly against the same file.
func InitDB(path string) (*DB, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("db: creating parent directory: %w", err)
		}
	}

	// _pragma DSN params apply to every connection the pool opens. WAL plus
	// a busy timeout let the CLI and a running TUI share the file.
	dsn := "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("db: opening %s: %w", path, err)
	}

	d := &DB{conn: conn}
	if err := d.migrate(); err != nil {
		conn.Close()
		return nil, err
	}
	return d, nil
}

func (d *DB) Close() error {
	return d.conn.Close()
}

// cardMigrationColumns are the columns added to `cards` after its minimal
// CREATE TABLE shape, applied in order and skipped when already present.
var cardMigrationColumns = []struct {
	name string
	ddl  string
}{
	{"priority", "ALTER TABLE cards ADD COLUMN priority TEXT NOT NULL DEFAULT 'medium'"},
	{"due_date", "ALTER TABLE cards ADD COLUMN due_date TEXT DEFAULT NULL"},
	{"labels", "ALTER TABLE cards ADD COLUMN labels TEXT NOT NULL DEFAULT ''"},
	{"label", "ALTER TABLE cards ADD COLUMN label TEXT DEFAULT NULL"},
	{"status_changed_at", "ALTER TABLE cards ADD COLUMN status_changed_at TEXT DEFAULT NULL"},
}

// legacyCardColumns are obsolete gitlab_* columns; migrate() drops them, and
// their data, from databases that still have them.
var legacyCardColumns = []string{
	"gitlab_mr_url", "gitlab_mr_state", "gitlab_pipeline_status", "gitlab_checked_at",
}

// migrate creates missing tables and columns. Idempotent; runs on every startup.
func (d *DB) migrate() error {
	if _, err := d.conn.Exec(`
		CREATE TABLE IF NOT EXISTS labels (
			id   INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT    NOT NULL UNIQUE
		)
	`); err != nil {
		return fmt.Errorf("db: creating labels table: %w", err)
	}

	// Minimal shape; the remaining columns come from cardMigrationColumns.
	if _, err := d.conn.Exec(`
		CREATE TABLE IF NOT EXISTS cards (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			title       TEXT    NOT NULL,
			description TEXT    NOT NULL DEFAULT '',
			status      TEXT    NOT NULL DEFAULT 'todo',
			position    INTEGER NOT NULL DEFAULT 0,
			created_at  TEXT    NOT NULL,
			updated_at  TEXT    NOT NULL
		)
	`); err != nil {
		return fmt.Errorf("db: creating cards table: %w", err)
	}

	// notes.id deliberately lacks AUTOINCREMENT, unlike labels/cards: existing
	// databases have it this way and the schema must stay identical.
	if _, err := d.conn.Exec(`
		CREATE TABLE IF NOT EXISTS notes (
			id         INTEGER PRIMARY KEY,
			text       TEXT    NOT NULL,
			created_at TEXT    NOT NULL
		)
	`); err != nil {
		return fmt.Errorf("db: creating notes table: %w", err)
	}

	existing, err := d.cardColumns()
	if err != nil {
		return err
	}
	for _, col := range cardMigrationColumns {
		if existing[col.name] {
			continue
		}
		if _, err := d.conn.Exec(col.ddl); err != nil {
			return fmt.Errorf("db: migrating column %s: %w", col.name, err)
		}
	}

	for _, name := range legacyCardColumns {
		if !existing[name] {
			continue
		}
		if _, err := d.conn.Exec("ALTER TABLE cards DROP COLUMN " + name); err != nil {
			return fmt.Errorf("db: dropping legacy column %s: %w", name, err)
		}
	}

	// "planned" is a retired status; fold leftover cards into Backlog.
	if _, err := d.conn.Exec(`UPDATE cards SET status = 'todo' WHERE status = 'planned'`); err != nil {
		return fmt.Errorf("db: folding legacy planned status: %w", err)
	}

	return nil
}

// cardColumns returns the set of column names currently on the cards table.
func (d *DB) cardColumns() (map[string]bool, error) {
	rows, err := d.conn.Query(`PRAGMA table_info(cards)`)
	if err != nil {
		return nil, fmt.Errorf("db: reading cards schema: %w", err)
	}
	defer rows.Close()

	cols := make(map[string]bool)
	for rows.Next() {
		var (
			cid       int
			name      string
			ctype     string
			notNull   int
			dfltValue sql.NullString
			pk        int
		)
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &dfltValue, &pk); err != nil {
			return nil, fmt.Errorf("db: reading cards schema: %w", err)
		}
		cols[name] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("db: reading cards schema: %w", err)
	}
	return cols, nil
}

// nullableArg maps a nil *string to SQL NULL and dereferences otherwise.
func nullableArg(p *string) any {
	if p == nil {
		return nil
	}
	return *p
}

// nullableToPtr returns nil for a NULL column so JSON output is null, not "".
func nullableToPtr(ns sql.NullString) *string {
	if !ns.Valid {
		return nil
	}
	v := ns.String
	return &v
}
