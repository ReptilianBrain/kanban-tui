package tui

import (
	"testing"
	"time"
)

var t0 = time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)

func addCard(t *testing.T, m *Model, status string) int64 {
	t.Helper()
	id, err := m.store.CreateCard("watched", "", status, "medium", nil, nil)
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

func TestWatcher_FlatlinesAfterWaitingWindowAndReturns(t *testing.T) {
	m := newTestModel(t)
	id := addCard(t, m, "ongoing")
	waiting := map[int64]string{id: "blocked"}

	reconcileAt(m, 0, waiting)
	reconcileAt(m, flatlineAfter-time.Second, waiting)
	if got := statusOf(t, m, id); got != "ongoing" {
		t.Fatalf("inside the window status = %q, want ongoing", got)
	}

	reconcileAt(m, flatlineAfter+time.Second, waiting)
	if got := statusOf(t, m, id); got != "blocked" {
		t.Fatalf("after the window status = %q, want blocked", got)
	}

	reconcileAt(m, flatlineAfter+10*time.Second, map[int64]string{id: "working"})
	if got := statusOf(t, m, id); got != "ongoing" {
		t.Errorf("after the agent resumed status = %q, want ongoing", got)
	}
}

func TestWatcher_AnsweredPromptRestartsTheClock(t *testing.T) {
	m := newTestModel(t)
	id := addCard(t, m, "ongoing")

	reconcileAt(m, 0, map[int64]string{id: "blocked"})
	reconcileAt(m, 30*time.Second, map[int64]string{id: "working"}) // prompt answered
	reconcileAt(m, 100*time.Second, map[int64]string{id: "blocked"})
	reconcileAt(m, 100*time.Second+flatlineAfter-time.Second, map[int64]string{id: "blocked"})
	if got := statusOf(t, m, id); got != "ongoing" {
		t.Errorf("status = %q, want ongoing: the 90s must count from the second prompt", got)
	}
}

func TestWatcher_NeverReturnsAHandBlockedCard(t *testing.T) {
	m := newTestModel(t)
	id := addCard(t, m, "blocked") // blocked by the user, not the watcher

	reconcileAt(m, 0, map[int64]string{id: "working"})
	reconcileAt(m, time.Hour, map[int64]string{id: "working"})
	if got := statusOf(t, m, id); got != "blocked" {
		t.Errorf("status = %q, want blocked kept", got)
	}
}

func TestWatcher_DropsTrackingWhenTheUserMovesTheCard(t *testing.T) {
	m := newTestModel(t)
	id := addCard(t, m, "ongoing")
	waiting := map[int64]string{id: "blocked"}
	reconcileAt(m, 0, waiting)
	reconcileAt(m, flatlineAfter+time.Second, waiting) // watcher blocks it

	if err := m.store.UpdateCardStatus(id, "in_review"); err != nil { // user takes over
		t.Fatal(err)
	}
	_ = m.loadData()
	reconcileAt(m, 3*time.Minute, map[int64]string{id: "working"})
	if m.watch.autoBlocked[id] {
		t.Error("autoBlocked still tracks a card the user moved away")
	}
	if got := statusOf(t, m, id); got != "in_review" {
		t.Errorf("status = %q, want in_review untouched", got)
	}
}

func TestWatcher_OnlyTouchesOngoingAndBlocked(t *testing.T) {
	m := newTestModel(t)
	for _, status := range []string{"todo", "in_review", "completed"} {
		id := addCard(t, m, status)
		reconcileAt(m, 0, map[int64]string{id: "blocked"})
		reconcileAt(m, time.Hour, map[int64]string{id: "blocked"})
		reconcileAt(m, 2*time.Hour, map[int64]string{id: "working"})
		if got := statusOf(t, m, id); got != status {
			t.Errorf("%s card moved to %q", status, got)
		}
	}
}

func TestWatcher_WaitsOutAnOpenModal(t *testing.T) {
	m := newTestModel(t)
	id := addCard(t, m, "ongoing")
	waiting := map[int64]string{id: "blocked"}
	reconcileAt(m, 0, waiting)

	m.modalStack = []modal{nil} // only the length is consulted
	reconcileAt(m, flatlineAfter+time.Second, waiting)
	if got := statusOf(t, m, id); got != "ongoing" {
		t.Fatalf("moved while a modal was open: %q", got)
	}

	m.modalStack = nil
	reconcileAt(m, flatlineAfter+2*time.Second, waiting)
	if got := statusOf(t, m, id); got != "blocked" {
		t.Errorf("after the modal closed status = %q, want blocked", got)
	}
}

func TestWatcher_UnknownCardIsIgnored(t *testing.T) {
	m := newTestModel(t)
	reconcileAt(m, 0, map[int64]string{9999: "blocked"})
	reconcileAt(m, time.Hour, map[int64]string{9999: "blocked"})
}
