package tui

import (
	"hash/fnv"
	"path/filepath"
	"sort"

	"github.com/ReptilianBrain/kanban-tui/internal/db"
)

// Orphan rows: agents running in no card worktree, typically a session started
// before its card existed. They are opt-in ("o") because herdr lists every
// agent on the machine, so they would bury the cards in motion. A row has no
// card, so the selection key (card id) is a synthetic negative id derived from
// the pane, stable across polls; real card ids are always positive.

// orphanID maps a pane id to a stable negative id.
func orphanID(pane string) int64 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(pane))
	return -int64(h.Sum32()) - 1
}

// orphanRows lists the agents that belong to no card, most urgent first.
func (m *Model) orphanRows() []matrixRow {
	rows := make([]matrixRow, 0, len(m.orphans))
	for _, a := range m.orphans {
		title := a.Title
		if title == "" {
			title = filepath.Base(a.CWD)
		}
		rows = append(rows, matrixRow{
			card:  db.Card{ID: orphanID(a.PaneID), Title: title},
			agent: a.Status,
			pane:  a.PaneID,
		})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		return agentRank[rows[i].agent] < agentRank[rows[j].agent]
	})
	return rows
}
