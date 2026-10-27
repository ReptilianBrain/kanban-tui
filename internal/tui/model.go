// Package tui is the Bubble Tea board: columns, cards, modals and keybindings
// over the SQLite store, drawn on a Nord Polar Night canvas.
package tui

import (
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/ReptilianBrain/kanban-tui/internal/cost"
	"github.com/ReptilianBrain/kanban-tui/internal/db"
	"github.com/ReptilianBrain/kanban-tui/internal/herdrsync"
)

// cycleText renders a card's total spend (its Cycle), or "" with no data.
func (m *Model) cycleText(id int64) string {
	if total, ok := cost.CardTotal(m.costDir, id); ok {
		return fmt.Sprintf("$%.2f", total)
	}
	return ""
}

// statuses is the fixed column order.
var statuses = []string{"todo", "ongoing", "blocked", "in_review", "completed"}

// statusLabel maps each stored status value to its column label under the
// active skin (see lexicon.go); the stored value for In Progress is "ongoing"
// everywhere outside the display.
var statusLabel = activeSkin().status

var priorityOptions = []string{"low", "medium", "high", "critical"}

// toastKind selects the toast banner's style.
type toastKind int

const (
	toastInfo toastKind = iota
	toastWarn
	toastError
)

// Model is the root Bubble Tea model. It is mutated in place through pointer
// receivers, which keeps the modal callbacks simple.
type Model struct {
	store *db.DB

	width, height int

	col         int
	row         int
	showEnds    bool
	labelFilter string
	searchQuery string

	// searchInput/searching drive the inline live-search bar: while
	// searching, keys go to the input and searchQuery tracks it per keystroke.
	searchInput textinput.Model
	searching   bool

	data    map[string][]db.Card
	lastRev db.Revision

	modalStack []modal

	toastMsg  string
	toastKind toastKind
	toastSeq  int

	herdrBusy bool

	// agents maps card id to its herdr agent status (see agentsMsg); empty
	// until the first poll answers or when herdr is not installed.
	agents map[int64]string

	// agentPanes maps card id to the herdr pane of the agent behind its
	// status; focusAgent is a seam so tests never shell out.
	agentPanes map[int64]string
	focusAgent func(target string) error

	// watch moves cards between In Progress and Blocked from agent state;
	// clock is a seam so tests can fast-forward the waiting window.
	watch *watcher
	clock func() time.Time

	// costDir holds the per-card spend files (see internal/cost).
	costDir string

	// popup is set by KANBANTUI_POPUP=1 (the herdr keys.command that opens the
	// board in a popup). A popup receives all terminal input until the app
	// exits, so closing it is the app's job: Esc at the board, q, and F15
	// (the key that opened it) all quit.
	popup bool

	// matrixHome is set by KANBANTUI_START=matrix: the board opens straight
	// into the Matrix, and closing it quits instead of revealing the board.
	matrixHome bool

	quitting bool
}

// newModel builds a Model on an already-open store, so tests can use a
// temporary database instead of going through Run (which opens the real one).
func newModel(store *db.DB) *Model {
	ti := textinput.New()
	ti.Prompt = "/ "
	ti.Placeholder = "Fuzzy search title & description…"
	styleInput(&ti, colorPanel)
	return &Model{
		store:       store,
		showEnds:    true,
		searchInput: ti,
		data:        map[string][]db.Card{},
		watch:       newWatcher(),
		focusAgent:  herdrsync.FocusAgent,
		clock:       time.Now,
		costDir:     cost.Dir(),
		popup:       os.Getenv("KANBANTUI_POPUP") != "",
		matrixHome:  os.Getenv("KANBANTUI_START") == "matrix",
	}
}

