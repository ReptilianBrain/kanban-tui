package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ReptilianBrain/kanban-tui/internal/config"
	"github.com/ReptilianBrain/kanban-tui/internal/db"
	"github.com/ReptilianBrain/kanban-tui/internal/herdrsync"
)

// handleBoardKey dispatches a keypress when no modal is open.
func (m *Model) handleBoardKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "n":
		return m.actionNewCard()
	case "e":
		return m.actionEditCard()
	case "d":
		return m.actionDeleteCard()
	case "enter":
		return m.actionViewCard()
	case "left", "h":
		m.actionColPrev()
	case "right", "l":
		m.actionColNext()
	case "up", "k":
		m.actionCardUp()
	case "down", "j":
		m.actionCardDown()
	case "shift+left", "[":
		return m.actionMoveCardLeft()
	case "shift+right", "]":
		return m.actionMoveCardRight()
	case "shift+up", "{":
		return m.actionMoveCardUp()
	case "shift+down", "}":
		return m.actionMoveCardDown()
	case "f":
		return m.actionFilter()
	case "m":
		return m.actionMatrix()
	case "/":
		return m.actionSearch()
	case "esc":
		// In a herdr popup nothing else closes the app, so Esc with no filter
		// or search left to clear quits.
		if m.popup && m.labelFilter == "" && m.searchQuery == "" {
			m.quitting = true
			return tea.Quit
		}
		return m.actionClearFilter()
	case "t":
		return m.actionManageLabels()
	case "o":
		return m.actionNotes()
	case "c":
		return m.actionToggleCompleted()
	case "H":
		return m.actionOpenHerdr()
	case "r":
		return m.actionRefresh()
	case "q":
		m.quitting = true
		return tea.Quit
	}
	return nil
}

func (m *Model) actionColPrev() {
	minCol := 0
	if !m.showEnds {
		minCol = 1
	}
	if m.col > minCol {
		m.col--
		m.clampRow()
	}
}

func (m *Model) actionColNext() {
	maxCol := len(statuses) - 1
	if !m.showEnds {
		maxCol = len(statuses) - 2
	}
	if m.col < maxCol {
		m.col++
		m.clampRow()
	}
}

func (m *Model) actionCardUp() {
	if m.row > 0 {
		m.row--
	}
}

func (m *Model) actionCardDown() {
	cards := m.visibleCards(statuses[m.col])
	if m.row < len(cards)-1 {
		m.row++
	}
}

// capitalizeFirst upper-cases only the first rune and leaves the rest as is.
func capitalizeFirst(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	return strings.ToUpper(string(r[0])) + string(r[1:])
}

// Card moves guard only on column index, not on whether Backlog/Completed are
// hidden, so with the ends hidden a card moved left from In Progress still
// lands in the hidden Backlog column (and the cursor follows it).

func (m *Model) actionMoveCardLeft() tea.Cmd {
	card := m.currentCard()
	if card == nil || m.col == 0 {
		return nil
	}
	newStatus := statuses[m.col-1]
	_ = m.store.UpdateCardStatus(card.ID, newStatus)
	m.col--
	_ = m.loadData()
	m.selectCard(newStatus, card.ID)
	if hint := db.CompletionHint(card.Description, newStatus); hint != nil {
		return m.toast(toastWarn, capitalizeFirst(*hint))
	}
	return m.toast(toastInfo, "Moved to "+statusLabel[newStatus])
}

func (m *Model) actionMoveCardRight() tea.Cmd {
	card := m.currentCard()
	if card == nil || m.col == len(statuses)-1 {
		return nil
	}
	newStatus := statuses[m.col+1]
	_ = m.store.UpdateCardStatus(card.ID, newStatus)
	m.col++
	_ = m.loadData()
	m.selectCard(newStatus, card.ID)
	if hint := db.CompletionHint(card.Description, newStatus); hint != nil {
		return m.toast(toastWarn, capitalizeFirst(*hint))
	}
	return m.toast(toastInfo, "Moved to "+statusLabel[newStatus])
}

func (m *Model) actionMoveCardUp() tea.Cmd   { return m.reorderCard(-1) }
func (m *Model) actionMoveCardDown() tea.Cmd { return m.reorderCard(1) }

// reorderCard only toasts: every column is sorted by created_at DESC
// (Completed by completion time), so a manual position swap would have no
// visible effect.
func (m *Model) reorderCard(delta int) tea.Cmd {
	if m.currentCard() == nil {
		return nil
	}
	return m.toast(toastWarn, "Columns are sorted newest-first, not manually reorderable")
}

