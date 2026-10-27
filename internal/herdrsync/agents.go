package herdrsync

import (
	"context"
	"regexp"
	"strconv"
	"time"
)

const listTimeout = 5 * time.Second

// AgentInfo is one running agent from `herdr agent list`. Status is herdr's
// own word: idle, working, blocked, done or unknown.
type AgentInfo struct {
	PaneID string
	CWD    string
	Status string
}

// ListAgents returns every agent herdr knows about. A missing herdr binary
// comes back as a *ConfigError so callers can stop polling instead of
// retrying forever.
func ListAgents(ctx context.Context) ([]AgentInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, listTimeout)
	defer cancel()
	result, err := run(ctx, "listing agents", "agent", "list")
	if err != nil {
		return nil, err
	}
	raw, _ := result["agents"].([]any)
	agents := make([]AgentInfo, 0, len(raw))
	for _, item := range raw {
		entry, _ := item.(map[string]any)
		if entry == nil {
			continue
		}
		pane, _ := entry["pane_id"].(string)
		cwd, _ := entry["cwd"].(string)
		status, _ := entry["agent_status"].(string)
		agents = append(agents, AgentInfo{PaneID: pane, CWD: cwd, Status: status})
	}
	return agents, nil
}

var cardPathRE = regexp.MustCompile(`(?:^|/)card-(\d+)-`)

// CardID reads the card id out of a worktree path such as
// ~/.herdr/worktrees/repo/card-543-slug (or any directory below it).
func CardID(cwd string) (int64, bool) {
	m := cardPathRE.FindStringSubmatch(cwd)
	if m == nil {
		return 0, false
	}
	id, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil {
		return 0, false
	}
	return id, true
}

// statusRank orders what a card should show when several agents work in its
// worktree: a blocked one needs attention before anything else.
var statusRank = map[string]int{"blocked": 4, "working": 3, "idle": 2, "done": 1}

// CardStates maps card id to the most urgent agent status among the agents
// whose cwd sits in that card's worktree. Agents outside card worktrees and
// agents with an unknown status are left out.
func CardStates(agents []AgentInfo) map[int64]string {
	states := map[int64]string{}
	for _, a := range agents {
		id, ok := CardID(a.CWD)
		if !ok || statusRank[a.Status] == 0 {
			continue
		}
		if statusRank[a.Status] > statusRank[states[id]] {
			states[id] = a.Status
		}
	}
	return states
}
