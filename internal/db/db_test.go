package db

import (
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func openTestDB(t *testing.T) *DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "kanban.db")
	d, err := InitDB(path)
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

// Re-running migration against the same database must not error or duplicate
// columns or rows.
func TestMigrationIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kanban.db")
	d, err := InitDB(path)
	if err != nil {
		t.Fatalf("first InitDB: %v", err)
	}
	id, err := d.CreateCard("a card", "", "todo", "medium", nil, nil, nil)
	if err != nil {
		t.Fatalf("CreateCard: %v", err)
	}
	d.Close()

	d2, err := InitDB(path)
	if err != nil {
		t.Fatalf("second InitDB: %v", err)
	}
	if err := d2.migrate(); err != nil {
		t.Fatalf("running migrate a third time: %v", err)
	}

	cols, err := d2.cardColumns()
	if err != nil {
		t.Fatalf("cardColumns: %v", err)
	}
	for _, want := range []string{
		"priority", "due_date", "labels", "label", "status_changed_at",
	} {
		if !cols[want] {
			t.Errorf("expected column %q to exist after migration", want)
		}
	}

	all, err := d2.GetAllCards()
	if err != nil {
		t.Fatalf("GetAllCards: %v", err)
	}
	if len(all) != 1 || all[0].ID != id {
		t.Fatalf("expected exactly one card (id %d), got %+v", id, all)
	}
	d2.Close()
}

// The retired "planned" status is folded into "todo" on init.
func TestPlannedStatusFoldedToTodo(t *testing.T) {
	d := openTestDB(t)
	id, err := d.CreateCard("legacy planned card", "", "planned", "medium", nil, nil, nil)
	if err != nil {
		t.Fatalf("CreateCard: %v", err)
	}
	// Write the retired status directly to simulate a pre-existing row.
	if _, err := d.conn.Exec(`UPDATE cards SET status = 'planned' WHERE id = ?`, id); err != nil {
		t.Fatalf("forcing planned status: %v", err)
	}

	if err := d.migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	card, err := d.GetCard(id)
	if err != nil {
		t.Fatalf("GetCard: %v", err)
	}
	if card == nil {
		t.Fatal("expected card to exist")
	}
	if card.Status != "todo" {
		t.Errorf("expected status %q, got %q", "todo", card.Status)
	}
}

// UpdateCardStatus must write nothing when the status is unchanged, or a
// repeated move would reset the aging clock.
func TestUpdateCardStatusNoOpWhenUnchanged(t *testing.T) {
	d := openTestDB(t)
	id, err := d.CreateCard("card", "desc", "todo", "medium", nil, nil, nil)
	if err != nil {
		t.Fatalf("CreateCard: %v", err)
	}

	before, err := d.GetCard(id)
	if err != nil {
		t.Fatalf("GetCard before: %v", err)
	}

	time.Sleep(2 * time.Millisecond) // a stray write would get a distinct timestamp

	if err := d.UpdateCardStatus(id, "todo"); err != nil {
		t.Fatalf("UpdateCardStatus: %v", err)
	}

	after, err := d.GetCard(id)
	if err != nil {
		t.Fatalf("GetCard after: %v", err)
	}

	if !reflect.DeepEqual(before, after) {
		t.Errorf("expected byte-identical card row, before=%+v after=%+v", before, after)
	}
	if before.UpdatedAt != after.UpdatedAt {
		t.Errorf("updated_at changed on a no-op status update: %q -> %q", before.UpdatedAt, after.UpdatedAt)
	}
	if before.Position != after.Position {
		t.Errorf("position changed on a no-op status update: %d -> %d", before.Position, after.Position)
	}
}

func TestUpdateCardStatusMovesAndStampsAging(t *testing.T) {
	d := openTestDB(t)
	id, err := d.CreateCard("card", "desc", "todo", "medium", nil, nil, nil)
	if err != nil {
		t.Fatalf("CreateCard: %v", err)
	}
	before, err := d.GetCard(id)
	if err != nil {
		t.Fatalf("GetCard: %v", err)
	}

	if err := d.UpdateCardStatus(id, "ongoing"); err != nil {
		t.Fatalf("UpdateCardStatus: %v", err)
	}

	after, err := d.GetCard(id)
	if err != nil {
		t.Fatalf("GetCard: %v", err)
	}
	if after.Status != "ongoing" {
		t.Fatalf("expected status %q, got %q", "ongoing", after.Status)
	}
	if after.StatusChangedAt == nil || *after.StatusChangedAt == "" {
		t.Fatalf("expected status_changed_at to be stamped, got %v", after.StatusChangedAt)
	}
	if before.UpdatedAt == after.UpdatedAt {
		t.Errorf("expected updated_at to change on a real status move")
	}
}