func (m *Model) actionFilter() tea.Cmd {
	return m.pushModal(newFilterModal(m.store, m.labelFilter, func(m *Model, result *string) tea.Cmd {
		if result == nil {
			return nil
		}
		m.labelFilter = *result
		m.clampRow()
		if *result != "" {
			return m.toast(toastInfo, "Filtering by #"+*result)
		}
		return m.toast(toastInfo, "Filter cleared")
	}))
}

// actionSearch opens the inline live-search bar (re-focusing it with the
// committed query intact if one is already active).
func (m *Model) actionSearch() tea.Cmd {
	m.searching = true
	m.searchInput.SetValue(m.searchQuery)
	m.searchInput.CursorEnd()
	return m.searchInput.Focus()
}

// handleSearchKey routes a keypress while the search bar has focus. Every
// edit re-filters the board immediately; Enter keeps the filter and hands
// focus back to the board, Esc clears it.
func (m *Model) handleSearchKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "enter":
		m.searching = false
		m.searchInput.Blur()
		return nil
	case "esc":
		m.clearSearch()
		m.clampRow()
		return nil
	case "up", "ctrl+p":
		m.actionCardUp()
		return nil
	case "down", "ctrl+n":
		m.actionCardDown()
		return nil
	case "left":
		m.actionColPrev()
		return nil
	case "right":
		m.actionColNext()
		return nil
	}
	var cmd tea.Cmd
	m.searchInput, cmd = m.searchInput.Update(msg)
	if q := strings.TrimSpace(m.searchInput.Value()); q != m.searchQuery {
		m.searchQuery = q
		m.row = 0
		m.clampRow()
	}
	return cmd
}

// clearSearch drops the query and closes the bar.
func (m *Model) clearSearch() {
	m.searchQuery = ""
	m.searching = false
	m.searchInput.SetValue("")
	m.searchInput.Blur()
}

func (m *Model) actionClearFilter() tea.Cmd {
	if m.labelFilter != "" || m.searchQuery != "" {
		m.labelFilter = ""
		m.clearSearch()
		m.clampRow()
		return m.toast(toastInfo, "Filters cleared")
	}
	return nil
}

func (m *Model) actionManageLabels() tea.Cmd {
	return m.pushModal(newLabelManagerModal(m.store, func(m *Model) tea.Cmd {
		m.refresh() // labels may have changed and cards referenced deleted ones
		return nil
	}))
}

func firstLineOf(text string) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return ""
	}
	lines := strings.SplitN(text, "\n", 2)
	return strings.TrimSpace(lines[0])
}

func truncateTitle60(s string) string {
	r := []rune(s)
	if len(r) <= 60 {
		return s
	}
	return strings.TrimRight(string(r[:57]), " ") + "…"
}

func (m *Model) actionNotes() tea.Cmd {
	return m.pushModal(newNotesModal(m.store, func(m *Model, promoted *db.Note) tea.Cmd {
		if promoted == nil {
			return nil
		}
		note := promoted
		firstLine := firstLineOf(note.Text)
		if firstLine == "" {
			firstLine = "Note"
		}
		title := truncateTitle60(firstLine)

		var labelPre *string
		if m.labelFilter != "" {
			lf := m.labelFilter
			labelPre = &lf
		}
		status := statuses[m.col]

		return m.pushModal(newCardModal(m.store, title, note.Text, "medium", "", labelPre, "", nil,
			func(m *Model, result *cardFormResult) tea.Cmd {
				if result == nil {
					return nil // cancelled — keep the note
				}
				id, _ := m.store.CreateCard(result.Title, result.Description, status, result.Priority, result.DueDate, result.Label, result.JiraKey)
				_ = m.store.DeleteNote(note.ID)
				_ = m.loadData()
				m.selectCard(status, id)
				return m.toast(toastInfo, fmt.Sprintf("Note promoted to %s in %s", cardNoun(), statusLabel[status]))
			}))
	}))
}

func (m *Model) actionToggleCompleted() tea.Cmd {
	m.showEnds = !m.showEnds
	if !m.showEnds {
		switch m.col {
		case len(statuses) - 1:
			m.col = len(statuses) - 2
		case 0:
			m.col = 1
		}
		m.clampRow()
	}
	state := "shown"
	if !m.showEnds {
		state = "hidden"
	}
	return m.toast(toastInfo, statusLabel["todo"]+"/"+statusLabel["completed"]+" columns "+state)
}

// herdrOpenDoneMsg reports the result of the herdr pickup, which opens the
// selected card's work in an isolated worktree-backed herdr pane running a
// fresh Claude Code session seeded with the card's context.

type herdrOpenDoneMsg struct {
	cardID int64
	result *herdrsync.OpenResult
	err    error
}

