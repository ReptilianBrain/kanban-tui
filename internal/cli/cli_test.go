package cli

import (
	"path/filepath"
	"testing"

	"github.com/ReptilianBrain/kanban-tui/internal/db"
)

func TestValidateDue(t *testing.T) {
	cmd := newRootCmd()
	if err := validateDue(cmd, ""); err != nil {
		t.Errorf("empty due should be valid, got %v", err)
	}
	if err := validateDue(cmd, "2026-01-05"); err != nil {
		t.Errorf("valid due date rejected: %v", err)
	}
	for _, bad := range []string{"not-a-date", "2026/01/05", "2026-13-01", "2026-02-30"} {
		if err := validateDue(cmd, bad); err == nil {
			t.Errorf("expected error for due date %q", bad)
		}
	}
}

func TestDuePhraseAndStatus(t *testing.T) {
	if got := duePhrase(nil); got != "" {
		t.Errorf("nil due: got %q", got)
	}
	empty := ""
	if got := duePhrase(&empty); got != "" {
		t.Errorf("empty due: got %q", got)
	}
	future := "2099-01-01"
	if got := dueStatus(&future); got != "future" {
		t.Errorf("future due status: got %q", got)
	}
	if got := duePhrase(&future); got != "due 2099-01-01" {
		t.Errorf("future due phrase: got %q", got)
	}
}

func TestPossibleDuplicate(t *testing.T) {
	cards := []db.Card{
		{ID: 1, Title: "Fix login bug", Status: "todo"},
		{ID: 2, Title: "Completed dup title match", Status: "completed"},
		{ID: 3, Title: "abc", Status: "todo"}, // too short to match
	}
	if dup := possibleDuplicate("Fix login bug urgently", cards); dup == nil || dup.ID != 1 {
		t.Errorf("expected match on card 1, got %+v", dup)
	}
	if dup := possibleDuplicate("Completed dup title match", cards); dup != nil {
		t.Errorf("completed cards must be skipped, got %+v", dup)
	}
	if dup := possibleDuplicate("xyz", cards); dup != nil {
		t.Errorf("short titles must be skipped, got %+v", dup)
	}
	if dup := possibleDuplicate("totally unrelated", cards); dup != nil {
		t.Errorf("expected no match, got %+v", dup)
	}
}

func TestRuneTruncate(t *testing.T) {
	s, truncated := runeTruncate("hello", 10)
	if s != "hello" || truncated {
		t.Errorf("short string should not truncate, got %q %v", s, truncated)
	}
	s, truncated = runeTruncate("hello world", 5)
	if s != "hello" || !truncated {
		t.Errorf("expected truncation, got %q %v", s, truncated)
	}
	// Multi-byte runes count as one, not per byte.
	s, truncated = runeTruncate("héllo wörld", 6)
	if len([]rune(s)) != 6 || !truncated {
		t.Errorf("expected 6-rune truncation, got %q (%d runes) %v", s, len([]rune(s)), truncated)
	}
}

func TestSplitLines(t *testing.T) {
	got := splitLines("a\nb\r\nc\rd")
	want := []string{"a", "b", "c", "d"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d: got %q, want %q", i, got[i], want[i])
		}
	}
	if got := splitLines(""); got != nil {
		t.Errorf("empty string: got %v", got)
	}
}

// The Execute tests pin the exit-code contract: 1 for business errors, 2 for
// usage errors (bad choice, non-numeric id).

func testDBPath(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "kanban.db")
	t.Setenv("KANBANTUI_DB_PATH", path)
	return path
}

func TestExecuteExitCodes(t *testing.T) {
	testDBPath(t)

	if code := Execute([]string{"add", "Test card"}); code != 0 {
		t.Fatalf("add: exit code %d, want 0", code)
	}
	if code := Execute([]string{"show", "1"}); code != 0 {
		t.Fatalf("show existing card: exit code %d, want 0", code)
	}
	if code := Execute([]string{"show", "999"}); code != 1 {
		t.Fatalf("show missing card: exit code %d, want 1", code)
	}
	if code := Execute([]string{"add", "Bad status", "-s", "bogus"}); code != 2 {
		t.Fatalf("invalid choice: exit code %d, want 2", code)
	}
	if code := Execute([]string{"move", "1", "bogus"}); code != 2 {
		t.Fatalf("invalid move status: exit code %d, want 2", code)
	}
	if code := Execute([]string{"add", "Bad due", "--due", "not-a-date"}); code != 1 {
		t.Fatalf("bad due date: exit code %d, want 1", code)
	}
	if code := Execute([]string{"show", "notanumber"}); code != 2 {
		t.Fatalf("non-numeric card id: exit code %d, want 2", code)
	}
	if code := Execute([]string{"delete", "1"}); code != 0 {
		t.Fatalf("delete: exit code %d, want 0", code)
	}
	if code := Execute([]string{"delete", "1"}); code != 1 {
		t.Fatalf("delete missing: exit code %d, want 1", code)
	}
}

func TestExecuteNextEmptyBoard(t *testing.T) {
	testDBPath(t)
	if code := Execute([]string{"next"}); code != 0 {
		t.Fatalf("next on empty board: exit code %d, want 0", code)
	}
}

func TestExecuteNoteLifecycle(t *testing.T) {
	testDBPath(t)
	if code := Execute([]string{"note", "hello"}); code != 0 {
		t.Fatalf("note: exit code %d, want 0", code)
	}
	if code := Execute([]string{"note-delete", "1"}); code != 0 {
		t.Fatalf("note-delete: exit code %d, want 0", code)
	}
	if code := Execute([]string{"note-delete", "1"}); code != 1 {
		t.Fatalf("note-delete missing: exit code %d, want 1", code)
	}
}

func TestExecuteJiraFlag(t *testing.T) {
	path := testDBPath(t)
	if code := Execute([]string{"add", "Linked", "--jira", "prj-12"}); code != 0 {
		t.Fatalf("add --jira: exit code %d, want 0", code)
	}
	if code := Execute([]string{"add", "Bad", "--jira", "nope"}); code != 1 {
		t.Fatalf("add with a bad key: exit code %d, want 1", code)
	}

	jiraOf := func() *string {
		t.Helper()
		d, err := db.InitDB(path)
		if err != nil {
			t.Fatal(err)
		}
		defer d.Close()
		c, err := d.GetCard(1)
		if err != nil || c == nil {
			t.Fatalf("GetCard: %v, %v", c, err)
		}
		return c.JiraKey
	}
	if k := jiraOf(); k == nil || *k != "PRJ-12" {
		t.Fatalf("jira key after add = %v, want PRJ-12", k)
	}
	if code := Execute([]string{"update", "1", "-t", "Renamed"}); code != 0 {
		t.Fatalf("update: exit code %d", code)
	}
	if k := jiraOf(); k == nil || *k != "PRJ-12" {
		t.Fatalf("an update that omits --jira must keep it, got %v", k)
	}
	if code := Execute([]string{"update", "1", "--jira", ""}); code != 0 {
		t.Fatalf("update --jira '': exit code %d", code)
	}
	if k := jiraOf(); k != nil {
		t.Fatalf("jira key after clearing = %v, want nil", *k)
	}
}
