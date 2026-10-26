package tui

import tea "github.com/charmbracelet/bubbletea"

// modal is one entry in the root Model's modal stack. Update receives every
// Bubble Tea message while the modal is on top and mutates itself and/or the
// root Model directly; closures passed in at construction time deliver the
// result to the caller. View renders the modal's own box; the root
// composites it centered over the board.
//
// labelManagerModal hosts a private child modal (labelNameModal) itself
// rather than pushing it on the root stack.
type modal interface {
	// Init runs once, immediately after the modal is pushed (e.g. to focus
	// an input or kick off async work), returning any resulting Cmd.
	Init(m *Model) tea.Cmd
	Update(msg tea.Msg, m *Model) tea.Cmd
	View(m *Model) string
}

// pushModal opens a modal on top of the stack and runs its Init.
func (m *Model) pushModal(mod modal) tea.Cmd {
	m.modalStack = append(m.modalStack, mod)
	return mod.Init(m)
}

func (m *Model) popModal() {
	if len(m.modalStack) > 0 {
		m.modalStack = m.modalStack[:len(m.modalStack)-1]
	}
}

// topModal returns nil when the stack is empty.
func (m *Model) topModal() modal {
	if len(m.modalStack) == 0 {
		return nil
	}
	return m.modalStack[len(m.modalStack)-1]
}
