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

func TestMatrix_ShowsTheFlatlineCountdown(t *testing.T) {
	m := newTestModel(t)
	id := addCard(t, m, "ongoing")
	m.agents = map[int64]string{id: "blocked"}
	reconcileAt(m, 0, m.agents)
	m.clock = func() time.Time { return t0.Add(30 * time.Second) }

	send(m, "m")
	view := m.topModal().View(m)
	if !strings.Contains(view, "Flatline in 1:00") {
		t.Errorf("want a 1:00 countdown after 30s of waiting, got:\n%s", view)
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
	reconcileAt(m, flatlineAfter+time.Second, waiting)
	if got := statusOf(t, m, id); got != "blocked" {
		t.Errorf("watcher should run under the Matrix, status = %q", got)
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
