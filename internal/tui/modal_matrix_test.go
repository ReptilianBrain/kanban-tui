package tui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ReptilianBrain/kanban-tui/internal/db"
)

func TestMatrix_OpensClosesAndListsInFlightCards(t *testing.T) {
	m := newTestModel(t)
	running := addCard(t, m, "ongoing")
	waiting := addCard(t, m, "ongoing")
	m.agents = map[int64]string{running: "working", waiting: "blocked"}

	send(m, "m")
	if _, ok := m.topModal().(*matrixModal); !ok {
		t.Fatalf("m should open the Matrix, top modal = %T", m.topModal())
	}
	view := m.topModal().View(m)
	for _, want := range []string{"Matrix", "working", "waiting", "in flight"} {
		if !strings.Contains(view, want) {
			t.Errorf("Matrix view lacks %q:\n%s", want, view)
		}
	}
	if rows := m.matrixRows(false, false); len(rows) < 2 || rows[0].card.ID != waiting {
		t.Errorf("the card waiting on you should sort first, rows = %+v", rows)
	}

	send(m, "esc")
	if m.topModal() != nil {
		t.Error("esc should close the Matrix")
	}
}

func TestMatrix_ShowsWaitingStateWithoutATimer(t *testing.T) {
	m := newTestModel(t)
	id := addCard(t, m, "ongoing")
	m.agents = map[int64]string{id: "blocked"}
	reconcileAt(m, 0, m.agents)
	m.clock = func() time.Time { return t0.Add(30 * time.Second) }

	send(m, "m")
	view := m.topModal().View(m)
	if !strings.Contains(view, "waiting") {
		t.Errorf("want the waiting state, got:\n%s", view)
	}
	if strings.Contains(view, "0:30") {
		t.Errorf("the wait timer should not be shown, got:\n%s", view)
	}
}

func TestMatrix_DoesNotFreezeTheWatcherButOtherModalsDo(t *testing.T) {
	m := newTestModel(t)
	id := addCard(t, m, "ongoing")
	waiting := map[int64]string{id: "blocked"}
	reconcileAt(m, 0, waiting)

	m.modalStack = []modal{newMatrixModal()}
	if m.blockingModal() {
		t.Fatal("the Matrix must not block the watcher")
	}
	reconcileAt(m, waitAfter+time.Second, waiting)
	if !m.watch.notified[id] {
		t.Error("watcher should run under the Matrix and notify")
	}

	m.modalStack = []modal{newDeleteConfirmModal("x", nil)}
	if !m.blockingModal() {
		t.Error("a delete modal must block the watcher")
	}
}

func TestMatrix_EmptyBoard(t *testing.T) {
	m := newTestModel(t)
	m.data = map[string][]db.Card{}
	if view := newMatrixModal().View(m); !strings.Contains(view, "Nothing in flight") {
		t.Errorf("empty Matrix should say so:\n%s", view)
	}
}

