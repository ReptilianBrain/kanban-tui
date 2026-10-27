package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	if rows := m.matrixRows(false); len(rows) < 2 || rows[0].card.ID != waiting {
		t.Errorf("the card waiting on you should sort first, rows = %+v", rows)
	}

	send(m, "esc")
	if m.topModal() != nil {
		t.Error("esc should close the Matrix")
	}
}

func TestMatrix_ShowsHowLongTheAgentHasWaited(t *testing.T) {
	m := newTestModel(t)
	id := addCard(t, m, "ongoing")
	m.agents = map[int64]string{id: "blocked"}
	reconcileAt(m, 0, m.agents)
	m.clock = func() time.Time { return t0.Add(30 * time.Second) }

	send(m, "m")
	view := m.topModal().View(m)
	if !strings.Contains(view, "waiting 0:30") {
		t.Errorf("want 0:30 after 30s of waiting, got:\n%s", view)
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
