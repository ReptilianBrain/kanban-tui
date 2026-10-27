package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

type deleteConfirmModal struct {
	cardTitle string
	done      func(m *Model, confirmed bool) tea.Cmd
}

func newDeleteConfirmModal(cardTitle string, done func(m *Model, confirmed bool) tea.Cmd) *deleteConfirmModal {
	return &deleteConfirmModal{cardTitle: cardTitle, done: done}
}

func (d *deleteConfirmModal) Init(m *Model) tea.Cmd { return nil }

func (d *deleteConfirmModal) Update(msg tea.Msg, m *Model) tea.Cmd {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil
	}
	switch keyMsg.String() {
	case "y":
		m.popModal()
		return d.done(m, true)
	case "n", "esc":
		m.popModal()
		return d.done(m, false)
	}
	return nil
}

func (d *deleteConfirmModal) View(m *Model) string {
	body := fmt.Sprintf(
		"%s\n\n%s\n\n%s\n\n%s",
		errorBoldStyle.Render("Delete "+cardTitle()+"?"),
		boldStyle.Render(d.cardTitle),
		modalHintStyle.Render("This action cannot be undone."),
		modalHintStyle.Render("No [n]     Yes [y]"),
	)
	return renderModal(modalDangerStyle, 52, body)
}