func (m *Model) actionOpenHerdr() tea.Cmd {
	card := m.currentCard()
	if card == nil {
		return nil
	}
	if m.herdrBusy {
		return m.toast(toastWarn, "Already opening a herdr workspace")
	}
	repos, err := config.LoadRepos()
	if err != nil {
		return m.toast(toastError, err.Error())
	}
	if len(repos) == 0 {
		return m.toast(toastWarn, "No repos configured — add one to ~/.kanbantui/repos.json")
	}

	cardID := card.ID
	title := card.Title
	description := card.Description

	return m.pushModal(newHerdrRepoModal(repos, func(m *Model, repo config.RepoEntry) tea.Cmd {
		m.herdrBusy = true
		branch := herdrsync.BranchName(cardID, title)
		agentName := fmt.Sprintf("card-%d", cardID)
		label := fmt.Sprintf("#%d %s", cardID, title)
		store := m.store

		toastCmd := m.toast(toastInfo, "Opening "+branch+" in herdr…")
		// The card move runs before OpenCard so the seed prompt can state it.
		openCmd := func() tea.Msg {
			_ = store.UpdateCardStatus(cardID, "ongoing")
			prompt := herdrsync.SeedPrompt(cardID, title, description, branch)
			result, err := herdrsync.OpenCard(repo.Path, repo.Base, branch, agentName, label, prompt)
			return herdrOpenDoneMsg{cardID: cardID, result: result, err: err}
		}
		return tea.Batch(toastCmd, openCmd)
	}))
}

func (m *Model) handleHerdrOpenDone(msg herdrOpenDoneMsg) tea.Cmd {
	m.herdrBusy = false
	m.refresh()
	if msg.err != nil {
		return m.toast(toastError, msg.err.Error())
	}
	warn := ""
	// The pickup just moved the card into In Progress; flag a blown WIP limit.
	if limit, n := wipLimits["ongoing"], len(m.data["ongoing"]); limit > 0 && n > limit {
		warn += fmt.Sprintf(" — %s over WIP limit (%d/%d)", statusLabel["ongoing"], n, limit)
	}
	kind := toastInfo
	if warn != "" {
		kind = toastWarn
	}
	text := fmt.Sprintf("Opened %s in herdr, %s moved to %s%s", msg.result.Branch, cardNoun(), statusLabel["ongoing"], warn)
	if msg.result.Reused {
		text = fmt.Sprintf("Reconnected to %s in herdr%s", msg.result.Branch, warn)
	}
	// Herdr already focused the new workspace; a popup would keep holding
	// input over it until the app exits, as after a Matrix jump.
	if m.popup {
		m.quitting = true
		return tea.Quit
	}
	return m.toast(kind, text)
}

func (m *Model) actionRefresh() tea.Cmd {
	m.refresh()
	return m.toast(toastInfo, "Refreshed")
}

func (m *Model) actionNewCard() tea.Cmd {
	status := statuses[m.col]
	var labelPre *string
	if m.labelFilter != "" {
		lf := m.labelFilter
		labelPre = &lf
	}
	return m.pushModal(newCardModal(m.store, "", "", "medium", "", labelPre, "", nil,
		func(m *Model, result *cardFormResult) tea.Cmd {
			if result == nil {
				return nil
			}
			id, _ := m.store.CreateCard(result.Title, result.Description, status, result.Priority, result.DueDate, result.Label, result.JiraKey)
			_ = m.loadData()
			m.selectCard(status, id)
			return nil
		}))
}

func (m *Model) actionEditCard() tea.Cmd {
	card := m.currentCard()
	if card == nil {
		return nil
	}
	cardID := card.ID
	due := ""
	if card.DueDate != nil {
		due = *card.DueDate
	}
	jiraKey := ""
	if card.JiraKey != nil {
		jiraKey = *card.JiraKey
	}
	return m.pushModal(newCardModal(m.store, card.Title, card.Description, card.Priority, due, card.Label, jiraKey, &cardID,
		func(m *Model, result *cardFormResult) tea.Cmd {
			if result == nil {
				return nil
			}
			_ = m.store.UpdateCard(cardID, result.Title, result.Description, result.Priority, result.DueDate, result.Label, result.JiraKey)
			m.refresh()
			return nil
		}))
}

func (m *Model) actionDeleteCard() tea.Cmd {
	card := m.currentCard()
	if card == nil {
		return nil
	}
	cardID := card.ID
	title := card.Title
	return m.pushModal(newDeleteConfirmModal(title, func(m *Model, confirmed bool) tea.Cmd {
		if !confirmed {
			return nil
		}
		_ = m.store.DeleteCard(cardID)
		m.refresh()
		return m.toast(toastWarn, fmt.Sprintf("Deleted %q", title))
	}))
}

func (m *Model) actionViewCard() tea.Cmd {
	card := m.currentCard()
	if card == nil {
		return nil
	}
	return m.pushModal(newCardDetailModal(*card))
}