func TestMatrix_ShowsCycleFromTheCostFiles(t *testing.T) {
	m := newTestModel(t)
	id := addCard(t, m, "ongoing")
	m.agents = map[int64]string{id: "working"}
	if view := newMatrixModal().View(m); strings.Contains(view, "$") {
		t.Fatalf("no cost recorded yet, view should show no amount:\n%s", view)
	}

	path := filepath.Join(m.costDir, fmt.Sprintf("card-%d.json", id))
	if err := os.WriteFile(path, []byte(`{"a":1.25,"b":0.5}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if view := newMatrixModal().View(m); !strings.Contains(view, "$1.75") {
		t.Errorf("want the summed $1.75 in the Matrix:\n%s", view)
	}

	card, _ := m.store.GetCard(id)
	if view := newCardDetailModal(*card).View(m); !strings.Contains(view, "Cycle: $1.75") {
		t.Errorf("card detail lacks the Cycle:\n%s", view)
	}
}

func TestMatrix_AgentsOnlyByDefaultAndAToggles(t *testing.T) {
	m := newTestModel(t)
	withAgent := addCard(t, m, "ongoing")
	stale := addCard(t, m, "ongoing")
	m.agents = map[int64]string{withAgent: "working"}

	send(m, "m")
	view := m.topModal().View(m)
	if strings.Contains(view, fmt.Sprintf("#%d ", stale)) {
		t.Errorf("a card without an agent should be hidden by default:\n%s", view)
	}
	send(m, "a")
	if view := m.topModal().View(m); !strings.Contains(view, fmt.Sprintf("#%d ", stale)) {
		t.Errorf("a should show the agentless card:\n%s", view)
	}
}

func TestMatrix_NoAgentsHintsAtTheHiddenCards(t *testing.T) {
	m := newTestModel(t)
	addCard(t, m, "ongoing")
	m.agents = nil
	if view := newMatrixModal().View(m); !strings.Contains(view, "No agents running") {
		t.Errorf("want the no-agents hint:\n%s", view)
	}
}

// The Matrix sits on the nord1 modal surface, where the muted nord3 text
// (rgb 76;86;106) is about 1.3:1 and unreadable. Nothing in it may use it.
func TestMatrix_NoMutedTextOnTheModalSurface(t *testing.T) {
	forceTrueColor(t)
	// Read the muted color's SGR parameters from a rendered sample rather than
	// hardcoding them: Lip Gloss rounds the channels (nord3 comes out as 76;86;105).
	nord3 := strings.TrimSuffix(strings.TrimPrefix(dimStyle.Render("x"), "\x1b["), "mx\x1b[0m")
	m := newTestModel(t)
	id := addCard(t, m, "ongoing")
	addCard(t, m, "ongoing") // agentless row, shown by "a"
	m.agents = map[int64]string{id: "idle"}
	reconcileAt(m, 0, map[int64]string{id: "blocked"})

	mm := newMatrixModal()
	for _, all := range []bool{false, true} {
		mm.showAll = all
		if view := mm.View(m); strings.Contains(view, nord3) {
			t.Errorf("showAll=%v: Matrix uses nord3 text:\n%q", all, view)
		}
	}
	m.agents = nil
	mm.showAll = false
	if view := mm.View(m); strings.Contains(view, nord3) {
		t.Errorf("empty hint uses nord3 text:\n%q", view)
	}
	if strings.Contains(agentMarkup("idle"), nord3) {
		t.Error("the idle badge uses nord3 text")
	}
}

// Every modal draws on the nord1 surface, so none may use the muted nord3
// styles. A source scan keeps a future modal from reintroducing them without
// building each modal's view.
func TestModals_UseNoMutedStylesOnTheSurface(t *testing.T) {
	files, err := filepath.Glob("modal*.go")
	if err != nil || len(files) == 0 {
		t.Fatalf("no modal sources found: %v", err)
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, banned := range []string{"hintStyle.Render", "dimStyle.Render"} {
			// modalHintStyle.Render contains "HintStyle.Render", not "hintStyle.Render"
			if strings.Contains(string(src), banned) {
				t.Errorf("%s uses %s on the modal surface; use modalHintStyle or textStyle", f, banned)
			}
		}
	}
}

// matrixWithAgents opens the Matrix over two In Progress cards with agents:
// a waiting one (sorted first) and an idle one.
func matrixWithAgents(t *testing.T) (m *Model, first, second int64) {
	t.Helper()
	m = newTestModel(t)
	first = addCard(t, m, "ongoing")
	second = addCard(t, m, "ongoing")
	m.agents = map[int64]string{first: "blocked", second: "idle"}
	m.agentPanes = map[int64]string{first: "w1:p1", second: "w1:p2"}
	send(m, "m")
	return m, first, second
}

func TestMatrix_CursorMovesAndClamps(t *testing.T) {
	m, first, second := matrixWithAgents(t)
	mm := m.topModal().(*matrixModal)
	mm.View(m)
	if mm.cardID != first {
		t.Fatalf("selection starts on %d, want the first row %d", mm.cardID, first)
	}
	send(m, "up")
	if mm.cardID != first {
		t.Errorf("up at the top moved the selection to %d", mm.cardID)
	}
	send(m, "down")
	if mm.cardID != second {
		t.Errorf("down: selection = %d, want %d", mm.cardID, second)
	}
	send(m, "j")
	if mm.cardID != second {
		t.Errorf("down at the bottom moved the selection to %d", mm.cardID)
	}
	send(m, "k")
	if mm.cardID != first {
		t.Errorf("k: selection = %d, want %d", mm.cardID, first)
	}
}

func TestMatrix_SelectionFollowsTheCardWhenRowsReorder(t *testing.T) {
	m, first, second := matrixWithAgents(t)
	send(m, "down") // on the idle card
	m.agents[second] = "blocked"
	m.agents[first] = "done" // second now sorts above first
	mm := m.topModal().(*matrixModal)
	mm.View(m)
	rows := m.matrixRows(false, false)
	if rows[0].card.ID != second {
		t.Fatalf("setup: rows[0] = %d, want %d", rows[0].card.ID, second)
	}
	if mm.cardID != second || mm.cursor != 0 {
		t.Errorf("selection = card %d row %d, want card %d row 0", mm.cardID, mm.cursor, second)
	}
}

func TestMatrix_SelectedRowShowsTheBarAndHint(t *testing.T) {
	m, _, _ := matrixWithAgents(t)
	view := m.topModal().View(m)
	if strings.Count(view, "▌") != 1 {
		t.Errorf("want exactly one selection bar:\n%s", view)
	}
	if !strings.Contains(view, "Enter: jump to agent") {
		t.Errorf("hint lacks the jump key:\n%s", view)
	}
}

// runCmd executes a Cmd that is known to be synchronous (the focus stub) and
// feeds its message back through Update.
func runCmd(m *Model, cmd tea.Cmd) {
	if cmd != nil {
		m.Update(cmd())
	}
}

func TestMatrix_EnterFocusesTheSelectedAgentPane(t *testing.T) {
	m, _, _ := matrixWithAgents(t)
	var got []string
	m.focusAgent = func(target string) error { got = append(got, target); return nil }

	send(m, "down")
	_, cmd := m.Update(key("enter"))
	runCmd(m, cmd)
	if len(got) != 1 || got[0] != "w1:p2" {
		t.Fatalf("focused %v, want [w1:p2]", got)
	}
	if m.topModal() != nil {
		t.Error("the Matrix should close after a jump")
	}
	if m.quitting {
		t.Error("outside a popup the TUI must stay open")
	}
}

func TestMatrix_EnterClosesAPopup(t *testing.T) {
	m, _, _ := matrixWithAgents(t)
	m.popup = true
	m.focusAgent = func(string) error { return nil }
	_, cmd := m.Update(key("enter"))
	runCmd(m, cmd)
	if !m.quitting {
		t.Error("a popup keeps all input until the app exits, so a jump must quit it")
	}
}

func TestMatrix_EnterWithoutAnAgentToastsAndFocusesNothing(t *testing.T) {
	m := newTestModel(t)
	id := addCard(t, m, "ongoing")
	m.focusAgent = func(string) error { t.Error("focus called for an agentless card"); return nil }
	send(m, "m")
	send(m, "a") // widen to cards without an agent
	mm := m.topModal().(*matrixModal)
	mm.cardID = id
	send(m, "enter")
	if !strings.Contains(m.toastMsg, "No agent") {
		t.Errorf("toast = %q, want a no-agent warning", m.toastMsg)
	}
	if m.topModal() == nil {
		t.Error("the Matrix should stay open")
	}
}

func TestMatrix_FocusErrorKeepsTheMatrixOpen(t *testing.T) {
	m, _, _ := matrixWithAgents(t)
	m.focusAgent = func(string) error { return errors.New("pane gone") }
	_, cmd := m.Update(key("enter"))
	runCmd(m, cmd)
	if !strings.Contains(m.toastMsg, "pane gone") {
		t.Errorf("toast = %q, want the herdr error", m.toastMsg)
	}
	if m.topModal() == nil {
		t.Error("the Matrix should stay open after a failed jump")
	}
}

func TestMatrix_FlatlineHiddenByDefaultInAllMode(t *testing.T) {
	m := newTestModel(t)
	running := addCard(t, m, "ongoing")
	parked := addCard(t, m, "blocked")
	withAgent := addCard(t, m, "blocked")
	m.agents = map[int64]string{withAgent: "idle"}

	ids := func() map[int64]bool {
		got := map[int64]bool{}
		for _, r := range m.topModal().(*matrixModal).rows(m) {
			got[r.card.ID] = true
		}
		return got
	}

	send(m, "m")
	send(m, "a")
	got := ids()
	if !got[running] || !got[withAgent] {
		t.Errorf("all mode should list the ongoing card and the Blocked one with an agent: %v", got)
	}
	if got[parked] {
		t.Errorf("an agentless Blocked card must be hidden by default: %v", got)
	}
	if view := m.topModal().View(m); !strings.Contains(view, "f: show "+statusLabel["blocked"]) || !strings.Contains(view, "hidden") {
		t.Errorf("hint should offer f with the hidden count:\n%s", view)
	}

	send(m, "f")
	if !ids()[parked] {
		t.Error("f should reveal the agentless Blocked cards")
	}
	if view := m.topModal().View(m); !strings.Contains(view, "f: hide "+statusLabel["blocked"]) {
		t.Errorf("hint should offer hiding once shown:\n%s", view)
	}

	send(m, "f")
	if ids()[parked] {
		t.Error("f again should hide them")
	}
}

func TestMatrix_FlatlineKeyIsInertInAgentsOnlyMode(t *testing.T) {
	m := newTestModel(t)
	send(m, "m")
	send(m, "f")
	if m.topModal().(*matrixModal).showFlatline {
		t.Error("f must do nothing until the all-cards view is on")
	}
	if view := m.topModal().View(m); strings.Contains(view, "f: ") {
		t.Errorf("no f hint outside the all-cards view:\n%s", view)
	}
}