// GetRevision changes on card mutations but not on note create/update/delete.
func TestGetRevisionExcludesNotes(t *testing.T) {
	d := openTestDB(t)

	rev1, err := d.GetRevision()
	if err != nil {
		t.Fatalf("GetRevision: %v", err)
	}

	id, err := d.CreateCard("card", "desc", "todo", "medium", nil, nil, nil)
	if err != nil {
		t.Fatalf("CreateCard: %v", err)
	}
	rev2, err := d.GetRevision()
	if err != nil {
		t.Fatalf("GetRevision: %v", err)
	}
	if rev1 == rev2 {
		t.Fatalf("expected revision to change after CreateCard, got identical %+v", rev1)
	}

	noteID, err := d.CreateNote("scratch idea")
	if err != nil {
		t.Fatalf("CreateNote: %v", err)
	}
	rev3, err := d.GetRevision()
	if err != nil {
		t.Fatalf("GetRevision: %v", err)
	}
	if rev2 != rev3 {
		t.Fatalf("expected revision to stay the same after CreateNote, got %+v -> %+v", rev2, rev3)
	}

	if err := d.UpdateNote(noteID, "updated idea"); err != nil {
		t.Fatalf("UpdateNote: %v", err)
	}
	rev4, err := d.GetRevision()
	if err != nil {
		t.Fatalf("GetRevision: %v", err)
	}
	if rev3 != rev4 {
		t.Fatalf("expected revision to stay the same after UpdateNote, got %+v -> %+v", rev3, rev4)
	}

	if err := d.DeleteNote(noteID); err != nil {
		t.Fatalf("DeleteNote: %v", err)
	}
	rev5, err := d.GetRevision()
	if err != nil {
		t.Fatalf("GetRevision: %v", err)
	}
	if rev4 != rev5 {
		t.Fatalf("expected revision to stay the same after DeleteNote, got %+v -> %+v", rev4, rev5)
	}

	if err := d.DeleteCard(id); err != nil {
		t.Fatalf("DeleteCard: %v", err)
	}
	rev6, err := d.GetRevision()
	if err != nil {
		t.Fatalf("GetRevision: %v", err)
	}
	if rev6 == rev5 {
		t.Fatalf("expected revision to change after DeleteCard")
	}
}

func TestLabelCRUDAndProtection(t *testing.T) {
	d := openTestDB(t)

	id, err := d.CreateLabel("backend")
	if err != nil {
		t.Fatalf("CreateLabel: %v", err)
	}

	cardID, err := d.CreateCard("card", "", "todo", "medium", nil, strPtr("backend"), nil)
	if err != nil {
		t.Fatalf("CreateCard: %v", err)
	}

	if err := d.UpdateLabel(id, "server"); err != nil {
		t.Fatalf("UpdateLabel: %v", err)
	}
	card, err := d.GetCard(cardID)
	if err != nil {
		t.Fatalf("GetCard: %v", err)
	}
	if card.Label == nil || *card.Label != "server" {
		t.Fatalf("expected rename to cascade onto card, got %v", card.Label)
	}

	if err := d.EnsureLabel("server"); err != nil {
		t.Fatalf("EnsureLabel (existing): %v", err)
	}
	labels, err := d.GetLabels()
	if err != nil {
		t.Fatalf("GetLabels: %v", err)
	}
	count := 0
	for _, l := range labels {
		if l.Name == "server" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected EnsureLabel to be a no-op for an existing name, got %d matches", count)
	}

	if err := d.DeleteLabel(id); err != nil {
		t.Fatalf("DeleteLabel: %v", err)
	}
	card, err = d.GetCard(cardID)
	if err != nil {
		t.Fatalf("GetCard: %v", err)
	}
	if card.Label != nil {
		t.Fatalf("expected label delete to cascade to NULL on card, got %v", card.Label)
	}
}

