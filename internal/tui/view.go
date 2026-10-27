package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const searchHint = "type to filter  Enter:keep  Esc:clear  ↑↓:select  ←→:column"

const footerHint = "n:New  e:Edit  d:Delete  Enter:View  ←→/hl:Col  ↑↓/kj:Row  " +
	"Shift←→/[]:Move  f:Filter  /:Search  t:Labels  o:Notes  c:Toggle Ends  H:Herdr  m:Matrix  r:Refresh  q:Quit"

// View renders the board (title bar, columns, footer hint, toast line). An
// open modal replaces the board and is centered on the frame rather than
// overlaid on it.
func (m *Model) View() string {
	if m.quitting {
		return ""
	}
	if m.width <= 0 || m.height <= 0 {
		return "Loading…"
	}

	var frame string
	if top := m.topModal(); top != nil {
		frame = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, top.View(m),
			lipgloss.WithWhitespaceBackground(colorPanel))
	} else {
		frame = m.renderBoardFrame()
	}
	return paintCanvas(frame, m.width, m.height)
}

// paintCanvas lays the Polar Night background (nord0) under every cell of the
// frame, including bars, gaps and the area around a modal, and pads short
// frames with blank painted rows. It never crops, so a layout that overflows
// the terminal still shows up in the height tests.
func paintCanvas(frame string, width, height int) string {
	frame = paintBg(frame, colorPanel, width)
	if rows := strings.Count(frame, "\n") + 1; rows < height {
		blank := paintBg("", colorPanel, width)
		frame += strings.Repeat("\n"+blank, height-rows)
	}
	return frame
}

// oneLine hard-truncates s to one screen line. lipgloss.Height only counts
// "\n"s, so a bar that wraps visually (the title bar embeds an arbitrary
// search query, the toast unbounded card titles) would make the reserved
// chrome height undercount and push the bottom of the board off screen.
func oneLine(width int, s string) string {
	return lipgloss.NewStyle().MaxWidth(width).Render(s)
}

