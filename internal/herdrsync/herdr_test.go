package herdrsync

import (
	"context"
	"strings"
	"testing"
)

type call struct {
	args []string
}

// scriptedRun replays one herdrResult per herdr call, in order, and records
// the args of each call. An unscripted extra call fails the test.
func scriptedRun(t *testing.T, results ...herdrResult) *[]call {
	t.Helper()
	calls := &[]call{}
	i := 0
	orig := runHerdr
	runHerdr = func(ctx context.Context, args ...string) (herdrResult, error) {
		*calls = append(*calls, call{args: args})
		if i >= len(results) {
			t.Fatalf("unexpected extra herdr call: %v", args)
		}
		res := results[i]
		i++
		return res, nil
	}
	t.Cleanup(func() { runHerdr = orig })
	return calls
}

func agentGetOK(pane string) herdrResult {
	return herdrResult{ExitCode: 0, Stdout: []byte(`{"result":{"agent":{"pane_id":"` + pane + `"}}}`)}
}

func agentNotFound() herdrResult {
	return herdrResult{ExitCode: 1, Stdout: []byte(`{"error":{"code":"agent_not_found","message":"agent target x not found"}}`)}
}

// agentNotFoundOnStderr is what a real `herdr agent get` on a missing agent
// returns (verified against herdr 0.8.2): the envelope on stderr, empty
// stdout.
func agentNotFoundOnStderr() herdrResult {
	return herdrResult{ExitCode: 1, Stderr: []byte(`{"error":{"code":"agent_not_found","message":"agent target card-254 not found"}}`)}
}

func worktreeCreateOK(pane string) herdrResult {
	return herdrResult{ExitCode: 0, Stdout: []byte(`{"result":{"root_pane":{"pane_id":"` + pane + `"}}}`)}
}

func worktreeAlreadyExists() herdrResult {
	return herdrResult{ExitCode: 1, Stdout: []byte(`{"error":{"code":"worktree_create_failed","message":"fatal: '/path' already exists"}}`)}
}

func okResult() herdrResult {
	return herdrResult{ExitCode: 0, Stdout: []byte(`{"result":{}}`)}
}

func TestOpenCard_FreshWorktree(t *testing.T) {
	calls := scriptedRun(t,
		agentNotFound(),        // agent get
		worktreeCreateOK("p1"), // worktree create
		okResult(),             // agent start
		okResult(),             // agent prompt
	)

	res, err := OpenCard("/repo", "", "card-1-foo", "card-1", "#1 Foo", "prompt")
	if err != nil {
		t.Fatalf("OpenCard() error = %v", err)
	}
	if res.Reused {
		t.Errorf("Reused = true, want false for a fresh worktree")
	}
	if res.PaneID != "p1" {
		t.Errorf("PaneID = %q, want p1", res.PaneID)
	}

	if len(*calls) != 4 {
		t.Fatalf("got %d herdr calls, want 4: %+v", len(*calls), *calls)
	}
	if (*calls)[1].args[0] != "worktree" || (*calls)[1].args[1] != "create" {
		t.Errorf("call[1] = %v, want worktree create", (*calls)[1].args)
	}
	if got := strings.Join((*calls)[1].args, " "); !strings.Contains(got, "--label #1 Foo") {
		t.Errorf("worktree create args = %q, want --label #1 Foo", got)
	}
	if got := strings.Join((*calls)[2].args, " "); !strings.HasSuffix(got, "--name #1 Foo") {
		t.Errorf("agent start args = %q, want trailing --name #1 Foo", got)
	}
}

func TestOpenCard_BaseFlag(t *testing.T) {
	tests := []struct {
		name     string
		base     string
		wantBase bool
	}{
		{"base set", "development", true},
		{"base empty", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			calls := scriptedRun(t,
				agentNotFound(),
				worktreeCreateOK("p1"),
				okResult(),
				okResult(),
			)
			if _, err := OpenCard("/repo", tc.base, "card-1-foo", "card-1", "Foo", "prompt"); err != nil {
				t.Fatalf("OpenCard() error = %v", err)
			}
			got := strings.Join((*calls)[1].args, " ")
			if tc.wantBase && !strings.Contains(got, "--base "+tc.base) {
				t.Errorf("worktree create args = %q, want --base %s", got, tc.base)
			}
			if !tc.wantBase && strings.Contains(got, "--base") {
				t.Errorf("worktree create args = %q, want no --base", got)
			}
		})
	}
}

func TestBranchName(t *testing.T) {
	if got, want := BranchName(7, "Fix login"), "card-7-fix-login"; got != want {
		t.Errorf("BranchName() = %q, want %q", got, want)
	}
}

