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

// matrixModal is the live monitor: every card in flight (In Progress or
// Blocked, or with a herdr agent anywhere) with its agent state and, for a
// waiting agent, how long until the watcher blocks the card. It is read-only,
// so unlike other modals it does not pause the DB poll or the watcher; see
// blockingModal.
type matrixModal struct{}

func newMatrixModal() *matrixModal { return &matrixModal{} }

func (*matrixModal) Init(*Model) tea.Cmd { return nil }

func (*matrixModal) Update(msg tea.Msg, m *Model) tea.Cmd {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "esc", "q", "m":
			m.popModal()
		}
	}
	return nil
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

// matrixRows lists the cards in flight, most urgent first.
func (m *Model) matrixRows() []matrixRow {
	var rows []matrixRow
	for _, cards := range m.data {
		for _, c := range cards {
			agent := m.agents[c.ID]
			if agent == "" && c.Status != "ongoing" && c.Status != "blocked" {
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

// watchNote says what the watcher is doing about a row, if anything.
func (m *Model) watchNote(r matrixRow) string {
	if since, ok := m.watch.waitingSince[r.card.ID]; ok && r.card.Status == "ongoing" {
		left := flatlineAfter - m.clock().Sub(since)
		if left <= 0 {
			return warnStyle.Render("Flatline next poll")
		}
		return warnStyle.Render(fmt.Sprintf("Flatline in %s (waited %s)", formatWait(left), formatWait(m.clock().Sub(since))))
	}
	if m.watch.autoBlocked[r.card.ID] {
		return dimStyle.Render("auto-blocked, returns when the agent resumes")
	}
	return ""
}

func (*matrixModal) View(m *Model) string {
	rows := m.matrixRows()
	waiting := 0
	for _, r := range rows {
		if r.agent == "blocked" {
			waiting++
		}
	}

	var b strings.Builder
	b.WriteString(accentBold.Render("Matrix"))
	b.WriteString("  " + hintStyle.Render(fmt.Sprintf("%d in flight, %d waiting on you", len(rows), waiting)))
	b.WriteString("\n\n")
	if len(rows) == 0 {
		b.WriteString(dimStyle.Render("Nothing in flight."))
		b.WriteString("\n")
	}
	for _, r := range rows {
		agent := agentMarkup(r.agent)
		if agent == "" {
			agent = dimStyle.Render("no agent")
		}
		cycle := dimStyle.Render("-")
		if c := m.cycleText(r.card.ID); c != "" {
			cycle = textStyle.Render(c)
		}
		line := padCell(dimStyle.Render(fmt.Sprintf("#%d", r.card.ID)), 6) +
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
	b.WriteString(hintStyle.Render("Cycle is the card's total spend across sessions   Updates live   Esc/m: close"))
	return renderModal(modalBoxStyle, 112, b.String())
}
