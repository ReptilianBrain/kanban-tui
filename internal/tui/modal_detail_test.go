package tui

import (
	"fmt"
	"os"
	"path/filepath"
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

func TestCardDetailShowsCycleFromTheCostFiles(t *testing.T) {
	m := newTestModel(t)
	m.costDir = t.TempDir()
	id := addCard(t, m, "ongoing")
	card, _ := m.store.GetCard(id)
	if view := newCardDetailModal(*card).View(m); strings.Contains(view, "Cycle") {
		t.Fatalf("no cost recorded yet, detail should show no Cycle:\n%s", view)
	}

	path := filepath.Join(m.costDir, fmt.Sprintf("card-%d.json", id))
	if err := os.WriteFile(path, []byte(`{"a":1.25,"b":0.5}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if view := newCardDetailModal(*card).View(m); !strings.Contains(view, "Cycle: $1.75") {
		t.Errorf("card detail lacks the summed Cycle:\n%s", view)
	}
}
