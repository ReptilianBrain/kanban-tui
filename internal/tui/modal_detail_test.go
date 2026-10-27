package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestCardDetailFillsTerminalHeight(t *testing.T) {
	m := newTestModel(t)
	id := addCard(t, m, "todo")
	card, _ := m.store.GetCard(id)
	card.Description = strings.Repeat("a long description line that wraps around the modal width ", 80)

	for _, size := range []struct{ w, h int }{{220, 60}, {120, 40}, {80, 24}} {
		m.Update(tea.WindowSizeMsg{Width: size.w, Height: size.h})
		view := newCardDetailModal(*card).View(m)
		if got, want := lipgloss.Height(view), size.h-detailBoardGap; got != want {
			t.Errorf("%dx%d: modal is %d rows, want %d", size.w, size.h, got, want)
		}
		if got := lipgloss.Width(view); got > size.w {
			t.Errorf("%dx%d: modal is %d cols wide", size.w, size.h, got)
		}
	}
}
