package db

import (
	"database/sql"
	"fmt"
)

// Card is one row of the cards table. Nullable columns are *string so JSON
// output distinguishes null from "" (the CLI --json contract).
type Card struct {
	ID              int64   `json:"id"`
	Title           string  `json:"title"`
	Description     string  `json:"description"`
	Status          string  `json:"status"`
	Position        int64   `json:"position"`
	Priority        string  `json:"priority"`
	DueDate         *string `json:"due_date"`
	Labels          string  `json:"labels"` // legacy free-form column, unused, always ""
	Label           *string `json:"label"`
	StatusChangedAt *string `json:"status_changed_at"`
	CreatedAt       string  `json:"created_at"`
	UpdatedAt       string  `json:"updated_at"`
}

// Revision is a cheap, comparable signature of the board's state; it changes
// on any card or label add, edit, move or removal. Notes are excluded on
// purpose: they never render on the board, so a note change must not make a
// polling TUI reload.
type Revision struct {
	CardCount    int64
	MaxUpdatedAt string
	LabelCount   int64
	LabelNames   string
}

// cardColumnsSQL is the column list for every card SELECT. Never SELECT *:
// migrated databases have a different physical column order.
const cardColumnsSQL = `id, title, description, status, position, priority, due_date, labels, label,
	status_changed_at, created_at, updated_at`

func scanCard(rs rowScanner) (Card, error) {
	var (
		c                               Card
		dueDate, label, statusChangedAt sql.NullString
	)
	err := rs.Scan(
		&c.ID, &c.Title, &c.Description, &c.Status, &c.Position, &c.Priority,
		&dueDate, &c.Labels, &label,
		&statusChangedAt,
		&c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return Card{}, err
	}
	c.DueDate = nullableToPtr(dueDate)
	c.Label = nullableToPtr(label)
	c.StatusChangedAt = nullableToPtr(statusChangedAt)
	return c, nil
}