func TestNoteCRUD(t *testing.T) {
	d := openTestDB(t)

	id1, err := d.CreateNote("first")
	if err != nil {
		t.Fatalf("CreateNote: %v", err)
	}
	id2, err := d.CreateNote("second")
	if err != nil {
		t.Fatalf("CreateNote: %v", err)
	}

	notes, err := d.GetNotes()
	if err != nil {
		t.Fatalf("GetNotes: %v", err)
	}
	if len(notes) != 2 || notes[0].ID != id2 || notes[1].ID != id1 {
		t.Fatalf("expected newest-first order [%d, %d], got %+v", id2, id1, notes)
	}

	if err := d.UpdateNote(id1, "first, edited"); err != nil {
		t.Fatalf("UpdateNote: %v", err)
	}
	n, err := d.GetNote(id1)
	if err != nil {
		t.Fatalf("GetNote: %v", err)
	}
	if n == nil || n.Text != "first, edited" {
		t.Fatalf("expected updated text, got %+v", n)
	}

	if err := d.DeleteNote(id1); err != nil {
		t.Fatalf("DeleteNote: %v", err)
	}
	n, err = d.GetNote(id1)
	if err != nil {
		t.Fatalf("GetNote after delete: %v", err)
	}
	if n != nil {
		t.Fatalf("expected nil after delete, got %+v", n)
	}
}

func TestCreateCardPositionSequencing(t *testing.T) {
	d := openTestDB(t)
	id1, err := d.CreateCard("first", "", "todo", "medium", nil, nil, nil)
	if err != nil {
		t.Fatalf("CreateCard: %v", err)
	}
	id2, err := d.CreateCard("second", "", "todo", "medium", nil, nil, nil)
	if err != nil {
		t.Fatalf("CreateCard: %v", err)
	}

	c1, _ := d.GetCard(id1)
	c2, _ := d.GetCard(id2)
	if c1.Position != 0 {
		t.Errorf("expected first card position 0, got %d", c1.Position)
	}
	if c2.Position != 1 {
		t.Errorf("expected second card position 1, got %d", c2.Position)
	}
	if c1.Labels != "" || c2.Labels != "" {
		t.Errorf("expected legacy labels column to always be empty string")
	}
}

func TestMoveCardPosition(t *testing.T) {
	d := openTestDB(t)
	id1, _ := d.CreateCard("first", "", "todo", "medium", nil, nil, nil)
	id2, _ := d.CreateCard("second", "", "todo", "medium", nil, nil, nil)
	id3, _ := d.CreateCard("third", "", "todo", "medium", nil, nil, nil)

	// Middle card moves ahead of the first.
	moved, err := d.MoveCardPosition(id2, -1)
	if err != nil {
		t.Fatalf("MoveCardPosition: %v", err)
	}
	if !moved {
		t.Fatalf("expected swap-up to report moved=true")
	}
	cards, _ := d.GetAllCards()
	gotOrder := []int64{cards[0].ID, cards[1].ID, cards[2].ID}
	wantOrder := []int64{id2, id1, id3}
	if !reflect.DeepEqual(gotOrder, wantOrder) {
		t.Fatalf("after swap-up: got order %v, want %v", gotOrder, wantOrder)
	}

	// id2 is now first; moving up again is a no-op.
	moved, err = d.MoveCardPosition(id2, -1)
	if err != nil {
		t.Fatalf("MoveCardPosition at top: %v", err)
	}
	if moved {
		t.Fatalf("expected no-op at top of column, got moved=true")
	}
	cards, _ = d.GetAllCards()
	gotOrder = []int64{cards[0].ID, cards[1].ID, cards[2].ID}
	if !reflect.DeepEqual(gotOrder, wantOrder) {
		t.Fatalf("no-op at top mutated order: got %v, want %v", gotOrder, wantOrder)
	}

	// Deleting the middle card leaves a position gap; the neighbor must still
	// be found by nearest position, not position±1.
	if err := d.DeleteCard(id1); err != nil {
		t.Fatalf("DeleteCard: %v", err)
	}
	moved, err = d.MoveCardPosition(id2, 1)
	if err != nil {
		t.Fatalf("MoveCardPosition across a position gap: %v", err)
	}
	if !moved {
		t.Fatalf("expected swap-down across the gap to report moved=true")
	}
	cards, _ = d.GetAllCards()
	gotOrder = []int64{cards[0].ID, cards[1].ID}
	wantOrder = []int64{id3, id2}
	if !reflect.DeepEqual(gotOrder, wantOrder) {
		t.Fatalf("after swap across gap: got order %v, want %v", gotOrder, wantOrder)
	}
}

