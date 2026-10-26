package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"
)

// Color palette: Nord (https://www.nordtheme.com/docs/colors-and-palettes) on
// a Polar Night canvas. The whole frame is painted on nord0 (see paintBg and
// View), so the board looks the same on any terminal theme. Other files
// should use the semantic roles below, never raw nordN values.
//
//	Polar Night: nord0 #2E3440  nord1 #3B4252  nord2 #434C5E  nord3 #4C566A
//	Snow Storm:  nord4 #D8DEE9  nord5 #E5E9F0  nord6 #ECEFF4
//	Frost:       nord7 #8FBCBB  nord8 #88C0D0  nord9 #81A1C1  nord10 #5E81AC
//	Aurora:      nord11 #BF616A nord12 #D08770 nord13 #EBCB8B nord14 #A3BE8C nord15 #B48EAD

// nord builds a palette color with explicit 256/16-color fallbacks: Lip
// Gloss's automatic truecolor-to-256 downsampling maps nord0 to a teal (23),
// which would tint the whole canvas on 256-color terminals.
func nord(trueColor, ansi256, ansi string) lipgloss.CompleteColor {
	return lipgloss.CompleteColor{TrueColor: trueColor, ANSI256: ansi256, ANSI: ansi}
}

var (
	colorMuted   = nord("#4C566A", "60", "8")   // dim/muted text (nord3)
	colorText    = nord("#D8DEE9", "253", "7")  // normal foreground (nord4)
	colorHeading = nord("#E5E9F0", "254", "15") // card titles / emphasized values (nord5)
	colorAccent  = nord("#88C0D0", "110", "6")  // focused column / headings (nord8)
	colorWarning = nord("#EBCB8B", "186", "3")  // due soon / aging mid / high priority (nord13)
	colorError   = nord("#BF616A", "131", "1")  // overdue / aging high / critical (nord11)
	colorSuccess = nord("#A3BE8C", "144", "2")  // success (nord14)
	colorCyan    = nord("#8FBCBB", "109", "6")  // ordinary labels (nord7)
	colorSurface = nord("#3B4252", "238", "0")  // card/box background (nord1)
	colorPanel   = nord("#2E3440", "236", "0")  // column background (nord0)
	colorBorder  = nord("#434C5E", "239", "8")  // unfocused border (nord2)

	// Text drawn on top of a colorAccent/colorWarning/colorError fill.
	colorOnFill  = nord("#2E3440", "236", "0")  // nord0
	colorOnError = nord("#ECEFF4", "255", "15") // nord6
)

var (
	dimStyle       = lipgloss.NewStyle().Foreground(colorMuted)
	textStyle      = lipgloss.NewStyle().Foreground(colorText)
	boldStyle      = lipgloss.NewStyle().Bold(true).Foreground(colorHeading)
	warnStyle      = lipgloss.NewStyle().Foreground(colorWarning)
	warnBoldStyle  = lipgloss.NewStyle().Foreground(colorWarning).Bold(true)
	errorStyle     = lipgloss.NewStyle().Foreground(colorError)
	errorBoldStyle = lipgloss.NewStyle().Foreground(colorError).Bold(true)
	successStyle   = lipgloss.NewStyle().Foreground(colorSuccess)
	accentStyle    = lipgloss.NewStyle().Foreground(colorAccent)
	accentBold     = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	cyanStyle      = lipgloss.NewStyle().Foreground(colorCyan)
	plainStyle     = lipgloss.NewStyle().Foreground(colorText)

	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorText).
			Background(colorPanel).
			Align(lipgloss.Center)
	// The focused column is marked by its border, so its header text is not restyled.
	headerActiveStyle = headerStyle
	headerOverloaded  = headerStyle.Foreground(colorError)
	headerActiveOver  = headerOverloaded

	columnStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBorder)
	// The focused column differs only in its border color.
	columnFocusedStyle = columnStyle.BorderForeground(colorAccent)

	// The left border column replaces left padding so the selected card gets a
	// highlighted edge instead of a background fill; unselected, the border
	// blends into the column background. The gap row between cards is added
	// by the caller (see renderColumn) so paintBg doesn't cover it.
	cardStyle = lipgloss.NewStyle().
			Border(lipgloss.Border{Left: "▌"}, false, false, false, true).
			BorderForeground(colorPanel).
			BorderBackground(colorPanel).
			Background(colorSurface).
			Padding(1, 1, 1, 0)
	// The selected card differs only in its left-edge color.
	cardSelectedStyle = cardStyle.BorderForeground(colorAccent)

	toastInfoStyle  = lipgloss.NewStyle().Bold(true).Foreground(colorOnFill).Background(colorAccent).Padding(0, 1)
	toastWarnStyle  = lipgloss.NewStyle().Bold(true).Foreground(colorOnFill).Background(colorWarning).Padding(0, 1)
	toastErrorStyle = lipgloss.NewStyle().Bold(true).Foreground(colorOnError).Background(colorError).Padding(0, 1)

	modalBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.ThickBorder()).
			BorderForeground(colorAccent).
			Background(colorSurface).
			Foreground(colorText).
			Padding(1, 2)
	modalWarnBoxStyle = modalBoxStyle.BorderForeground(colorWarning)
	modalDangerStyle  = modalBoxStyle.BorderForeground(colorError)

	hintStyle   = lipgloss.NewStyle().Foreground(colorMuted)
	errMsgStyle = lipgloss.NewStyle().Foreground(colorError).Bold(true)

	// Selected-row bar for the notes and label-manager lists, like cardSelectedStyle.
	selectedBarStyle = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)

	// Picker values ("‹ value ›") in the card/filter/herdr modals.
	pickerArrowStyle = lipgloss.NewStyle().Foreground(colorMuted)
	pickerValueStyle = lipgloss.NewStyle().Foreground(colorHeading)
)

