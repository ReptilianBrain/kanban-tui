package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/ReptilianBrain/kanban-tui/internal/db"
)

// filterModal picks a label to filter the board by, as a left/right cycler
// over "(all cards)" plus every defined label. Applying "(all cards)" clears
// the filter.
type filterModal struct {
	labels []db.Label
	idx    int // 0 = "(all cards)"
	done   func(m *Model, result *string) tea.Cmd
}

func newFilterModal(store *db.DB, current string, done func(m *Model, result *string) tea.Cmd) *filterModal {
	labels, _ := store.GetLabels()
	idx := 0
	for i, l := range labels {
		if l.Name == current {
			idx = i + 1
		}
	}
	return &filterModal{labels: labels, idx: idx, done: done}
}

func (f *filterModal) Init(m *Model) tea.Cmd { return nil }

func (f *filterModal) current() string {
	if f.idx == 0 || f.idx-1 >= len(f.labels) {
		return ""
	}
	return f.labels[f.idx-1].Name
}

func (f *filterModal) Update(msg tea.Msg, m *Model) tea.Cmd {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil
	}
	switch keyMsg.String() {
	case "esc":
		m.popModal()
		return f.done(m, nil)
	case "enter":
		value := f.current()
		m.popModal()
		return f.done(m, &value)
	case "left", "h":
		if len(f.labels) > 0 {
			n := len(f.labels) + 1
			f.idx = (f.idx - 1 + n) % n
		}
	case "right", "l":
		if len(f.labels) > 0 {
			n := len(f.labels) + 1
			f.idx = (f.idx + 1) % n
		}
	}
	return nil
}

func (f *filterModal) View(m *Model) string {
	body := accentBold.Render("Filter by label") + "\n\n"
	if len(f.labels) == 0 {
		body += modalHintStyle.Render("No labels defined — press 't' to manage labels.") + "\n\n"
	} else {
		name := "(all cards)"
		if f.idx > 0 {
			name = f.labels[f.idx-1].Name
		}
		body += pickerValue(name, true) + "\n\n"
	}
	body += modalHintStyle.Render("←/→: change   Enter: apply   Esc: cancel")
	return renderModal(modalWarnBoxStyle, 52, body)
}
