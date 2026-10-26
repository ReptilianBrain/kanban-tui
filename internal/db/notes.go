package db

import (
	"database/sql"
	"fmt"
)

// Note is a quick-capture scratch idea; notes are excluded from GetRevision.
type Note struct {
	ID        int64  `json:"id"`
	Text      string `json:"text"`
	CreatedAt string `json:"created_at"`
}

func (d *DB) GetNotes() ([]Note, error) {
	rows, err := d.conn.Query(`SELECT id, text, created_at FROM notes ORDER BY id DESC`)
	if err != nil {
		return nil, fmt.Errorf("db: listing notes: %w", err)
	}
	defer rows.Close()

	notes := []Note{}
	for rows.Next() {
		var n Note
		if err := rows.Scan(&n.ID, &n.Text, &n.CreatedAt); err != nil {
			return nil, fmt.Errorf("db: listing notes: %w", err)
		}
		notes = append(notes, n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("db: listing notes: %w", err)
	}
	return notes, nil
}

// GetNote returns (nil, nil) if no such note exists.
func (d *DB) GetNote(id int64) (*Note, error) {
	var n Note
	err := d.conn.QueryRow(`SELECT id, text, created_at FROM notes WHERE id = ?`, id).
		Scan(&n.ID, &n.Text, &n.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("db: fetching note %d: %w", id, err)
	}
	return &n, nil
}

func (d *DB) CreateNote(text string) (int64, error) {
	res, err := d.conn.Exec(
		`INSERT INTO notes (text, created_at) VALUES (?, ?)`, text, nowStamp(),
	)
	if err != nil {
		return 0, fmt.Errorf("db: creating note: %w", err)
	}
	return res.LastInsertId()
}

func (d *DB) UpdateNote(id int64, text string) error {
	if _, err := d.conn.Exec(`UPDATE notes SET text = ? WHERE id = ?`, text, id); err != nil {
		return fmt.Errorf("db: updating note %d: %w", id, err)
	}
	return nil
}

func (d *DB) DeleteNote(id int64) error {
	if _, err := d.conn.Exec(`DELETE FROM notes WHERE id = ?`, id); err != nil {
		return fmt.Errorf("db: deleting note %d: %w", id, err)
	}
	return nil
}
