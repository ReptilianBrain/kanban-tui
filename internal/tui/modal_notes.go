package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/ReptilianBrain/kanban-tui/internal/db"
)

const notesPlaceholderAdd = "Type a note, Enter to add…"
const notesPlaceholderEdit = "Editing note — Enter to save, Esc to cancel"

// notesModal is the quick-notes list. While the input is focused, only
// up/down/escape/enter are handled here and everything else is typed into
// it (so d/e/p/o/q are text); those letters become list commands once Escape
// has blurred the input into "list mode". Escape is two-stage: the first
// press blurs (or cancels an edit), a second one closes the modal.
type notesModal struct {
	notes  []db.Note
	row    int
	input  textinput.Model
	editID *int64

	// done receives the promoted note, or nil if the modal closed without
	// promoting one.
	done func(m *Model, promoted *db.Note) tea.Cmd
}

func newNotesModal(store *db.DB, done func(m *Model, promoted *db.Note) tea.Cmd) *notesModal {
	ti := textinput.New()
	ti.Placeholder = notesPlaceholderAdd
	styleInput(&ti, colorSurface)
	n := &notesModal{input: ti, done: done}
	n.reload(store)
	return n
}

func (n *notesModal) reload(store *db.DB) {
	notes, _ := store.GetNotes()
	n.notes = notes
}

func (n *notesModal) clamp() {
	if len(n.notes) == 0 {
		n.row = 0
		return
	}
	if n.row >= len(n.notes) {
		n.row = len(n.notes) - 1
	}
}

func (n *notesModal) Init(m *Model) tea.Cmd { return n.input.Focus() }

func flattenNote(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

func (n *notesModal) submit(m *Model) tea.Cmd {
	text := strings.TrimSpace(n.input.Value())
	if text == "" {
		return nil
	}
	if n.editID != nil {
		_ = m.store.UpdateNote(*n.editID, text)
		editedID := *n.editID
		n.editID = nil
		n.input.SetValue("")
		n.input.Placeholder = notesPlaceholderAdd
		n.reload(m.store)
		n.row = 0
		for i, note := range n.notes {
			if note.ID == editedID {
				n.row = i
				break
			}
		}
		n.input.Blur() // came from list mode — return there
		return nil
	}
	_, _ = m.store.CreateNote(text)
	n.input.SetValue("")
	n.reload(m.store)
	n.row = 0 // newest first — select what was just added
	return nil
}

func (n *notesModal) smartEscape(m *Model) tea.Cmd {
	if n.input.Focused() {
		if n.editID != nil {
			n.editID = nil
			n.input.SetValue("")
			n.input.Placeholder = notesPlaceholderAdd
		}
		n.input.Blur()
		return nil
	}
	m.popModal()
	return n.done(m, nil)
}

func (n *notesModal) Update(msg tea.Msg, m *Model) tea.Cmd {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		if n.input.Focused() {
			var cmd tea.Cmd
			n.input, cmd = n.input.Update(msg)
			return cmd
		}
		return nil
	}

	if n.input.Focused() {
		switch keyMsg.String() {
		case "up":
			if n.row > 0 {
				n.row--
			}
			return nil
		case "down":
			if n.row < len(n.notes)-1 {
				n.row++
			}
			return nil
		case "esc":
			return n.smartEscape(m)
		case "enter":
			return n.submit(m)
		}
		var cmd tea.Cmd
		n.input, cmd = n.input.Update(keyMsg)
		return cmd
	}

	// List mode.
	switch keyMsg.String() {
	case "up", "k":
		if n.row > 0 {
			n.row--
		}
	case "down", "j":
		if n.row < len(n.notes)-1 {
			n.row++
		}
	case "d":
		if len(n.notes) == 0 {
			return nil
		}
		_ = m.store.DeleteNote(n.notes[n.row].ID)
		n.reload(m.store)
		n.clamp()
		return m.toast(toastWarn, "Note deleted")
	case "e":
		if len(n.notes) == 0 {
			return nil
		}
		note := n.notes[n.row]
		n.editID = &note.ID
		n.input.SetValue(flattenNote(note.Text))
		n.input.Placeholder = notesPlaceholderEdit
		return n.input.Focus()
	case "p":
		if len(n.notes) == 0 {
			return nil
		}
		note := n.notes[n.row]
		m.popModal()
		return n.done(m, &note)
	case "i":
		return n.input.Focus()
	case "esc", "o", "q":
		m.popModal()
		return n.done(m, nil)
	}
	return nil
}

func (n *notesModal) View(m *Model) string {
	var b strings.Builder
	b.WriteString(accentBold.Render("Quick Notes"))
	b.WriteString("\n\n")
	b.WriteString(n.input.View())
	b.WriteString("\n\n")
	if len(n.notes) == 0 {
		b.WriteString(dimStyle.Render("No notes"))
	} else {
		for i, note := range n.notes {
			text := flattenNote(note.Text)
			date := note.CreatedAt
			if len(date) >= 10 {
				date = date[:10]
			}
			bar := "  "
			if i == n.row {
				bar = selectedBarStyle.Render("▌") + " "
			}
			b.WriteString(bar + textStyle.Render(text) + "  " + dimStyle.Render(date))
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")
	b.WriteString(hintStyle.Render(
		"Enter:Add  ↑/↓:Select  Esc:List mode  Esc Esc:Close\n" +
			"List mode — e:Edit  d:Delete  p:Promote to card  i:Type  o:Close",
	))
	return renderModal(modalBoxStyle, 72, b.String())
}
