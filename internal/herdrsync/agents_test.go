package herdrsync

import (
	"context"
	"errors"
	"os/exec"
	"reflect"
	"testing"
)

func TestCardID(t *testing.T) {
	cases := []struct {
		cwd  string
		want int64
		ok   bool
	}{
		{"/Users/x/.herdr/worktrees/kanbantui/card-543-wintermute", 543, true},
		{"/Users/x/.herdr/worktrees/kanbantui/card-543-wintermute/internal/tui", 543, true},
		{"card-7-x", 7, true},
		{"/Users/x/Documents/Tools", 0, false},
		{"/Users/x/discard-12-x", 0, false},
		{"/Users/x/card-notanumber-x", 0, false},
	}
	for _, c := range cases {
		got, ok := CardID(c.cwd)
		if got != c.want || ok != c.ok {
			t.Errorf("CardID(%q) = %d, %v; want %d, %v", c.cwd, got, ok, c.want, c.ok)
		}
	}
}

func TestListAgents(t *testing.T) {
	calls := scriptedRun(t, herdrResult{Stdout: []byte(`{"result":{"agents":[
		{"pane_id":"w1:p1","cwd":"/wt/card-5-a","agent_status":"working"},
		{"pane_id":"w1:p2","cwd":"/wt/card-5-a","agent_status":"blocked"},
		{"pane_id":"w2:p1","cwd":"/elsewhere","agent_status":"idle"},
		{"pane_id":"w3:p1","cwd":"/wt/card-9-b","agent_status":"unknown"}
	],"type":"agent_list"}}`)})
	agents, err := ListAgents(context.Background())
	if err != nil {
		t.Fatalf("ListAgents: %v", err)
	}
	if len(*calls) != 1 || !reflect.DeepEqual((*calls)[0].args, []string{"agent", "list"}) {
		t.Errorf("calls = %v, want one `agent list`", *calls)
	}
	if len(agents) != 4 {
		t.Fatalf("got %d agents, want 4", len(agents))
	}
	want := map[int64]string{5: "blocked"}
	if got := CardStates(agents); !reflect.DeepEqual(got, want) {
		t.Errorf("CardStates = %v, want %v", got, want)
	}
}

func TestListAgents_EmptyAndGarbage(t *testing.T) {
	scriptedRun(t, herdrResult{Stdout: []byte(`{"result":{"agents":[]}}`)})
	agents, err := ListAgents(context.Background())
	if err != nil || len(agents) != 0 {
		t.Errorf("empty list: got %v, %v", agents, err)
	}
	scriptedRun(t, herdrResult{Stdout: []byte(`not json`)})
	if _, err := ListAgents(context.Background()); err == nil {
		t.Error("garbage response should error")
	}
}

func TestListAgents_MissingHerdrIsConfigError(t *testing.T) {
	orig := runHerdr
	runHerdr = func(ctx context.Context, args ...string) (herdrResult, error) {
		return herdrResult{}, exec.ErrNotFound
	}
	t.Cleanup(func() { runHerdr = orig })
	_, err := ListAgents(context.Background())
	var cfg *ConfigError
	if !errors.As(err, &cfg) {
		t.Errorf("err = %v, want *ConfigError", err)
	}
}

func TestCardStates_PrefersMostUrgent(t *testing.T) {
	got := CardStates([]AgentInfo{
		{CWD: "/wt/card-1-a", Status: "idle"},
		{CWD: "/wt/card-1-a/sub", Status: "working"},
		{CWD: "/wt/card-2-b", Status: "done"},
		{CWD: "/wt/card-2-b", Status: "idle"},
	})
	want := map[int64]string{1: "working", 2: "idle"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("CardStates = %v, want %v", got, want)
	}
}

func TestCardPanes_FollowsTheMostUrgentAgent(t *testing.T) {
	got := CardPanes([]AgentInfo{
		{PaneID: "w1:p1", CWD: "/wt/card-1-a", Status: "idle"},
		{PaneID: "w1:p2", CWD: "/wt/card-1-a/sub", Status: "blocked"},
		{PaneID: "w2:p1", CWD: "/wt/card-2-b", Status: "done"},
		{PaneID: "w3:p1", CWD: "/elsewhere", Status: "working"},
		{PaneID: "w4:p1", CWD: "/wt/card-3-c", Status: "unknown"},
	})
	want := map[int64]string{1: "w1:p2", 2: "w2:p1"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("CardPanes = %v, want %v", got, want)
	}
}

func TestFocusAgent(t *testing.T) {
	calls := scriptedRun(t, okResult())
	if err := FocusAgent("w1:p2"); err != nil {
		t.Fatalf("FocusAgent: %v", err)
	}
	if len(*calls) != 1 || !reflect.DeepEqual((*calls)[0].args, []string{"agent", "focus", "w1:p2"}) {
		t.Errorf("calls = %v, want one `agent focus w1:p2`", *calls)
	}
	scriptedRun(t, herdrResult{ExitCode: 1, Stdout: []byte(`{"error":{"code":"agent_not_found","message":"agent target w1:p9 not found"}}`)})
	if err := FocusAgent("w1:p9"); err == nil {
		t.Error("a herdr failure should surface as an error")
	}
}
