package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ReptilianBrain/kanban-tui/internal/db"
)

// cardDetailModal is the read-only full-card view. The description sits in a
// scrollable viewport because it can exceed the modal's max height.
type cardDetailModal struct {
	card db.Card
	vp   viewport.Model
}

var priorityDisplay = map[string]string{
	"low": "Low", "medium": "Medium", "high": "High", "critical": "Critical",
}

func newCardDetailModal(card db.Card) *cardDetailModal {
	vp := viewport.New(60, 8)
	desc := card.Description
	if desc == "" {
		desc = textStyle.Render("(no description)")
	} else {
		desc = lipgloss.NewStyle().Width(vp.Width).Render(desc)
	}
	vp.SetContent(desc)
	return &cardDetailModal{card: card, vp: vp}
}

func (d *cardDetailModal) Init(m *Model) tea.Cmd { return nil }

func (d *cardDetailModal) Update(msg tea.Msg, m *Model) tea.Cmd {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil
	}
	switch keyMsg.String() {
	case "esc", "q":
		m.popModal()
		return nil
	case "up", "k":
		d.vp.ScrollUp(1)
	case "down", "j":
		d.vp.ScrollDown(1)
	case "pgup":
		d.vp.PageUp()
	case "pgdown":
		d.vp.PageDown()
	case "home":
		d.vp.GotoTop()
	case "end":
		d.vp.GotoBottom()
	case "H":
		return m.actionOpenHerdr()
	}
	return nil
}

func (d *cardDetailModal) View(m *Model) string {
	c := d.card
	var b strings.Builder
	b.WriteString(accentBold.Render(c.Title))
	b.WriteString("\n")
	b.WriteString(modalHintStyle.Render(fmt.Sprintf("[ %s ]  #%d", statusLabel[c.Status], c.ID)))
	b.WriteString("\n")
	prio := priorityDisplay[c.Priority]
	if prio == "" {
		prio = c.Priority
	}
	b.WriteString(textStyle.Render("Priority: " + prio))
	if cycle := m.cycleText(c.ID); cycle != "" {
		b.WriteString("   " + textStyle.Render("Cycle: "+cycle))
	}
	b.WriteString("\n\n")
	b.WriteString(boldStyle.Render("Description"))
	b.WriteString("\n")
	b.WriteString(d.vp.View())
	b.WriteString("\n")

	due := dueMarkup(c.DueDate)
	lbl := labelMarkup(c.Label)
	meta := []string{}
	if due != "" {
		meta = append(meta, due)
	}
	if lbl != "" {
		meta = append(meta, lbl)
	}
	if len(meta) > 0 {
		b.WriteString(strings.Join(meta, "  "))
		b.WriteString("\n")
	}

	created := c.CreatedAt
	if len(created) >= 10 {
		created = created[:10]
	}
	updated := c.UpdatedAt
	if len(updated) >= 10 {
		updated = updated[:10]
	}
	b.WriteString(modalHintStyle.Render(fmt.Sprintf("Created: %s   Updated: %s", created, updated)))
	b.WriteString("\n\n")
	b.WriteString(modalHintStyle.Render("↑/↓/PgUp/PgDn/Home/End: scroll description   H: open in herdr   Esc: close"))

	return renderModal(modalBoxStyle, 72, b.String())
}