// GetAllCards returns every card ordered by status, position, id.
func (d *DB) GetAllCards() ([]Card, error) {
	rows, err := d.conn.Query(`SELECT ` + cardColumnsSQL + ` FROM cards ORDER BY status, position, id`)
	if err != nil {
		return nil, fmt.Errorf("db: listing cards: %w", err)
	}
	defer rows.Close()

	cards := []Card{}
	for rows.Next() {
		c, err := scanCard(rows)
		if err != nil {
			return nil, fmt.Errorf("db: listing cards: %w", err)
		}
		cards = append(cards, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("db: listing cards: %w", err)
	}
	return cards, nil
}

// GetCard returns (nil, nil) if no such card exists.
func (d *DB) GetCard(id int64) (*Card, error) {
	row := d.conn.QueryRow(`SELECT `+cardColumnsSQL+` FROM cards WHERE id = ?`, id)
	c, err := scanCard(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("db: fetching card %d: %w", id, err)
	}
	return &c, nil
}

func (d *DB) GetRevision() (Revision, error) {
	var rev Revision
	if err := d.conn.QueryRow(
		`SELECT COUNT(*), COALESCE(MAX(updated_at), '') FROM cards`,
	).Scan(&rev.CardCount, &rev.MaxUpdatedAt); err != nil {
		return Revision{}, fmt.Errorf("db: computing revision: %w", err)
	}
	if err := d.conn.QueryRow(
		`SELECT COUNT(*), COALESCE(GROUP_CONCAT(name, '|'), '') FROM labels`,
	).Scan(&rev.LabelCount, &rev.LabelNames); err != nil {
		return Revision{}, fmt.Errorf("db: computing revision: %w", err)
	}
	return rev, nil
}

// nextPosition returns one past the max position within status (0 if empty).
func (d *DB) nextPosition(status string) (int64, error) {
	var pos int64
	err := d.conn.QueryRow(
		`SELECT COALESCE(MAX(position), -1) + 1 FROM cards WHERE status = ?`, status,
	).Scan(&pos)
	return pos, err
}

// CreateCard inserts a card at the end of its column and returns its id. The
// legacy labels column is always written as "".
func (d *DB) CreateCard(title, description, status, priority string, dueDate, label *string) (int64, error) {
	pos, err := d.nextPosition(status)
	if err != nil {
		return 0, fmt.Errorf("db: creating card: %w", err)
	}
	now := nowStamp()
	res, err := d.conn.Exec(
		`INSERT INTO cards
			(title, description, status, position, priority, due_date, label, labels,
			 status_changed_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		title, description, status, pos, priority, nullableArg(dueDate), nullableArg(label), "",
		now, now, now,
	)
	if err != nil {
		return 0, fmt.Errorf("db: creating card: %w", err)
	}
	return res.LastInsertId()
}

// UpdateCard edits content fields and bumps updated_at; status, position and
// status_changed_at are untouched.
func (d *DB) UpdateCard(id int64, title, description, priority string, dueDate, label *string) error {
	now := nowStamp()
	_, err := d.conn.Exec(
		`UPDATE cards SET title=?, description=?, priority=?, due_date=?, label=?, updated_at=? WHERE id=?`,
		title, description, priority, nullableArg(dueDate), nullableArg(label), now, id,
	)
	if err != nil {
		return fmt.Errorf("db: updating card %d: %w", id, err)
	}
	return nil
}

// UpdateCardStatus moves a card to the end of another column. It writes
// nothing when the status is unchanged, so a repeated move can't reset the
// card's aging clock (status_changed_at).
func (d *DB) UpdateCardStatus(id int64, newStatus string) error {
	var current string
	err := d.conn.QueryRow(`SELECT status FROM cards WHERE id=?`, id).Scan(&current)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("db: updating status of card %d: %w", id, err)
	}
	found := err == nil
	if found && current == newStatus {
		return nil
	}

	pos, err := d.nextPosition(newStatus)
	if err != nil {
		return fmt.Errorf("db: updating status of card %d: %w", id, err)
	}
	now := nowStamp()
	if _, err := d.conn.Exec(
		`UPDATE cards SET status=?, position=?, updated_at=?, status_changed_at=? WHERE id=?`,
		newStatus, pos, now, now, id,
	); err != nil {
		return fmt.Errorf("db: updating status of card %d: %w", id, err)
	}
	return nil
}

func (d *DB) DeleteCard(id int64) error {
	if _, err := d.conn.Exec(`DELETE FROM cards WHERE id = ?`, id); err != nil {
		return fmt.Errorf("db: deleting card %d: %w", id, err)
	}
	return nil
}

// MoveCardPosition swaps a card with its neighbor in the same column (delta
// < 0 = previous, otherwise next). Positions have gaps after deletes, so the
// neighbor is the nearest position, not position±1. Returns moved=false at
// the column edge. Bumps updated_at on both cards so other TUIs see the change.
func (d *DB) MoveCardPosition(id int64, delta int) (moved bool, err error) {
	tx, err := d.conn.Begin()
	if err != nil {
		return false, fmt.Errorf("db: reordering card %d: %w", id, err)
	}
	defer tx.Rollback()

	var status string
	var pos int64
	if err := tx.QueryRow(`SELECT status, position FROM cards WHERE id=?`, id).Scan(&status, &pos); err != nil {
		return false, fmt.Errorf("db: reordering card %d: %w", id, err)
	}

	var neighborID, neighborPos int64
	var row *sql.Row
	if delta < 0 {
		row = tx.QueryRow(`SELECT id, position FROM cards WHERE status=? AND position < ? ORDER BY position DESC LIMIT 1`, status, pos)
	} else {
		row = tx.QueryRow(`SELECT id, position FROM cards WHERE status=? AND position > ? ORDER BY position ASC LIMIT 1`, status, pos)
	}
	if err := row.Scan(&neighborID, &neighborPos); err == sql.ErrNoRows {
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("db: reordering card %d: %w", id, err)
	}

	now := nowStamp()
	if _, err := tx.Exec(`UPDATE cards SET position=?, updated_at=? WHERE id=?`, neighborPos, now, id); err != nil {
		return false, fmt.Errorf("db: reordering card %d: %w", id, err)
	}
	if _, err := tx.Exec(`UPDATE cards SET position=?, updated_at=? WHERE id=?`, pos, now, neighborID); err != nil {
		return false, fmt.Errorf("db: reordering card %d: %w", id, err)
	}
	return true, tx.Commit()
}
