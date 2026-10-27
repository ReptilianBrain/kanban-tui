package tui

import (
	"context"
	"errors"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ReptilianBrain/kanban-tui/internal/herdrsync"
)

// tickMsg drives the ~1s DB-change poll. Bubble Tea's Tick is one-shot, so
// every handler that receives a tickMsg must schedule the next one itself.
type tickMsg struct{}

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return tickMsg{} })
}

// agentPollEvery is how often herdr is asked for agent states. It is a
// subprocess per poll, so it is slower than the 1s DB poll.
const agentPollEvery = 3 * time.Second

// agentsMsg carries one poll of `herdr agent list`, reduced to a status per
// card. disabled means herdr is not installed, so polling stops for good; any
// other error keeps the previous states and tries again next round.
type agentsMsg struct {
	states   map[int64]string
	panes    map[int64]string
	err      error
	disabled bool
}

// pollAgentsCmd waits, then asks herdr for the agent list. Like tickCmd it is
// one-shot: the agentsMsg handler schedules the next poll.
func pollAgentsCmd(after time.Duration) tea.Cmd {
	return func() tea.Msg {
		time.Sleep(after)
		agents, err := herdrsync.ListAgents(context.Background())
		var cfg *herdrsync.ConfigError
		if errors.As(err, &cfg) {
			return agentsMsg{disabled: true}
		}
		if err != nil {
			return agentsMsg{err: err}
		}
		return agentsMsg{states: herdrsync.CardStates(agents), panes: herdrsync.CardPanes(agents)}
	}
}

// toastClearMsg clears the toast banner, but only if seq still matches the
// toast currently on screen — otherwise a later toast's own timer would be
// cancelled early by an earlier one's.
type toastClearMsg struct{ seq int }

func toastClearCmd(seq int) tea.Cmd {
	return tea.Tick(2500*time.Millisecond, func(time.Time) tea.Msg {
		return toastClearMsg{seq: seq}
	})
}

// matrixFocusDoneMsg is the result of jumping to a card's agent pane from the
// Matrix.
type matrixFocusDoneMsg struct{ err error }
