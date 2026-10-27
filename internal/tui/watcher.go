package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// waitAfter is how long a card's agent must sit waiting for input before the
// watcher toasts about it. A permission prompt answered within the window
// stays quiet.
const waitAfter = 90 * time.Second

// watcher tells the user when an In Progress card's agent has been waiting for
// input for a while. It never changes a card's status: Blocked is the user's
// own call, and the card already shows the waiting badge.
type watcher struct {
	waitingSince map[int64]time.Time // card id -> first poll seen waiting
	notified     map[int64]bool      // cards already toasted for the current wait
}

func newWatcher() *watcher {
	return &watcher{waitingSince: map[int64]time.Time{}, notified: map[int64]bool{}}
}

// cardStatus finds a card's current column in the loaded board.
func (m *Model) cardStatus(id int64) (string, bool) {
	for status, cards := range m.data {
		for _, c := range cards {
			if c.ID == id {
				return status, true
			}
		}
	}
	return "", false
}

// reconcileAgents applies one poll of agent states and returns a toast Cmd
// when a card's agent has waited past waitAfter. While a modal is open it does
// nothing, like the DB poll, so a toast never lands on an edit in progress;
// the waiting clock keeps running, so the toast fires on the first poll after
// the modal closes.
func (m *Model) reconcileAgents(states map[int64]string) tea.Cmd {
	if m.blockingModal() {
		return nil
	}
	now := m.clock()
	w := m.watch

	for id := range w.waitingSince {
		if status, _ := m.cardStatus(id); states[id] != "blocked" || status != "ongoing" {
			delete(w.waitingSince, id)
			delete(w.notified, id)
		}
	}

	var due []int64
	for id, state := range states {
		if state != "blocked" {
			continue
		}
		if status, ok := m.cardStatus(id); !ok || status != "ongoing" {
			continue
		}
		since, seen := w.waitingSince[id]
		if !seen {
			w.waitingSince[id] = now
			continue
		}
		if w.notified[id] || now.Sub(since) < waitAfter {
			continue
		}
		w.notified[id] = true
		due = append(due, id)
	}
	if len(due) == 0 {
		return nil
	}

	sort.Slice(due, func(i, j int) bool { return due[i] < due[j] })
	refs := make([]string, len(due))
	for i, id := range due {
		refs[i] = fmt.Sprintf("#%d", id)
	}
	verb := "is"
	if len(due) > 1 {
		verb = "are"
	}
	return m.toast(toastWarn, fmt.Sprintf("Wintermute: %s %s waiting for input", strings.Join(refs, ", "), verb))
}
