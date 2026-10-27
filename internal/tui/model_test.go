package tui

import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ReptilianBrain/kanban-tui/internal/db"
)

// seedCard is one card of the synthetic test board.
type seedCard struct {
	title, desc, status, priority, label string
}

// seedCards is the board every test runs against. Every lane is populated and
// labels repeat inside a lane, so the label-filter and search tests always
// have a strict subset to narrow to.
var seedCards = []seedCard{
	{"Fix login redirect loop", "Users bounce between /login and /home.", "todo", "high", "backend"},
	{"Add CSV export", "Export the board as a CSV file.", "todo", "medium", "backend"},
	{"Dark mode for the settings page", "", "todo", "low", "frontend"},
	{"Write onboarding guide", "A short guide for new contributors.", "todo", "medium", ""},
	{"Migrate cache layer", "Swap the in-memory cache for Redis.", "ongoing", "critical", "backend"},
	{"Redesign the card form", "Fewer fields, clearer labels.", "ongoing", "medium", "frontend"},
	{"Profile slow dashboard query", "", "ongoing", "high", "backend"},
	{"Waiting on API credentials", "Vendor has not replied yet.", "blocked", "medium", "backend"},
	{"Vendor contract review", "", "blocked", "low", ""},
	{"Pagination for the list endpoint", "Cursor based.", "in_review", "medium", "backend"},
	{"Keyboard shortcut cheat sheet", "", "in_review", "low", "frontend"},
	{"Set up CI pipeline", "Run vet and tests on every push.", "completed", "high", "backend"},
	{"Rename the project", "", "completed", "low", ""},
}

var seedLabels = []string{"backend", "frontend", "docs"}

var seedNotes = []string{
	"Look into SQLite FTS for search",
	"Is two-pane layout worth it?",
	"Try a calendar view for due dates",
}

