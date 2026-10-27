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

const (
	detailWidth    = 88
	detailMinVPH   = 4
	modalChromeW   = 4 // horizontal padding inside the box
	modalChromeH   = 4 // border (2) + vertical padding (2)
	detailBoardGap = 3 // rows left for the title bar and footer, as in board mode
)

var priorityDisplay = map[string]string{
	"low": "Low", "medium": "Medium", "high": "High", "critical": "Critical",
}

func newCardDetailModal(card db.Card) *cardDetailModal {
	d := &cardDetailModal{card: card, vp: viewport.New(detailWidth-modalChromeW, 8)}
	d.setContent()
	return d
}

func (d *cardDetailModal) setContent() {
	desc := d.card.Description
	if desc == "" {
		desc = textStyle.Render("(no description)")
	} else {
		desc = lipgloss.NewStyle().Width(d.vp.Width).Render(desc)
	}
	d.vp.SetContent(desc)
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
	width := detailWidth
	if m.width > 0 && width > m.width-2 {
		width = m.width - 2
	}
	inner := width - modalChromeW

	var head strings.Builder
	b := &head
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

	var foot strings.Builder
	b = &foot
	b.WriteString("\n")
	due := dueMarkup(c.DueDate)
	jira := ""
	if c.JiraKey != nil && *c.JiraKey != "" {
		jira = accentStyle.Render(*c.JiraKey)
	}
	lbl := labelMarkup(c.Label)
	meta := []string{}
	if jira != "" {
		meta = append(meta, jira)
	}
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

	footer := foot.String()

	// Fill the terminal height like the board does; the description viewport
	// takes whatever the header and footer leave over.
	// Both strings carry a joining newline, which Height counts as a row.
	headH := lipgloss.Height(lipgloss.NewStyle().Width(inner).Render(head.String())) - 1
	footH := lipgloss.Height(lipgloss.NewStyle().Width(inner).Render(footer)) - 1
	vpH := m.height - detailBoardGap - modalChromeH - headH - footH
	if vpH < detailMinVPH {
		vpH = detailMinVPH
	}
	if d.vp.Width != inner || d.vp.Height != vpH {
		d.vp.Width, d.vp.Height = inner, vpH
		d.setContent()
	}

	return renderModal(modalBoxStyle, width, head.String()+d.vp.View()+footer)
}
