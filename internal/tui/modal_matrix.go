package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ReptilianBrain/kanban-tui/internal/db"
)

// Text on the modal surface (nord1) uses Snow Storm (textStyle) and Frost
// (cyanStyle), never dimStyle/hintStyle: nord3 on nord1 is about 1.3:1 and
// unreadable in a real terminal.
//
// matrixModal is the live monitor: the cards that have a herdr agent, with
// the agent's state and, for a waiting agent, how long it has waited. "a"
// widens it to every card in flight (In Progress or Blocked), since a board
// accumulates stale cards that have no agent. Enter jumps to the selected
// card's herdr pane. It never edits a card, so unlike other modals it does not
// pause the DB poll or the watcher; see blockingModal.
//
// The selection follows the card id, not the row index: rows reorder as agent
// states change, and the cursor must not slide onto another card.
type matrixModal struct {
	showAll bool
	cursor  int
	cardID  int64
}

func newMatrixModal() *matrixModal { return &matrixModal{} }

func (*matrixModal) Init(*Model) tea.Cmd { return nil }

func (mm *matrixModal) Update(msg tea.Msg, m *Model) tea.Cmd {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil
	}
	switch k.String() {
	case "esc", "q", "m":
		m.popModal()
	case "a":
		mm.showAll = !mm.showAll
	case "up", "k":
		mm.move(m, -1)
	case "down", "j":
		mm.move(m, 1)
	case "enter":
		return mm.jump(m)
	}
	return nil
}

// sync resolves the selection against the current rows: the selected card
// keeps the cursor wherever it moved, and when it left the list the cursor
// stays at its index, clamped. It returns the cursor row, or -1 when empty.
func (mm *matrixModal) sync(rows []matrixRow) int {
	if len(rows) == 0 {
		return -1
	}
	for i, r := range rows {
		if mm.cardID != 0 && r.card.ID == mm.cardID {
			mm.cursor = i
			return i
		}
	}
	if mm.cursor >= len(rows) {
		mm.cursor = len(rows) - 1
	}
	if mm.cursor < 0 {
		mm.cursor = 0
	}
	mm.cardID = rows[mm.cursor].card.ID
	return mm.cursor
}

func (mm *matrixModal) move(m *Model, delta int) {
	rows := m.matrixRows(mm.showAll)
	i := mm.sync(rows)
	if i < 0 {
		return
	}
	i += delta
	if i < 0 || i >= len(rows) {
		return
	}
	mm.cursor, mm.cardID = i, rows[i].card.ID
}

// jump focuses the selected card's agent pane in herdr. The result comes back
// as a matrixFocusDoneMsg, which closes the Matrix (and a popup).
func (mm *matrixModal) jump(m *Model) tea.Cmd {
	rows := m.matrixRows(mm.showAll)
	i := mm.sync(rows)
	if i < 0 {
		return nil
	}
	pane := m.agentPanes[rows[i].card.ID]
	if pane == "" {
		return m.toast(toastWarn, fmt.Sprintf("No agent for this %s", cardNoun()))
	}
	focus := m.focusAgent
	return func() tea.Msg { return matrixFocusDoneMsg{err: focus(pane)} }
}

// blockingModal reports whether an open modal should freeze the live
// refresh and the watcher: any modal except the read-only Matrix.
func (m *Model) blockingModal() bool {
	if len(m.modalStack) == 0 {
		return false
	}
	_, isMatrix := m.modalStack[len(m.modalStack)-1].(*matrixModal)
	return !isMatrix
}

func (m *Model) actionMatrix() tea.Cmd {
	return m.pushModal(newMatrixModal())
}

type matrixRow struct {
	card  db.Card
	agent string
}

// agentRank orders rows: an agent waiting on you first, no agent last.
var agentRank = map[string]int{"blocked": 0, "working": 1, "idle": 2, "done": 3, "": 4}

// matrixRows lists the cards with an agent, most urgent first; all adds the
// agentless cards in In Progress or Blocked.
func (m *Model) matrixRows(all bool) []matrixRow {
	var rows []matrixRow
	for _, cards := range m.data {
		for _, c := range cards {
			agent := m.agents[c.ID]
			inFlight := c.Status == "ongoing" || c.Status == "blocked"
			if agent == "" && !(all && inFlight) {
				continue
			}
			rows = append(rows, matrixRow{card: c, agent: agent})
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		ri, rj := agentRank[rows[i].agent], agentRank[rows[j].agent]
		if ri != rj {
			return ri < rj
		}
		return rows[i].card.ID < rows[j].card.ID
	})
	return rows
}

func padCell(s string, w int) string {
	if n := lipgloss.Width(s); n < w {
		return s + strings.Repeat(" ", w-n)
	}
	return s
}

func clipRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return strings.TrimRight(string(r[:n-1]), " ") + "…"
}

// formatWait renders a duration as m:ss.
func formatWait(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	s := int(d.Seconds())
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

// watchNote says how long a row's agent has been waiting on you, if it is.
func (m *Model) watchNote(r matrixRow) string {
	if since, ok := m.watch.waitingSince[r.card.ID]; ok && r.card.Status == "ongoing" {
		return warnStyle.Render("waiting " + formatWait(m.clock().Sub(since)))
	}
	return ""
}

func (mm *matrixModal) View(m *Model) string {
	rows := m.matrixRows(mm.showAll)
	idle := len(m.matrixRows(true)) - len(m.matrixRows(false))
	waiting := 0
	for _, r := range rows {
		if r.agent == "blocked" {
			waiting++
		}
	}

	var b strings.Builder
	b.WriteString(accentBold.Render("Matrix"))
	b.WriteString("  " + cyanStyle.Render(fmt.Sprintf("%d shown, %d waiting on you", len(rows), waiting)))
	b.WriteString("\n\n")
	switch {
	case len(rows) == 0 && idle == 0:
		b.WriteString(textStyle.Render("Nothing in flight."))
		b.WriteString("\n")
	case len(rows) == 0:
		b.WriteString(textStyle.Render(fmt.Sprintf("No agents running. a: show the %d %s in flight without one.", idle, cardNouns())))
		b.WriteString("\n")
	}
	sel := mm.sync(rows)
	for i, r := range rows {
		agent := agentMarkup(r.agent)
		if agent == "" {
			agent = textStyle.Render("no agent")
		}
		cycle := textStyle.Render("-")
		if c := m.cycleText(r.card.ID); c != "" {
			cycle = textStyle.Render(c)
		}
		bar := "  "
		if i == sel {
			bar = selectedBarStyle.Render("▌") + " "
		}
		line := bar + padCell(cyanStyle.Render(fmt.Sprintf("#%d", r.card.ID)), 6) +
			padCell(boldStyle.Render(clipRunes(r.card.Title, 30)), 32) +
			padCell(textStyle.Render(statusLabel[r.card.Status]), 12) +
			padCell(agent, 12) +
			padCell(cycle, 9)
		if note := m.watchNote(r); note != "" {
			line += note
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(cyanStyle.Render("↑↓: select   Enter: jump to agent   a: agents only / all " + cardNouns() + " in flight   Esc/m: close"))
	b.WriteString("\n")
	b.WriteString(cyanStyle.Render("Cycle: total spend across sessions"))
	return renderModal(modalBoxStyle, 112, b.String())
}
