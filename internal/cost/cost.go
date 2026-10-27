// Package cost reads the per-card spend that the Claude Code status line
// records (~/.claude/hooks/statusline.sh). Each card has one file,
// card-<id>.json, mapping a session id to that session's latest total cost in
// USD; a card's cost (its Cycle) is the sum over its sessions.
package cost

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Dir is where the cost files live. KANBANTUI_COST_DIR overrides it and is
// meant for tests.
func Dir() string {
	if d := os.Getenv("KANBANTUI_COST_DIR"); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".kanbantui", "cost")
}

// CardTotal sums every session's cost recorded for the card. ok is false when
// nothing is recorded or the file is unreadable, so callers can show "no data"
// instead of a misleading $0.00.
func CardTotal(dir string, id int64) (total float64, ok bool) {
	if dir == "" {
		return 0, false
	}
	data, err := os.ReadFile(filepath.Join(dir, fmt.Sprintf("card-%d.json", id)))
	if err != nil {
		return 0, false
	}
	var sessions map[string]float64
	if err := json.Unmarshal(data, &sessions); err != nil || len(sessions) == 0 {
		return 0, false
	}
	for _, c := range sessions {
		total += c
	}
	return total, true
}