func strPtr(s string) *string { return &s }

// migrate() drops the legacy gitlab_* columns and their data, keeps
// everything else, and is a no-op the second time.
func TestMigrateDropsLegacyGitlabColumns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kanban.db")
	d, err := InitDB(path)
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	id, err := d.CreateCard("keep me", "desc", "ongoing", "high", nil, nil, nil)
	if err != nil {
		t.Fatalf("CreateCard: %v", err)
	}
	for _, name := range legacyCardColumns {
		if _, err := d.conn.Exec("ALTER TABLE cards ADD COLUMN " + name + " TEXT DEFAULT NULL"); err != nil {
			t.Fatalf("adding legacy column %s: %v", name, err)
		}
	}
	if _, err := d.conn.Exec(`UPDATE cards SET gitlab_mr_url='https://gitlab.example/mr/1', gitlab_mr_state='merged' WHERE id=?`, id); err != nil {
		t.Fatalf("populating legacy columns: %v", err)
	}
	d.Close()

	d2, err := InitDB(path)
	if err != nil {
		t.Fatalf("InitDB on legacy schema: %v", err)
	}
	defer d2.Close()
	if err := d2.migrate(); err != nil {
		t.Fatalf("second migrate: %v", err)
	}

	cols, err := d2.cardColumns()
	if err != nil {
		t.Fatalf("cardColumns: %v", err)
	}
	for _, name := range legacyCardColumns {
		if cols[name] {
			t.Errorf("legacy column %q still present after migration", name)
		}
	}
	card, err := d2.GetCard(id)
	if err != nil {
		t.Fatalf("GetCard: %v", err)
	}
	if card.Title != "keep me" || card.Status != "ongoing" || card.Priority != "high" {
		t.Errorf("card data changed by migration: %+v", card)
	}
}

func TestNormalizeJiraKey(t *testing.T) {
	good := map[string]string{
		"":                                     "",
		"  ":                                   "",
		"prj-123":                              "PRJ-123",
		" PRJ-1688 ":                           "PRJ-1688",
		"https://jira.example.com/browse/A1-9": "A1-9",
		"https://jira.example.com/browse/PRJ-5?focusedId=1": "PRJ-5",
	}
	for in, want := range good {
		got, err := NormalizeJiraKey(in)
		if err != nil || got != want {
			t.Errorf("NormalizeJiraKey(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"PRJ", "123", "PRJ-", "PRJ-x", "two words-1", "-1"} {
		if _, err := NormalizeJiraKey(bad); err == nil {
			t.Errorf("NormalizeJiraKey(%q): expected an error", bad)
		}
	}
}

func TestJiraKeyRoundTripAndSearch(t *testing.T) {
	d := openTestDB(t)
	id, err := d.CreateCard("linked", "body", "todo", "medium", nil, nil, strPtr("PRJ-42"))
	if err != nil {
		t.Fatalf("CreateCard: %v", err)
	}
	card, _ := d.GetCard(id)
	if card.JiraKey == nil || *card.JiraKey != "PRJ-42" {
		t.Fatalf("jira key after create: %v", card.JiraKey)
	}
	if !CardMatchesSearch(*card, "prj-42") || !CardMatchesSearch(*card, "PRJ-4") {
		t.Error("search should match the Jira key")
	}
	if CardMatchesSearch(*card, "PRJ-99") {
		t.Error("search matched a different key")
	}

	if err := d.UpdateCard(id, "linked", "body", "medium", nil, nil, nil); err != nil {
		t.Fatalf("UpdateCard: %v", err)
	}
	card, _ = d.GetCard(id)
	if card.JiraKey != nil {
		t.Errorf("jira key after clearing: %v", *card.JiraKey)
	}
}
