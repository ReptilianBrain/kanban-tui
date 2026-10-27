package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/ReptilianBrain/kanban-tui/internal/db"
)

// labelManagerModal lists labels and adds, renames and deletes them.
type labelManagerModal struct {
	labels []db.Label
	row    int
	done   func(m *Model) tea.Cmd

	// child is the add/rename input, when open. It is not on the root modal
	// stack; this modal routes input to it directly.
	child *labelNameModal
}

func newLabelManagerModal(store *db.DB, done func(m *Model) tea.Cmd) *labelManagerModal {
	lm := &labelManagerModal{done: done}
	lm.reload(store)
	return lm
}

func (lm *labelManagerModal) reload(store *db.DB) {
	labels, _ := store.GetLabels()
	lm.labels = labels
}

func (lm *labelManagerModal) clamp() {
	if len(lm.labels) == 0 {
		lm.row = 0
		return
	}
	if lm.row >= len(lm.labels) {
		lm.row = len(lm.labels) - 1
	}
}

func (lm *labelManagerModal) Init(m *Model) tea.Cmd { return nil }

func (lm *labelManagerModal) Update(msg tea.Msg, m *Model) tea.Cmd {
	if lm.child != nil {
		return lm.child.Update(msg, m)
	}
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil
	}
	switch keyMsg.String() {
	case "a":
		lm.child = newLabelNameModal("", func(m *Model, name *string) tea.Cmd {
			lm.child = nil
			if name == nil || *name == "" {
				return nil
			}
			if _, err := m.store.CreateLabel(*name); err != nil {
				return m.toast(toastError, fmt.Sprintf("Label %q already exists", *name))
			}
			lm.reload(m.store)
			lm.row = len(lm.labels) - 1
			return m.toast(toastInfo, fmt.Sprintf("Label %q created", *name))
		})
		return lm.child.Init(m)
	case "e":
		if len(lm.labels) == 0 {
			return nil
		}
		lbl := lm.labels[lm.row]
		lm.child = newLabelNameModal(lbl.Name, func(m *Model, name *string) tea.Cmd {
			lm.child = nil
			if name == nil || *name == "" || *name == lbl.Name {
				return nil
			}
			if err := m.store.UpdateLabel(lbl.ID, *name); err != nil {
				return m.toast(toastError, fmt.Sprintf("Label %q already exists", *name))
			}
			lm.reload(m.store)
			return m.toast(toastInfo, fmt.Sprintf("Renamed to %q", *name))
		})
		return lm.child.Init(m)
	case "d":
		if len(lm.labels) == 0 {
			return nil
		}
		lbl := lm.labels[lm.row]
		_ = m.store.DeleteLabel(lbl.ID)
		lm.reload(m.store)
		lm.clamp()
		return m.toast(toastWarn, fmt.Sprintf("Deleted label %q", lbl.Name))
	case "up", "k":
		if lm.row > 0 {
			lm.row--
		}
	case "down", "j":
		if lm.row < len(lm.labels)-1 {
			lm.row++
		}
	case "esc", "q":
		m.popModal()
		return lm.done(m)
	}
	return nil
}

func (lm *labelManagerModal) View(m *Model) string {
	if lm.child != nil {
		return lm.child.View(m)
	}
	var b strings.Builder
	b.WriteString(accentBold.Render("Manage Labels"))
	b.WriteString("\n\n")
	if len(lm.labels) == 0 {
		b.WriteString(textStyle.Render("No labels defined"))
	} else {
		for i, l := range lm.labels {
			bar := "  "
			if i == lm.row {
				bar = selectedBarStyle.Render("▌") + " "
			}
			b.WriteString(bar + textStyle.Render(l.Name))
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")
	b.WriteString(modalHintStyle.Render("a:Add  e:Edit  d:Delete  Esc:Back"))
	return renderModal(modalBoxStyle, 50, b.String())
}

// labelNameModal is the add/rename input nested in labelManagerModal; an
// empty current means add.
type labelNameModal struct {
	current string
	input   textinput.Model
	done    func(m *Model, name *string) tea.Cmd
}

func newLabelNameModal(current string, done func(m *Model, name *string) tea.Cmd) *labelNameModal {
	ti := textinput.New()
	ti.Placeholder = "Label name…"
	styleInput(&ti, colorSurface)
	ti.SetValue(current)
	return &labelNameModal{current: current, input: ti, done: done}
}

func (l *labelNameModal) Init(m *Model) tea.Cmd { return l.input.Focus() }

func (l *labelNameModal) Update(msg tea.Msg, m *Model) tea.Cmd {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		var cmd tea.Cmd
		l.input, cmd = l.input.Update(msg)
		return cmd
	}
	switch keyMsg.String() {
	case "esc":
		return l.done(m, nil)
	case "enter":
		name := strings.TrimSpace(l.input.Value())
		if name == "" {
			return nil // an empty name keeps the dialog open
		}
		return l.done(m, &name)
	}
	var cmd tea.Cmd
	l.input, cmd = l.input.Update(keyMsg)
	return cmd
}

func (l *labelNameModal) View(m *Model) string {
	heading := "New Label"
	if l.current != "" {
		heading = "Rename Label"
	}
	body := accentBold.Render(heading) + "\n\n" + l.input.View() +
		"\n\n" + modalHintStyle.Render("Enter: save   Esc: cancel")
	return renderModal(modalBoxStyle, 48, body)
}
