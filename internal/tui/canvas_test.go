package tui

import (
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// forceTrueColor makes Lip Gloss emit SGR codes. go test has no TTY, so the
// profile would otherwise be Ascii and every escape-based assertion would
// pass vacuously.
func forceTrueColor(t *testing.T) {
	t.Helper()
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
}

func TestPaintBgReemitsAfterInnerResets(t *testing.T) {
	forceTrueColor(t)
	seq := bgSeq(colorPanel)
	if seq == "" {
		t.Fatal("bgSeq empty under TrueColor")
	}
	in := dimStyle.Render("#12") + " " + boldStyle.Render("title")
	out := paintBg(in, colorPanel, 0)

	if !strings.HasPrefix(out, seq) {
		t.Errorf("line does not start with bg sequence: %q", out)
	}
	if !strings.HasSuffix(out, "\x1b[0m") {
		t.Errorf("line does not end with a reset: %q", out)
	}
	// Every reset except the closing one must be followed by the bg sequence.
	body := strings.TrimSuffix(out, "\x1b[0m")
	if n, want := strings.Count(body, "\x1b[0m"), strings.Count(body, "\x1b[0m"+seq); n != want {
		t.Errorf("%d resets, only %d re-select the bg: %q", n, want, out)
	}
}

func TestPaintBgPadsOnlyWithWidth(t *testing.T) {
	forceTrueColor(t)
	if w := lipgloss.Width(paintBg("abc", colorPanel, 0)); w != 3 {
		t.Errorf("width 0 padded the line to %d cells", w)
	}
	if w := lipgloss.Width(paintBg("abc", colorPanel, 10)); w != 10 {
		t.Errorf("width 10 gave %d cells", w)
	}
}

func TestViewHasNoDefaultBackgroundCells(t *testing.T) {
	forceTrueColor(t)
	m := newTestModel(t)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	seq := bgSeq(colorPanel)
	check := func(name, view string) {
		for i, line := range strings.Split(view, "\n") {
			if !strings.HasPrefix(line, seq) {
				t.Errorf("%s line %d does not start with the canvas bg: %q", name, i, line)
			}
			body := strings.TrimSuffix(line, "\x1b[0m")
			for _, idx := range regexp.MustCompile(`\x1b\[0m`).FindAllStringIndex(body, -1) {
				rest := body[idx[1]:]
				if !strings.HasPrefix(rest, seq) {
					t.Fatalf("%s line %d: reset not followed by the canvas bg: %q", name, i, body[idx[0]:])
				}
			}
		}
		if got := lipgloss.Height(view); got > 40 {
			t.Errorf("%s rendered %d rows, want <= 40", name, got)
		}
	}
	check("board", m.View())
}