func (m *Model) renderBoardFrame() string {
	titleBar := oneLine(m.width, m.renderTitleBar())
	// Unlike the title and toast bars, the footer hint may wrap: an explicit
	// Width makes it real lines, which lipgloss.Height below counts.
	hint := footerHint
	if m.popup {
		hint = strings.Replace(hint, "q:Quit", "Esc/q/F15:Close", 1)
	}
	if m.searching {
		hint = searchHint
	}
	footerBar := hintStyle.Width(m.width).Render(hint)
	toastBar := oneLine(m.width, m.renderToast())

	top := []string{titleBar}
	if m.searching || m.searchQuery != "" {
		top = append(top, oneLine(m.width, m.renderSearchBar()))
	}

	reserved := len(top) + lipgloss.Height(footerBar) + 1
	boardHeight := m.height - reserved
	if boardHeight < 4 {
		boardHeight = 4
	}
	bodyHeight := boardHeight - 3 // column border (2) + column header (1)
	if bodyHeight < 1 {
		bodyHeight = 1
	}

	boardRow := m.renderColumns(bodyHeight)

	parts := append(top, boardRow, footerBar, toastBar)
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (m *Model) renderTitleBar() string {
	subtitle := "Terminal Kanban Board"
	var filters []string
	if m.labelFilter != "" {
		filters = append(filters, "#"+m.labelFilter)
	}
	if len(filters) > 0 {
		subtitle = "Filter: " + strings.Join(filters, " + ")
	}
	return accentBold.Render("KanbanTUI") + "  " + hintStyle.Render(subtitle)
}

// renderSearchBar is the inline live-search row: the focused input while
// typing, a dimmed read-only echo of the committed query otherwise.
func (m *Model) renderSearchBar() string {
	if m.searching {
		return m.searchInput.View()
	}
	return hintStyle.Render("/ " + m.searchQuery)
}

func (m *Model) renderToast() string {
	if m.toastMsg == "" {
		return ""
	}
	switch m.toastKind {
	case toastWarn:
		return toastWarnStyle.Render(m.toastMsg)
	case toastError:
		return toastErrorStyle.Render(m.toastMsg)
	default:
		return toastInfoStyle.Render(m.toastMsg)
	}
}

// visibleStatuses returns all five columns, or just the three middle ones
// when Backlog/Completed are hidden; the remaining columns expand to fill
// the freed width.
func (m *Model) visibleStatuses() []string {
	if m.showEnds {
		return statuses
	}
	return statuses[1:4]
}

func (m *Model) renderColumns(bodyHeight int) string {
	cols := m.visibleStatuses()
	n := len(cols)
	borderOverhead := n * 2
	avail := m.width - borderOverhead
	if avail < n {
		avail = n * 12
	}
	colWidth := avail / n

	rendered := make([]string, n)
	for i, status := range cols {
		rendered[i] = m.renderColumn(status, colWidth, bodyHeight)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, rendered...)
}

func (m *Model) renderColumn(status string, colWidth, bodyHeight int) string {
	active := statuses[m.col] == status
	visible := m.visibleCards(status)
	total := len(m.data[status])

	countStr := fmt.Sprintf("%d", total)
	if m.labelFilter != "" || m.searchQuery != "" {
		countStr = fmt.Sprintf("%d/%d", len(visible), total)
	}
	// A WIP limit (see wip.go) replaces the generic ">5 cards" warning: the
	// header shows n/limit and turns red only once the limit is exceeded. While
	// a filter or search is active it shows visible/total max N instead.
	limit := wipLimits[status]
	if limit > 0 {
		if m.labelFilter != "" || m.searchQuery != "" {
			countStr += fmt.Sprintf(" max %d", limit)
		} else {
			countStr = fmt.Sprintf("%d/%d", total, limit)
		}
	}
	headerText := fmt.Sprintf("%s  (%s)", statusLabel[status], countStr)

	overloaded := status != "todo" && status != "completed" && total > 5
	if limit > 0 {
		overloaded = total > limit
	}
	var hs lipgloss.Style
	switch {
	case active && overloaded:
		hs = headerActiveOver
	case active:
		hs = headerActiveStyle
	case overloaded:
		hs = headerOverloaded
	default:
		hs = headerStyle
	}
	header := hs.Width(colWidth).Render(headerText)

	blocks := make([]string, len(visible))
	heights := make([]int, len(visible))
	for i, c := range visible {
		content := strings.Join(cardBlockLines(c, m.agents[c.ID]), "\n")
		st := cardStyle
		if active && i == m.row {
			st = cardSelectedStyle
		}
		// Paint the surface after wrapping/padding so continuation lines and
		// trailing cells keep it (width 0: the style already padded). The gap
		// row is added afterwards, on the column background.
		card := paintBg(st.Width(colWidth-2).Render(content), colorSurface, 0)
		block := card + "\n" + strings.Repeat(" ", lipgloss.Width(card))
		blocks[i] = block
		heights[i] = lipgloss.Height(block)
	}

	anchor := 0
	if active {
		anchor = m.row
	}
	start, end := visibleWindow(heights, anchor, bodyHeight)

	var body string
	if len(blocks) > 0 {
		body = strings.Join(blocks[start:end], "\n")
	}

	full := lipgloss.JoinVertical(lipgloss.Left, header, body)

	cs := columnStyle
	if active {
		cs = columnFocusedStyle
	}
	// visibleWindow always keeps the anchor card, even if it alone is taller
	// than bodyHeight. MaxHeight (+2 for the column's top/bottom border) keeps
	// that card from pushing the frame past the terminal height.
	return cs.Width(colWidth).Height(bodyHeight + 1).MaxHeight(bodyHeight + 3).Render(full)
}

// visibleWindow picks the [start,end) slice of blocks that fits within
// maxHeight (by cumulative height) while keeping index anchor visible, which
// scrolls the selected card into view. Anchor 0 just trims from the bottom.
func visibleWindow(heights []int, anchor, maxHeight int) (int, int) {
	n := len(heights)
	if n == 0 {
		return 0, 0
	}
	if anchor < 0 {
		anchor = 0
	}
	if anchor >= n {
		anchor = n - 1
	}
	sum := func(a, b int) int {
		s := 0
		for i := a; i < b; i++ {
			s += heights[i]
		}
		return s
	}
	start, end := 0, n
	for start < anchor && sum(start, end) > maxHeight {
		start++
	}
	for end > anchor+1 && sum(start, end) > maxHeight {
		end--
	}
	return start, end
}
