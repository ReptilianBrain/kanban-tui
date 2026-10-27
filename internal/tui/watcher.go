package tui

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// flatlineAfter is how long a card's agent must sit waiting for input before
// the watcher moves the card to Blocked. A permission prompt answered within
// the window never touches the board.
const flatlineAfter = 90 * time.Second

// watcher moves cards between In Progress and Blocked from herdr agent state.
// It never touches any other column: Completed stays with the ship skill and
// In Review stays with the session or the user.
//
// A card is only moved back by the watcher if the watcher moved it there
// (autoBlocked), as soon as its agent stops waiting; a card the user blocked
// by hand stays blocked.
type watcher struct {
	waitingSince map[int64]time.Time // card id -> first poll seen waiting
	autoBlocked  map[int64]bool      // cards this watcher moved to Blocked
}

func newWatcher() *watcher {
	return &watcher{waitingSince: map[int64]time.Time{}, autoBlocked: map[int64]bool{}}
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

// reconcileAgents applies one poll of agent states. It returns a toast Cmd
// when a card moved. While a modal is open it does nothing, like the DB poll,
// so the board never shifts under an edit in progress; the waiting clock keeps
// running, so the move happens on the first poll after the modal closes.
func (m *Model) reconcileAgents(states map[int64]string) tea.Cmd {
	if m.blockingModal() {
		return nil
	}
	now := m.clock()
	w := m.watch

	for id := range w.waitingSince {
		if states[id] != "blocked" {
			delete(w.waitingSince, id)
		}
	}

	var cmds []tea.Cmd
	for id, state := range states {
		status, ok := m.cardStatus(id)
		if !ok {
			continue
		}
		switch {
		case state == "blocked" && status == "ongoing":
			since, seen := w.waitingSince[id]
			if !seen {
				w.waitingSince[id] = now
				continue
			}
			if now.Sub(since) < flatlineAfter {
				continue
			}
			if m.watcherMove(id, "blocked") {
				w.autoBlocked[id] = true
				delete(w.waitingSince, id)
				cmds = append(cmds, m.toast(toastWarn, fmt.Sprintf(
					"Wintermute: #%d to %s, its agent is waiting for input", id, statusLabel["blocked"])))
			}
		case state != "blocked" && status == "blocked" && w.autoBlocked[id]:
			// Any state but waiting ends the reason for the block. An answered
			// prompt can go waiting -> done inside one 3s poll, so "working"
			// is not required.
			if m.watcherMove(id, "ongoing") {
				delete(w.autoBlocked, id)
				cmds = append(cmds, m.toast(toastInfo, fmt.Sprintf(
					"Wintermute: #%d back to %s, its agent is no longer waiting", id, statusLabel["ongoing"])))
			}
		}
	}

	// Stop tracking cards that left Blocked (moved by hand) or whose agent is gone.
	for id := range w.autoBlocked {
		status, ok := m.cardStatus(id)
		if !ok || status != "blocked" || states[id] == "" {
			delete(w.autoBlocked, id)
		}
	}
	return tea.Batch(cmds...)
}

// watcherMove writes the status change and reloads the board, keeping the
// cursor on the same card. Reloading records the new revision, so the DB poll
// does not report the watcher's own write as an external change.
func (m *Model) watcherMove(id int64, status string) bool {
	if err := m.store.UpdateCardStatus(id, status); err != nil {
		return false
	}
	cur := m.currentCard()
	_ = m.loadData()
	if cur != nil {
		for i, c := range m.visibleCards(statuses[m.col]) {
			if c.ID == cur.ID {
				m.row = i
				break
			}
		}
	}
	m.clampRow()
	return true
}