// pickerValue renders a left/right cycler value as "‹ value ›"; the focused
// picker's value takes the Frost accent.
func pickerValue(value string, focused bool) string {
	vs := pickerValueStyle
	if focused {
		vs = accentBold
	}
	return pickerArrowStyle.Render("‹ ") + vs.Render(value) + pickerArrowStyle.Render(" ›")
}

// bgSeq returns the SGR sequence that selects bg under the current color
// profile, or "" when the profile emits no color. Computed per call because
// the profile can change (tests force TrueColor).
func bgSeq(bg lipgloss.TerminalColor) string {
	out := lipgloss.NewStyle().Background(bg).Render("x")
	if i := strings.IndexByte(out, 'x'); i > 0 {
		return out[:i]
	}
	return ""
}

// paintBg gives s a background that survives nested styled segments. A Lip
// Gloss Background() on an outer style is cancelled by the "\x1b[0m" every
// inner Render ends with, so the rest of the line would fall back to the
// terminal background; paintBg re-selects bg after every reset and closes
// each line with a final reset so the color never leaks past it. With
// width > 0 each line is also padded with bg-colored spaces to that width.
// Only the outermost frame pass should pad: nested surfaces are padded by the
// Width/Padding of the style they render through, and pre-padding would wrap.
func paintBg(s string, bg lipgloss.TerminalColor, width int) string {
	seq := bgSeq(bg)
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if seq != "" {
			line = strings.ReplaceAll(line, "\x1b[0m", "\x1b[0m"+seq)
			line = strings.ReplaceAll(line, "\x1b[m", "\x1b[m"+seq)
		}
		if pad := width - lipgloss.Width(line); pad > 0 {
			line += strings.Repeat(" ", pad)
		}
		if seq != "" {
			line = seq + line + "\x1b[0m"
		}
		lines[i] = line
	}
	return strings.Join(lines, "\n")
}

// renderModal wraps body in the box style, painting the surface color behind
// every nested styled segment. Width 0 to paintBg: the box pads and wraps itself.
func renderModal(box lipgloss.Style, width int, body string) string {
	return box.Width(width).Render(paintBg(body, colorSurface, 0))
}

// styleInput recolors a text input to the palette. bg is the surface the
// input sits on. bubbles draws the cursor with Reverse, so the block takes
// the accent color and the character under it the surface color.
func styleInput(ti *textinput.Model, bg lipgloss.TerminalColor) {
	ti.PromptStyle = lipgloss.NewStyle().Foreground(colorAccent).Background(bg)
	ti.TextStyle = lipgloss.NewStyle().Foreground(colorText).Background(bg)
	ti.PlaceholderStyle = lipgloss.NewStyle().Foreground(colorMuted).Background(bg)
	ti.Cursor.Style = lipgloss.NewStyle().Foreground(colorAccent).Background(bg)
	ti.Cursor.TextStyle = lipgloss.NewStyle().Foreground(colorText).Background(bg)
}

// styleTextarea is styleInput for the description textarea, which always sits
// on the modal surface. The cursor line gets the surface color too, so it
// doesn't paint an off-palette stripe.
func styleTextarea(ta *textarea.Model) {
	for _, st := range []*textarea.Style{&ta.FocusedStyle, &ta.BlurredStyle} {
		st.CursorLine = lipgloss.NewStyle().Background(colorSurface)
		st.Text = lipgloss.NewStyle().Foreground(colorText).Background(colorSurface)
		st.Placeholder = lipgloss.NewStyle().Foreground(colorMuted).Background(colorSurface)
		st.Prompt = lipgloss.NewStyle().Foreground(colorAccent).Background(colorSurface)
		st.EndOfBuffer = lipgloss.NewStyle().Foreground(colorBorder).Background(colorSurface)
	}
	ta.Cursor.Style = lipgloss.NewStyle().Foreground(colorAccent).Background(colorSurface)
	ta.Cursor.TextStyle = lipgloss.NewStyle().Foreground(colorText).Background(colorSurface)
}
