package db

import (
	"database/sql"
	"fmt"
)

type Label struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

func (d *DB) GetLabels() ([]Label, error) {
	rows, err := d.conn.Query(`SELECT id, name FROM labels ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("db: listing labels: %w", err)
	}
	defer rows.Close()

	labels := []Label{}
	for rows.Next() {
		var l Label
		if err := rows.Scan(&l.ID, &l.Name); err != nil {
			return nil, fmt.Errorf("db: listing labels: %w", err)
		}
		labels = append(labels, l)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("db: listing labels: %w", err)
	}
	return labels, nil
}

func (d *DB) CreateLabel(name string) (int64, error) {
	res, err := d.conn.Exec(`INSERT INTO labels (name) VALUES (?)`, name)
	if err != nil {
		return 0, fmt.Errorf("db: creating label %q: %w", name, err)
	}
	return res.LastInsertId()
}

// EnsureLabel creates the label if missing; it is not an error if it exists.
func (d *DB) EnsureLabel(name string) error {
	if _, err := d.conn.Exec(`INSERT OR IGNORE INTO labels (name) VALUES (?)`, name); err != nil {
		return fmt.Errorf("db: ensuring label %q: %w", name, err)
	}
	return nil
}

// UpdateLabel renames a label and cascades the new name onto its cards.
func (d *DB) UpdateLabel(labelID int64, newName string) error {
	var oldName string
	err := d.conn.QueryRow(`SELECT name FROM labels WHERE id = ?`, labelID).Scan(&oldName)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("db: renaming label %d: %w", labelID, err)
	}
	if err == nil {
		if _, err := d.conn.Exec(`UPDATE cards SET label = ? WHERE label = ?`, newName, oldName); err != nil {
			return fmt.Errorf("db: renaming label %d: %w", labelID, err)
		}
	}

	if _, err := d.conn.Exec(`UPDATE labels SET name = ? WHERE id = ?`, newName, labelID); err != nil {
		return fmt.Errorf("db: renaming label %d: %w", labelID, err)
	}
	return nil
}

// DeleteLabel removes a label and sets label = NULL on its cards.
func (d *DB) DeleteLabel(labelID int64) error {
	var name string
	err := d.conn.QueryRow(`SELECT name FROM labels WHERE id = ?`, labelID).Scan(&name)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("db: deleting label %d: %w", labelID, err)
	}
	if err == nil {
		if _, err := d.conn.Exec(`UPDATE cards SET label = NULL WHERE label = ?`, name); err != nil {
			return fmt.Errorf("db: deleting label %d: %w", labelID, err)
		}
	}

	if _, err := d.conn.Exec(`DELETE FROM labels WHERE id = ?`, labelID); err != nil {
		return fmt.Errorf("db: deleting label %d: %w", labelID, err)
	}
	return nil
}