// testStore seeds a throwaway database in t.TempDir(), so tests never touch
// the live ~/.kanbantui/kanban.db.
func testStore(t *testing.T) *db.DB {
	t.Helper()
	store, err := db.InitDB(filepath.Join(t.TempDir(), "kanban.db"))
	if err != nil {
		t.Fatalf("opening scratch db: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	for _, name := range seedLabels {
		if err := store.EnsureLabel(name); err != nil {
			t.Fatalf("seeding label %q: %v", name, err)
		}
	}
	for _, c := range seedCards {
		var label *string
		if c.label != "" {
			l := c.label
			label = &l
		}
		if _, err := store.CreateCard(c.title, c.desc, c.status, c.priority, nil, label); err != nil {
			t.Fatalf("seeding card %q: %v", c.title, err)
		}
	}
	for _, text := range seedNotes {
		if _, err := store.CreateNote(text); err != nil {
			t.Fatalf("seeding note: %v", err)
		}
	}
	return store
}

// newTestModel loads the seeded board and sets a generous terminal size so
// View() skips its zero-width guard.
func newTestModel(t *testing.T) *Model {
	t.Helper()
	store := testStore(t)
	m := newModel(store)
	m.costDir = t.TempDir() // never read the real ~/.kanbantui/cost
	if err := m.loadData(); err != nil {
		t.Fatalf("loadData: %v", err)
	}
	m.Update(tea.WindowSizeMsg{Width: 220, Height: 60})
	return m
}

func key(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "shift+tab":
		return tea.KeyMsg{Type: tea.KeyShiftTab}
	case "ctrl+s":
		return tea.KeyMsg{Type: tea.KeyCtrlS}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "left":
		return tea.KeyMsg{Type: tea.KeyLeft}
	case "right":
		return tea.KeyMsg{Type: tea.KeyRight}
	case "shift+left":
		return tea.KeyMsg{Type: tea.KeyShiftLeft}
	case "shift+right":
		return tea.KeyMsg{Type: tea.KeyShiftRight}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

// send delivers one keypress via Update and deliberately does not run the
// returned Cmd. Every state change (DB writes, modal push/pop, cursor moves)
// happens synchronously inside Update; the Cmds are only toast-clear timers,
// which would block the test on a real timer (tea.Tick's Cmd waits on a
// channel when invoked directly), and herdr launches, which must not run.
func send(m *Model, s string) {
	m.Update(key(s))
}

func typeText(m *Model, s string) {
	for _, r := range s {
		send(m, string(r))
	}
}

func TestSeedFixtureCanary(t *testing.T) {
	store := testStore(t)
	cards, err := store.GetAllCards()
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != len(seedCards) {
		t.Errorf("expected %d cards in the fixture, got %d", len(seedCards), len(cards))
	}
	labels, err := store.GetLabels()
	if err != nil {
		t.Fatal(err)
	}
	if len(labels) != len(seedLabels) {
		t.Errorf("expected %d labels in the fixture, got %d", len(seedLabels), len(labels))
	}
	notes, err := store.GetNotes()
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != len(seedNotes) {
		t.Errorf("expected %d notes in the fixture, got %d", len(seedNotes), len(notes))
	}
}

// TestViewFitsNarrowTerminal guards against the footer wrapping at a normal
// width without the reserved-height math counting it, which would push the
// bottom of the board off screen.
func TestViewFitsNarrowTerminal(t *testing.T) {
	m := newTestModel(t)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	view := m.View()
	if got := lipgloss.Height(view); got > 24 {
		t.Errorf("View() at 80x24 rendered %d lines, want <= 24", got)
	}
}

// TestViewFitsNarrowTerminalWithLongToast uses a toast long enough to wrap at
// 80 columns unless oneLine truncates it (an empty toast trivially fits).
func TestViewFitsNarrowTerminalWithLongToast(t *testing.T) {
	m := newTestModel(t)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.toast(toastWarn, strings.Repeat("x", 200))
	if got := lipgloss.Height(m.View()); got > 24 {
		t.Errorf("View() at 80x24 with a long toast rendered %d lines, want <= 24", got)
	}
}

// TestViewFitsNarrowTerminalWithLongSearchTitle covers the title bar, which
// embeds the active search query verbatim.
func TestViewFitsNarrowTerminalWithLongSearchTitle(t *testing.T) {
	m := newTestModel(t)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.searchQuery = strings.Repeat("y", 200)
	if got := lipgloss.Height(m.View()); got > 24 {
		t.Errorf("View() at 80x24 with a long search query rendered %d lines, want <= 24", got)
	}
}

// ── Initial load ──

func TestInitialLoadCounts(t *testing.T) {
	m := newTestModel(t)
	cards, err := m.store.GetAllCards()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]int{}
	for _, c := range cards {
		want[c.Status]++
	}
	for _, s := range statuses {
		if got := len(m.data[s]); got != want[s] {
			t.Errorf("column %s: got %d cards, want %d", s, got, want[s])
		}
	}
	if v := m.View(); v == "" {
		t.Error("View() returned empty string after initial load")
	}
}

// ── Navigation ──

func TestColumnAndCardNavigation(t *testing.T) {
	m := newTestModel(t)

	if m.col != 0 {
		t.Fatalf("expected initial column 0, got %d", m.col)
	}
	send(m, "left") // already at leftmost column with ends shown — no-op
	if m.col != 0 {
		t.Errorf("previous-column at column 0 should be a no-op, got col=%d", m.col)
	}

	send(m, "right")
	if m.col != 1 {
		t.Errorf("expected col 1 after one next-column, got %d", m.col)
	}
	for i := 0; i < 10; i++ {
		send(m, "right")
	}
	if m.col != len(statuses)-1 {
		t.Errorf("next-column should cap at %d, got %d", len(statuses)-1, m.col)
	}
	for i := 0; i < 10; i++ {
		send(m, "left")
	}
	if m.col != 0 {
		t.Errorf("previous-column should floor at 0, got %d", m.col)
	}

	// Row navigation within whichever column currently has the most cards.
	bestStatus, bestCount := statuses[0], -1
	for _, s := range statuses {
		if n := len(m.data[s]); n > bestCount {
			bestStatus, bestCount = s, n
		}
	}
	if bestCount < 2 {
		t.Skip("fixture doesn't have a column with >=2 cards to test row navigation")
	}
	for i, s := range statuses {
		if s == bestStatus {
			m.col = i
		}
	}
	m.row = 0
	send(m, "up") // already at row 0 — no-op
	if m.row != 0 {
		t.Errorf("card_up at row 0 should be a no-op, got row=%d", m.row)
	}
	send(m, "down")
	if m.row != 1 {
		t.Errorf("expected row 1 after one card_down, got %d", m.row)
	}
	for i := 0; i < bestCount+5; i++ {
		send(m, "down")
	}
	if m.row != bestCount-1 {
		t.Errorf("card_down should cap at last row %d, got %d", bestCount-1, m.row)
	}
}

func TestHiddenEndsColumnClamp(t *testing.T) {
	m := newTestModel(t)
	m.col = 0
	send(m, "c") // hide Backlog/Completed
	if m.showEnds {
		t.Fatal("expected showEnds=false after toggling")
	}
	if m.col != 1 {
		t.Errorf("toggling ends while on Backlog should move col to 1 (ongoing), got %d", m.col)
	}

	// Moving left must floor at 1 (ongoing) while ends are hidden.
	send(m, "left")
	if m.col != 1 {
		t.Errorf("previous-column should floor at 1 while ends hidden, got %d", m.col)
	}

	// Moving right must cap at 3 (in_review) while ends are hidden.
	for i := 0; i < 5; i++ {
		send(m, "right")
	}
	if m.col != 3 {
		t.Errorf("next-column should cap at 3 while ends hidden, got %d", m.col)
	}

	m.col = len(statuses) - 1 // completed
	send(m, "c")              // show ends again, then hide from Completed
	send(m, "c")
	if m.col != len(statuses)-2 {
		t.Errorf("toggling ends while on Completed should move col to %d, got %d", len(statuses)-2, m.col)
	}
}

// ── Create card ──

func TestCreateCardEndToEnd(t *testing.T) {
	m := newTestModel(t)
	m.col = 0 // Backlog

	send(m, "n")
	if len(m.modalStack) != 1 {
		t.Fatalf("expected the card form to open, modal stack len=%d", len(m.modalStack))
	}

	typeText(m, "A Brand New Card")
	send(m, "ctrl+s")

	if len(m.modalStack) != 0 {
		t.Fatalf("expected the card form to close after save, modal stack len=%d", len(m.modalStack))
	}

	cards, err := m.store.GetAllCards()
	if err != nil {
		t.Fatal(err)
	}
	var found *db.Card
	for i := range cards {
		if cards[i].Title == "A Brand New Card" {
			found = &cards[i]
		}
	}
	if found == nil {
		t.Fatal("newly created card not found in the database")
	}
	if found.Status != "todo" {
		t.Errorf("expected new card in status todo, got %q", found.Status)
	}

	got, err := m.store.GetCard(found.ID)
	if err != nil || got == nil {
		t.Fatalf("db.GetCard(%d) failed: %v", found.ID, err)
	}
	if got.Title != "A Brand New Card" {
		t.Errorf("GetCard returned title %q", got.Title)
	}
}

func TestCreateCardRefusesEmptyTitle(t *testing.T) {
	m := newTestModel(t)
	before, _ := m.store.GetAllCards()

	send(m, "n")
	send(m, "ctrl+s") // empty title — must not save, form stays open

	if len(m.modalStack) != 1 {
		t.Fatalf("empty-title save should keep the form open, modal stack len=%d", len(m.modalStack))
	}
	send(m, "esc")

	after, _ := m.store.GetAllCards()
	if len(after) != len(before) {
		t.Errorf("expected no card created from an empty-title save attempt, before=%d after=%d", len(before), len(after))
	}
}

// ── Delete card ──

func TestDeleteCardEndToEnd(t *testing.T) {
	m := newTestModel(t)

	var target *db.Card
	var targetCol int
	for i, s := range statuses {
		if len(m.data[s]) > 0 {
			c := m.data[s][0]
			target = &c
			targetCol = i
			break
		}
	}
	if target == nil {
		t.Fatal("fixture has no cards at all")
	}
	m.col = targetCol
	m.row = 0

	send(m, "d")
	if len(m.modalStack) != 1 {
		t.Fatalf("expected delete confirmation to open, modal stack len=%d", len(m.modalStack))
	}
	send(m, "y")
	if len(m.modalStack) != 0 {
		t.Fatalf("expected delete confirmation to close, modal stack len=%d", len(m.modalStack))
	}

	got, err := m.store.GetCard(target.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Errorf("card %d still exists after confirmed delete", target.ID)
	}
}

func TestDeleteCardCancelKeepsCard(t *testing.T) {
	m := newTestModel(t)
	var target *db.Card
	for _, s := range statuses {
		if len(m.data[s]) > 0 {
			c := m.data[s][0]
			target = &c
			break
		}
	}
	if target == nil {
		t.Fatal("fixture has no cards at all")
	}

	send(m, "d")
	send(m, "n") // "No"

	got, err := m.store.GetCard(target.ID)
	if err != nil || got == nil {
		t.Errorf("card %d should still exist after cancelling delete: err=%v", target.ID, err)
	}
}

// ── Label filter / search ──

func TestLabelFilterNarrowsVisibleCards(t *testing.T) {
	m := newTestModel(t)

	// Find a status/label pair where the label matches a strict subset of
	// that column's cards.
	var chosenStatus, chosenLabel string
	for _, s := range statuses {
		total := len(m.data[s])
		if total < 2 {
			continue
		}
		counts := map[string]int{}
		for _, c := range m.data[s] {
			if c.Label != nil && *c.Label != "" {
				counts[*c.Label]++
			}
		}
		for name, n := range counts {
			if n > 0 && n < total {
				chosenStatus, chosenLabel = s, name
				break
			}
		}
		if chosenLabel != "" {
			break
		}
	}
	if chosenLabel == "" {
		t.Skip("fixture has no column with a label covering a strict subset of its cards")
	}

	m.labelFilter = chosenLabel
	m.clampRow()

	visible := m.visibleCards(chosenStatus)
	total := len(m.data[chosenStatus])
	if len(visible) == 0 || len(visible) >= total {
		t.Fatalf("expected the label filter to narrow %q from %d, got %d visible", chosenStatus, total, len(visible))
	}
	for _, c := range visible {
		if c.Label == nil || *c.Label != chosenLabel {
			t.Errorf("visible card %d does not carry the filtered label", c.ID)
		}
	}
}

// searchNeedleFor returns a lowercase query drawn from a card's title that
// matches some but not all of candidates, or "" if none qualifies. It lets
// the narrowing tests assert on a query that provably narrows the set.
func searchNeedleFor(candidates []db.Card) string {
	for _, target := range candidates {
		needle := strings.ToLower(strings.TrimSpace(target.Title))
		if len(needle) < 4 {
			continue
		}
		matches := 0
		for _, c := range candidates {
			if db.CardMatchesSearch(c, needle) {
				matches++
			}
		}
		if matches > 0 && matches < len(candidates) {
			return needle
		}
	}
	return ""
}

func TestSearchQueryNarrowsVisibleCards(t *testing.T) {
	m := newTestModel(t)

	var chosenStatus, needle string
	for _, s := range statuses {
		if n := searchNeedleFor(m.data[s]); n != "" {
			chosenStatus, needle = s, n
			break
		}
	}
	if needle == "" {
		t.Skip("fixture has no column where a card's own title fails to match every sibling card")
	}

	m.searchQuery = needle
	m.clampRow()

	visible := m.visibleCards(chosenStatus)
	total := len(m.data[chosenStatus])
	if len(visible) == 0 || len(visible) >= total {
		t.Fatalf("expected the search query %q to narrow %q from %d, got %d visible", needle, chosenStatus, total, len(visible))
	}
	for _, c := range visible {
		if !db.CardMatchesSearch(c, needle) {
			t.Errorf("visible card %d does not actually match the search query %q", c.ID, needle)
		}
	}
}

func TestLabelFilterAndSearchCombine(t *testing.T) {
	m := newTestModel(t)

	var chosenStatus, chosenLabel, needle string
	for _, s := range statuses {
		byLabel := map[string][]db.Card{}
		for _, c := range m.data[s] {
			if c.Label != nil && *c.Label != "" {
				byLabel[*c.Label] = append(byLabel[*c.Label], c)
			}
		}
		for name, withLabel := range byLabel {
			if len(withLabel) < 2 {
				continue
			}
			if n := searchNeedleFor(withLabel); n != "" {
				chosenStatus, chosenLabel, needle = s, name, n
				break
			}
		}
		if needle != "" {
			break
		}
	}
	if needle == "" {
		t.Skip("fixture has no label with >=2 cards in one column where a title fails to match a sibling")
	}

	m.labelFilter = chosenLabel
	labelOnly := len(m.visibleCards(chosenStatus))
	if labelOnly < 2 {
		t.Fatalf("expected the label filter alone to match at least 2 cards in %q, got %d", chosenStatus, labelOnly)
	}

	m.searchQuery = needle
	combined := m.visibleCards(chosenStatus)
	if len(combined) == 0 || len(combined) >= labelOnly {
		t.Fatalf("expected label+search combined (%d) to be a non-empty strict subset of label-only (%d)", len(combined), labelOnly)
	}
	for _, c := range combined {
		if c.Label == nil || *c.Label != chosenLabel {
			t.Errorf("combined-filter card %d does not carry the filtered label", c.ID)
		}
		if !db.CardMatchesSearch(c, needle) {
			t.Errorf("combined-filter card %d does not match the search query", c.ID)
		}
	}
}

// ── Inline live search ──

func typeRunes(m *Model, s string) {
	for _, r := range s {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
}

func totalVisible(m *Model) int {
	n := 0
	for _, s := range statuses {
		n += len(m.visibleCards(s))
	}
	return n
}

func TestLiveSearchFiltersPerKeystrokeAndEnterKeeps(t *testing.T) {
	m := newTestModel(t)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	all := totalVisible(m)
	var needle string
	for _, s := range statuses {
		if needle = searchNeedleFor(m.data[s]); needle != "" {
			break
		}
	}
	if needle == "" {
		t.Skip("no usable search needle in the test database")
	}

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	if !m.searching {
		t.Fatal("/ should open the inline search bar")
	}
	typeRunes(m, needle)
	if m.searchQuery != needle {
		t.Fatalf("searchQuery = %q, want %q (live, no Enter)", m.searchQuery, needle)
	}
	if got := totalVisible(m); got >= all {
		t.Errorf("typing %q left %d visible cards of %d; expected the board to narrow", needle, got, all)
	}

	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.searching || m.searchQuery != needle {
		t.Errorf("Enter should keep the filter and end typing; searching=%v query=%q", m.searching, m.searchQuery)
	}
	if got := lipgloss.Height(m.View()); got > 40 {
		t.Errorf("View() with search bar rendered %d lines, want <= 40", got)
	}
}

func TestLiveSearchEscClears(t *testing.T) {
	m := newTestModel(t)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	all := totalVisible(m)
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	typeRunes(m, "zzqx")
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.searching || m.searchQuery != "" {
		t.Errorf("Esc should clear and close; searching=%v query=%q", m.searching, m.searchQuery)
	}
	if got := totalVisible(m); got != all {
		t.Errorf("after Esc %d cards visible, want %d", got, all)
	}
}

func TestLiveSearchArrowsSwitchColumns(t *testing.T) {
	m := newTestModel(t)
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	start := m.col
	m.Update(tea.KeyMsg{Type: tea.KeyRight})
	if m.col != start+1 || !m.searching {
		t.Errorf("Right while searching: col=%d (want %d), searching=%v", m.col, start+1, m.searching)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if m.col != start {
		t.Errorf("Left while searching: col=%d, want %d", m.col, start)
	}
}

func TestColumnsSortedNewestCreatedFirst(t *testing.T) {
	m := newTestModel(t)
	m.col = 0

	send(m, "n")
	typeText(m, "Freshest Card")
	send(m, "ctrl+s")

	if got := m.data["todo"][0].Title; got != "Freshest Card" {
		t.Errorf("expected the newest card on top of Backlog, got %q", got)
	}
	for status, cards := range m.data {
		if status == "completed" {
			continue
		}
		for i := 1; i < len(cards); i++ {
			prev, cur := cards[i-1], cards[i]
			if prev.CreatedAt < cur.CreatedAt || (prev.CreatedAt == cur.CreatedAt && prev.ID < cur.ID) {
				t.Errorf("%s: card #%d (%s) sorted before newer #%d (%s)",
					status, prev.ID, prev.CreatedAt, cur.ID, cur.CreatedAt)
			}
		}
	}
}

// Columns sort by created_at, so a moved card does not land at the bottom of
// the destination column. The cursor must follow the card by id, otherwise a
// quick second Shift+→ moves the wrong card.
func TestMoveCardRightCursorFollowsCard(t *testing.T) {
	m := newTestModel(t)
	// Pick a card that is neither the newest nor the oldest of its column
	// so neither row 0 nor the last row can be right by accident.
	m.col = 1
	cards := m.visibleCards(statuses[m.col])
	if len(cards) < 3 {
		t.Skip("fixture column too small")
	}
	m.row = 1
	want := cards[1].ID

	send(m, "shift+right")
	if m.col != 2 {
		t.Fatalf("expected column 2 after move, got %d", m.col)
	}
	if c := m.currentCard(); c == nil || c.ID != want {
		t.Fatalf("cursor left the moved card after 1st move: want #%d, got %+v", want, c)
	}

	send(m, "shift+right")
	if m.col != 3 {
		t.Fatalf("expected column 3 after 2nd move, got %d", m.col)
	}
	if c := m.currentCard(); c == nil || c.ID != want {
		t.Fatalf("cursor left the moved card after 2nd move: want #%d, got %+v", want, c)
	}
	got, err := m.store.GetCard(want)
	if err != nil || got == nil || got.Status != statuses[3] {
		t.Fatalf("card #%d not in %s after double move: %+v (err %v)", want, statuses[3], got, err)
	}
}

func TestCreateCardSelectsNewCard(t *testing.T) {
	m := newTestModel(t)
	m.col = 0
	send(m, "n")
	typeText(m, "Selected On Create")
	send(m, "ctrl+s")
	if c := m.currentCard(); c == nil || c.Title != "Selected On Create" {
		t.Fatalf("expected the new card to be selected, got %+v", c)
	}
}