func TestOpenCard_ReusesLiveAgent(t *testing.T) {
	calls := scriptedRun(t,
		agentGetOK("p9"), // agent get: already alive
		okResult(),       // agent focus
	)

	res, err := OpenCard("/repo", "", "card-1-foo", "card-1", "Foo", "prompt")
	if err != nil {
		t.Fatalf("OpenCard() error = %v", err)
	}
	if !res.Reused {
		t.Errorf("Reused = false, want true when an agent is already live")
	}
	if res.PaneID != "p9" {
		t.Errorf("PaneID = %q, want p9", res.PaneID)
	}

	if len(*calls) != 2 {
		t.Fatalf("got %d herdr calls, want 2 (get + focus, no worktree/start/prompt): %+v", len(*calls), *calls)
	}
}

func TestOpenCard_ReopensExistingWorktreeOnConflict(t *testing.T) {
	calls := scriptedRun(t,
		agentNotFound(),         // agent get: no live agent
		worktreeAlreadyExists(), // worktree create: collides with a stale checkout
		worktreeCreateOK("p2"),  // worktree open: reconnects
		okResult(),              // agent start
		okResult(),              // agent prompt
	)

	res, err := OpenCard("/repo", "", "card-1-foo", "card-1", "Foo", "prompt")
	if err != nil {
		t.Fatalf("OpenCard() error = %v", err)
	}
	if res.Reused {
		t.Errorf("Reused = true, want false — a fresh agent still gets started in the reopened worktree")
	}
	if res.PaneID != "p2" {
		t.Errorf("PaneID = %q, want p2", res.PaneID)
	}

	if len(*calls) != 5 {
		t.Fatalf("got %d herdr calls, want 5: %+v", len(*calls), *calls)
	}
	if (*calls)[2].args[1] != "open" {
		t.Errorf("call[2] = %v, want worktree open fallback", (*calls)[2].args)
	}
}

func TestOpenCard_AgentNotFoundOnStderrIsNotAnError(t *testing.T) {
	calls := scriptedRun(t,
		agentNotFoundOnStderr(), // agent get: no live agent, reported on stderr
		worktreeCreateOK("p1"),  // worktree create
		okResult(),              // agent start
		okResult(),              // agent prompt
	)

	res, err := OpenCard("/repo", "", "card-254-foo", "card-254", "Foo", "prompt")
	if err != nil {
		t.Fatalf("OpenCard() error = %v, want agent_not_found on stderr treated as no existing agent", err)
	}
	if res.Reused {
		t.Errorf("Reused = true, want false for a fresh worktree")
	}
	if len(*calls) != 4 {
		t.Fatalf("got %d herdr calls, want 4: %+v", len(*calls), *calls)
	}
}

func TestOpenCard_WorktreeCreateOtherFailurePropagates(t *testing.T) {
	scriptedRun(t,
		agentNotFound(),
		herdrResult{ExitCode: 1, Stdout: []byte(`{"error":{"code":"worktree_create_failed","message":"fatal: not a git repository"}}`)},
	)

	_, err := OpenCard("/repo", "", "card-1-foo", "card-1", "Foo", "prompt")
	if err == nil {
		t.Fatal("OpenCard() error = nil, want an error for a non-conflict failure")
	}
	if !strings.Contains(err.Error(), "not a git repository") {
		t.Errorf("error = %v, want herdr's own message surfaced, not a raw JSON blob", err)
	}
}

func TestSeedPrompt(t *testing.T) {
	got := SeedPrompt(7, "Title", "Desc", "", "card-7-title")
	if !strings.Contains(got, "\nDesc\n") {
		t.Errorf("prompt should include the description: %q", got)
	}
	if !strings.Contains(got, "card-7-title") {
		t.Errorf("prompt should name the branch: %q", got)
	}
	if strings.Contains(got, "triage") {
		t.Errorf("a plain card should not get the triage brief: %q", got)
	}
	if noDesc := SeedPrompt(7, "Title", "", "", "card-7-title"); strings.Contains(noDesc, "\n\n\n") {
		t.Errorf("empty description should not leave a blank block: %q", noDesc)
	}
}

func TestSeedPromptTriage(t *testing.T) {
	got := SeedPrompt(7, "Run failed", "See run abc", TriageLabel, "card-7-run-failed")
	for _, want := range []string{"triage mode", "triage-failure", "card-7 in --refs", "See run abc", "card-7-run-failed"} {
		if !strings.Contains(got, want) {
			t.Errorf("incident prompt missing %q: %q", want, got)
		}
	}
	if capped := SeedPrompt(7, "T", "", "Incident", "b"); !strings.Contains(capped, "triage mode") {
		t.Errorf("label match should ignore case: %q", capped)
	}
	if other := SeedPrompt(7, "T", "", "server", "b"); strings.Contains(other, "triage mode") {
		t.Errorf("only the incident label should trigger the brief: %q", other)
	}
}
