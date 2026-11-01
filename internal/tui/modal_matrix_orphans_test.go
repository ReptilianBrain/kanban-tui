package tui

import (
	"errors"
	"strings"
	"testing"

	"github.com/ReptilianBrain/kanban-tui/internal/herdrsync"
)

func withOrphans(t *testing.T) (m *Model, card int64) {
	t.Helper()
	m = newTestModel(t)
	card = addCard(t, m, "ongoing")
	m.agents = map[int64]string{card: "working"}
	m.agentPanes = map[int64]string{card: "w1:p1"}
	m.orphans = []herdrsync.AgentInfo{
		{PaneID: "w2:p9", CWD: "/work/scratch", Status: "idle", Title: "scratch session"},
		{PaneID: "w2:p3", CWD: "/work/other", Status: "blocked"},
	}
	send(m, "m")
	return m, card
}

func TestMatrixOrphans_HiddenByDefault(t *testing.T) {
	m, card := withOrphans(t)
	rows := m.topModal().(*matrixModal).rows(m)
	if len(rows) != 1 || rows[0].card.ID != card {
		t.Fatalf("rows = %v, want only card %d", rows, card)
	}
	if !strings.Contains(m.topModal().(*matrixModal).View(m), "o: show 2 agents") {
		t.Error("the hint should advertise the hidden orphans")
	}
}

func TestMatrixOrphans_ToggleListsThemAfterCardsMostUrgentFirst(t *testing.T) {
	m, card := withOrphans(t)
	send(m, "o")
	mm := m.topModal().(*matrixModal)
	rows := mm.rows(m)
	if len(rows) != 3 || rows[0].card.ID != card {
		t.Fatalf("rows = %v, want the card then two orphans", rows)
	}
	if rows[1].pane != "w2:p3" || rows[2].pane != "w2:p9" {
		t.Errorf("orphans out of order: %q, %q", rows[1].pane, rows[2].pane)
	}
	if rows[1].card.Title != "other" || rows[2].card.Title != "scratch session" {
		t.Errorf("labels = %q, %q; want cwd basename then terminal title", rows[1].card.Title, rows[2].card.Title)
	}
	if rows[1].card.ID >= 0 || rows[1].card.ID == rows[2].card.ID {
		t.Errorf("orphan ids must be distinct and negative: %d, %d", rows[1].card.ID, rows[2].card.ID)
	}
	send(m, "o")
	if got := len(mm.rows(m)); got != 1 {
		t.Errorf("second o should hide them again, got %d rows", got)
	}
}

func TestMatrixOrphans_EnterFocusesThePane(t *testing.T) {
	m, _ := withOrphans(t)
	var got string
	m.focusAgent = func(target string) error { got = target; return nil }
	send(m, "o")
	send(m, "down") // the first orphan, the blocked one
	_, cmd := m.Update(key("enter"))
	runCmd(m, cmd)
	if got != "w2:p3" {
		t.Errorf("focused %q, want w2:p3", got)
	}
}

func TestMatrixOrphans_SelectionSurvivesReorder(t *testing.T) {
	m, _ := withOrphans(t)
	send(m, "o")
	send(m, "down")
	send(m, "down")
	mm := m.topModal().(*matrixModal)
	mm.View(m)
	sel := mm.cardID
	m.orphans = []herdrsync.AgentInfo{
		{PaneID: "w2:p3", CWD: "/work/other", Status: "blocked"},
		{PaneID: "w2:p9", CWD: "/work/scratch", Status: "blocked", Title: "scratch session"},
	}
	mm.View(m)
	if mm.cardID != sel {
		t.Errorf("selection moved from %d to %d when the orphans reordered", sel, mm.cardID)
	}
}

func TestMatrixOrphans_NoCostForAnOrphan(t *testing.T) {
	m, _ := withOrphans(t)
	send(m, "o")
	if v := m.topModal().(*matrixModal).View(m); !strings.Contains(v, "no "+cardNoun()) {
		t.Errorf("orphan rows should read %q in the status column", "no "+cardNoun())
	}
}

// The live path: a successful poll must hand the orphans to the Model, and a
// failed or disabled one must not leave stale rows behind.
func TestMatrixOrphans_PollFeedsTheModel(t *testing.T) {
	m := newTestModel(t)
	orphan := herdrsync.AgentInfo{PaneID: "w2:p9", CWD: "/work/scratch", Status: "idle"}
	m.Update(agentsMsg{orphans: []herdrsync.AgentInfo{orphan}})
	if len(m.orphans) != 1 || m.orphans[0].PaneID != "w2:p9" {
		t.Fatalf("orphans after a poll = %v, want the polled agent", m.orphans)
	}
	m.Update(agentsMsg{err: errors.New("herdr hiccup")})
	if len(m.orphans) != 1 {
		t.Errorf("a failed poll should keep the previous orphans, got %v", m.orphans)
	}
	m.Update(agentsMsg{disabled: true})
	if len(m.orphans) != 0 {
		t.Errorf("a disabled herdr should clear the orphans, got %v", m.orphans)
	}
}
