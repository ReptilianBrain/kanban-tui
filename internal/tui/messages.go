package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// tickMsg drives the ~1s DB-change poll. Bubble Tea's Tick is one-shot, so
// every handler that receives a tickMsg must schedule the next one itself.
type tickMsg struct{}

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return tickMsg{} })
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