// Run opens the default database and runs the board until the user quits.
func Run() error {
	store, err := db.Open()
	if err != nil {
		return fmt.Errorf("tui: opening database: %w", err)
	}
	defer store.Close()

	m := newModel(store)
	if err := m.loadData(); err != nil {
		return fmt.Errorf("tui: loading board: %w", err)
	}

	if m.matrixHome {
		m.actionMatrix()
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err = p.Run()
	return err
}

// Init starts the DB-change poll and the herdr agent poll; Run loads the
// board before the program starts.
func (m *Model) Init() tea.Cmd {
	return tea.Batch(tickCmd(), pollAgentsCmd(0))
}

// loadData reloads every card, buckets it by status and records the current
// revision. Recording it here is what keeps the poll from reporting the
// app's own writes as external changes.
func (m *Model) loadData() error {
	cards, err := m.store.GetAllCards()
	if err != nil {
		return err
	}
	data := make(map[string][]db.Card, len(statuses))
	for _, s := range statuses {
		data[s] = nil
	}
	for _, c := range cards {
		data[c.Status] = append(data[c.Status], c)
	}
	// Every column but Completed is newest-created first (id breaks ties
	// between same-instant inserts), so a fresh card lands on top. Completed
	// sorts by completion time instead. The stored position column does not
	// drive display order.
	for status, cs := range data {
		if status == "completed" {
			sort.SliceStable(cs, func(i, j int) bool {
				return completionTime(cs[i]) > completionTime(cs[j])
			})
			continue
		}
		sort.SliceStable(cs, func(i, j int) bool {
			if cs[i].CreatedAt != cs[j].CreatedAt {
				return cs[i].CreatedAt > cs[j].CreatedAt
			}
			return cs[i].ID > cs[j].ID
		})
	}
	m.data = data

	rev, err := m.store.GetRevision()
	if err != nil {
		return err
	}
	m.lastRev = rev
	return nil
}

// completionTime is status_changed_at, falling back to created_at when unset
// (same fallback as the aging indicator).
func completionTime(c db.Card) string {
	if c.StatusChangedAt != nil && *c.StatusChangedAt != "" {
		return *c.StatusChangedAt
	}
	return c.CreatedAt
}

// refresh reloads the board and clamps the cursor, silently.
func (m *Model) refresh() {
	_ = m.loadData()
	m.clampRow()
}

// visibleCards applies the label filter and search query to one column.
func (m *Model) visibleCards(status string) []db.Card {
	cards := m.data[status]
	out := make([]db.Card, 0, len(cards))
	for _, c := range cards {
		if m.labelFilter != "" {
			if c.Label == nil || *c.Label != m.labelFilter {
				continue
			}
		}
		if m.searchQuery != "" {
			if !db.CardMatchesSearch(c, m.searchQuery) {
				continue
			}
		}
		out = append(out, c)
	}
	return out
}

func (m *Model) clampRow() {
	cards := m.visibleCards(statuses[m.col])
	if len(cards) == 0 {
		m.row = 0
		return
	}
	if m.row >= len(cards) {
		m.row = len(cards) - 1
	}
	if m.row < 0 {
		m.row = 0
	}
}

// selectCard points the cursor at card id in a column. Columns sort by
// created_at, so a moved or new card can land anywhere. Falls back to
// clampRow if the card isn't visible (e.g. filtered out).
func (m *Model) selectCard(status string, id int64) {
	for i, c := range m.visibleCards(status) {
		if c.ID == id {
			m.row = i
			return
		}
	}
	m.clampRow()
}

func (m *Model) currentCard() *db.Card {
	cards := m.visibleCards(statuses[m.col])
	if len(cards) == 0 || m.row < 0 || m.row >= len(cards) {
		return nil
	}
	c := cards[m.row]
	return &c
}

// toast shows a banner and returns the Cmd that clears it. The sequence
// number stops an earlier toast's timer from clearing a later one.
func (m *Model) toast(kind toastKind, text string) tea.Cmd {
	m.toastSeq++
	m.toastMsg = text
	m.toastKind = kind
	return toastClearCmd(m.toastSeq)
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case tickMsg:
		return m, m.handleTick()

	case toastClearMsg:
		if msg.seq == m.toastSeq {
			m.toastMsg = ""
		}
		return m, nil

	case matrixFocusDoneMsg:
		if msg.err != nil {
			return m, m.toast(toastError, msg.err.Error())
		}
		if len(m.modalStack) > 0 {
			if _, ok := m.topModal().(*matrixModal); ok {
				m.popModal()
			}
		}
		if m.popup {
			m.quitting = true
			return m, tea.Quit
		}
		return m, nil

	case herdrOpenDoneMsg:
		return m, m.handleHerdrOpenDone(msg)

	case agentsMsg:
		if msg.disabled {
			m.agents, m.agentPanes = nil, nil
			return m, nil
		}
		var moved tea.Cmd
		if msg.err == nil {
			m.agents, m.agentPanes = msg.states, msg.panes
			moved = m.reconcileAgents(msg.states)
		}
		return m, tea.Batch(moved, pollAgentsCmd(agentPollEvery))

	case tea.KeyMsg:
		if k := msg.String(); k == "ctrl+c" || (m.popup && (k == "f15" || (m.matrixHome && k == "f20"))) {
			m.quitting = true
			return m, tea.Quit
		}
		if top := m.topModal(); top != nil {
			cmd := top.Update(msg, m)
			return m, cmd
		}
		if m.searching {
			return m, m.handleSearchKey(msg)
		}
		return m, m.handleBoardKey(msg)

	default:
		if top := m.topModal(); top != nil {
			cmd := top.Update(msg, m)
			return m, cmd
		}
		if m.searching {
			var cmd tea.Cmd
			m.searchInput, cmd = m.searchInput.Update(msg)
			return m, cmd
		}
		return m, nil
	}
}

// handleTick polls the revision once a second and reloads the board when
// something outside this process changed it (e.g. the CLI). It skips the
// reload while a modal is open and always reschedules itself, since Bubble
// Tea's Tick is one-shot. The cursor stays on the same card id across a reload.
func (m *Model) handleTick() tea.Cmd {
	next := tickCmd()
	if m.blockingModal() {
		return next
	}
	rev, err := m.store.GetRevision()
	if err != nil {
		return next
	}
	if rev == m.lastRev {
		return next
	}

	cur := m.currentCard()
	var curID int64 = -1
	if cur != nil {
		curID = cur.ID
	}
	if err := m.loadData(); err != nil {
		return next
	}
	if curID != -1 {
		for i, c := range m.visibleCards(statuses[m.col]) {
			if c.ID == curID {
				m.row = i
				break
			}
		}
	}
	m.clampRow()
	toastCmd := m.toast(toastInfo, "Board updated")
	return tea.Batch(next, toastCmd)
}
