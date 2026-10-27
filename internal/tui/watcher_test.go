package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

var t0 = time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)

func addCard(t *testing.T, m *Model, status string) int64 {
	t.Helper()
	id, err := m.store.CreateCard("watched", "", status, "medium", nil, nil, nil)
	if err != nil {
		t.Fatalf("CreateCard: %v", err)
	}
	if err := m.loadData(); err != nil {
		t.Fatalf("loadData: %v", err)
	}
	return id
}

// reconcileAt runs one watcher poll with the clock pinned at t0+offset.
func reconcileAt(m *Model, offset time.Duration, states map[int64]string) {
	m.clock = func() time.Time { return t0.Add(offset) }
	m.reconcileAgents(states)
}

func statusOf(t *testing.T, m *Model, id int64) string {
	t.Helper()
	card, err := m.store.GetCard(id)
	if err != nil {
		t.Fatalf("GetCard: %v", err)
	}
	return card.Status
}

func TestWatcher_ToastsOnceAfterTheWaitingWindowAndNeverMoves(t *testing.T) {
	m := newTestModel(t)
	id := addCard(t, m, "ongoing")
	waiting := map[int64]string{id: "blocked"}

	reconcileAt(m, 0, waiting)
	reconcileAt(m, waitAfter-time.Second, waiting)
	if m.toastMsg != "" {
		t.Fatalf("toast inside the window: %q", m.toastMsg)
	}

	reconcileAt(m, waitAfter+time.Second, waiting)
	if !strings.Contains(m.toastMsg, fmt.Sprintf("#%d ", id)) || !strings.Contains(m.toastMsg, "waiting for input") {
		t.Fatalf("want a waiting toast after the window, got %q", m.toastMsg)
	}
	if got := statusOf(t, m, id); got != "ongoing" {
		t.Errorf("status = %q, the watcher must never move a card", got)
	}

	m.toastMsg = ""
	reconcileAt(m, waitAfter+10*time.Second, waiting)
	if m.toastMsg != "" {
		t.Errorf("repeat toast while still waiting: %q", m.toastMsg)
	}
}

func TestWatcher_NotifiesAgainAfterAnotherPrompt(t *testing.T) {
	m := newTestModel(t)
	id := addCard(t, m, "ongoing")
	waiting := map[int64]string{id: "blocked"}

	reconcileAt(m, 0, waiting)
	reconcileAt(m, waitAfter+time.Second, waiting)
	reconcileAt(m, waitAfter+4*time.Second, map[int64]string{id: "working"})

	m.toastMsg = ""
	reconcileAt(m, 200*time.Second, waiting)
	reconcileAt(m, 200*time.Second+waitAfter+time.Second, waiting)
	if m.toastMsg == "" {
		t.Error("a second prompt should toast again")
	}
}

func TestWatcher_AnsweredPromptRestartsTheClock(t *testing.T) {
	m := newTestModel(t)
	id := addCard(t, m, "ongoing")

	reconcileAt(m, 0, map[int64]string{id: "blocked"})
	reconcileAt(m, 30*time.Second, map[int64]string{id: "working"}) // prompt answered
	reconcileAt(m, 100*time.Second, map[int64]string{id: "blocked"})
	reconcileAt(m, 100*time.Second+waitAfter-time.Second, map[int64]string{id: "blocked"})
	if m.toastMsg != "" {
		t.Errorf("toast %q: the 90s must count from the second prompt", m.toastMsg)
	}
}

func TestWatcher_GroupsSimultaneousWaits(t *testing.T) {
	m := newTestModel(t)
	a := addCard(t, m, "ongoing")
	b := addCard(t, m, "ongoing")
	waiting := map[int64]string{a: "blocked", b: "blocked"}

	reconcileAt(m, 0, waiting)
	reconcileAt(m, waitAfter+time.Second, waiting)
	if want := fmt.Sprintf("#%d, #%d are waiting", a, b); !strings.Contains(m.toastMsg, want) {
		t.Errorf("want one grouped toast, got %q", m.toastMsg)
	}
}

func TestWatcher_OnlyWatchesOngoing(t *testing.T) {
	m := newTestModel(t)
	for _, status := range []string{"todo", "blocked", "in_review", "completed"} {
		id := addCard(t, m, status)
		waiting := map[int64]string{id: "blocked"}
		reconcileAt(m, 0, waiting)
		reconcileAt(m, time.Hour, waiting)
		if m.toastMsg != "" {
			t.Errorf("%s card toasted: %q", status, m.toastMsg)
		}
		if got := statusOf(t, m, id); got != status {
			t.Errorf("%s card moved to %q", status, got)
		}
	}
}

func TestWatcher_DropsTrackingWhenTheCardLeavesOngoing(t *testing.T) {
	m := newTestModel(t)
	id := addCard(t, m, "ongoing")
	waiting := map[int64]string{id: "blocked"}
	reconcileAt(m, 0, waiting)

	if err := m.store.UpdateCardStatus(id, "in_review"); err != nil { // user takes over
		t.Fatal(err)
	}
	_ = m.loadData()
	reconcileAt(m, time.Minute, waiting)
	if _, tracked := m.watch.waitingSince[id]; tracked {
		t.Error("still tracking a card that left In Progress")
	}
}

func TestWatcher_WaitsOutAnOpenModal(t *testing.T) {
	m := newTestModel(t)
	id := addCard(t, m, "ongoing")
	waiting := map[int64]string{id: "blocked"}
	reconcileAt(m, 0, waiting)

	m.modalStack = []modal{nil} // only the length is consulted
	reconcileAt(m, waitAfter+time.Second, waiting)
	if m.toastMsg != "" {
		t.Fatalf("toasted while a modal was open: %q", m.toastMsg)
	}

	m.modalStack = nil
	reconcileAt(m, waitAfter+2*time.Second, waiting)
	if m.toastMsg == "" {
		t.Error("should toast on the first poll after the modal closed")
	}
}

func TestWatcher_UnknownCardIsIgnored(t *testing.T) {
	m := newTestModel(t)
	reconcileAt(m, 0, map[int64]string{9999: "blocked"})
	reconcileAt(m, time.Hour, map[int64]string{9999: "blocked"})
}

func TestColumnHeaderCountsWaitingAgents(t *testing.T) {
	m := newTestModel(t)
	a := addCard(t, m, "ongoing")
	b := addCard(t, m, "ongoing")

	if got := m.renderColumn("ongoing", 40, 10); strings.Contains(got, "▲") {
		t.Errorf("no agent waiting, header = %q", got)
	}
	m.agents = map[int64]string{a: "blocked", b: "working"}
	if got := m.renderColumn("ongoing", 40, 10); !strings.Contains(got, "▲1") {
		t.Errorf("want ▲1 in the In Progress header, got %q", got)
	}
}
