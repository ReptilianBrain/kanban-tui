package cli

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/ReptilianBrain/kanban-tui/internal/config"
	"github.com/ReptilianBrain/kanban-tui/internal/db"
	"github.com/ReptilianBrain/kanban-tui/internal/herdrsync"
)

func TestAppendDescription(t *testing.T) {
	cases := []struct{ current, text, want string }{
		{"spec", "handoff", "spec\n\nhandoff"},
		{"spec\n\n", "  handoff \n", "spec\n\nhandoff"},
		{"", "handoff", "handoff"},
		{"spec", "  ", "spec"},
	}
	for _, c := range cases {
		if got := appendDescription(c.current, c.text); got != c.want {
			t.Errorf("appendDescription(%q, %q) = %q, want %q", c.current, c.text, got, c.want)
		}
	}
}

func cardOf(t *testing.T, path string, id int64) *db.Card {
	t.Helper()
	d, err := db.InitDB(path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	c, err := d.GetCard(id)
	if err != nil || c == nil {
		t.Fatalf("GetCard(%d) = %v, %v", id, c, err)
	}
	return c
}

func TestExecuteUpdateAppend(t *testing.T) {
	path := testDBPath(t)
	if code := Execute([]string{"add", "Spec card", "-m", "the spec"}); code != 0 {
		t.Fatalf("add: exit %d", code)
	}
	if code := Execute([]string{"update", "1", "--append", "## Handoff\nDone."}); code != 0 {
		t.Fatalf("update --append: exit %d", code)
	}
	if got := cardOf(t, path, 1).Description; got != "the spec\n\n## Handoff\nDone." {
		t.Fatalf("description = %q", got)
	}
	if code := Execute([]string{"update", "1", "-m", "x", "--append", "y"}); code != 1 {
		t.Fatalf("-m with --append: exit %d, want 1", code)
	}
}

// stubFactory replaces the three seams of `factory run` and returns the args
// the dispatcher passed to the opener.
type openCall struct {
	repoPath, base, branch, agent, label, prompt string
	opts                                         herdrsync.OpenOptions
}

func stubFactory(t *testing.T, repos []config.RepoEntry, openErr error) (*[]openCall, string) {
	t.Helper()
	dir := t.TempDir()
	calls := &[]openCall{}
	origLoad, origOpen, origDir := loadRepos, openCardWith, factoryDir
	loadRepos = func() ([]config.RepoEntry, error) { return repos, nil }
	factoryDir = func() (string, error) { return dir, nil }
	openCardWith = func(repoPath, base, branch, agent, label, prompt string, opts herdrsync.OpenOptions) (*herdrsync.OpenResult, error) {
		*calls = append(*calls, openCall{repoPath, base, branch, agent, label, prompt, opts})
		if openErr != nil {
			return nil, openErr
		}
		return &herdrsync.OpenResult{Branch: branch, PaneID: "p1", AgentName: agent}, nil
	}
	t.Cleanup(func() { loadRepos, openCardWith, factoryDir = origLoad, origOpen, origDir })
	return calls, dir
}

var oneRepo = []config.RepoEntry{{Name: "Data Platform", Path: "/repo", Base: "origin/development", FactoryAllow: []string{"Bash(make check)"}}}

func TestFactoryRunDispatches(t *testing.T) {
	path := testDBPath(t)
	calls, dir := stubFactory(t, oneRepo, nil)
	if code := Execute([]string{"add", "Name the failing tests", "-m", "the spec"}); code != 0 {
		t.Fatalf("add: exit %d", code)
	}
	if code := Execute([]string{"factory", "run", "1"}); code != 0 {
		t.Fatalf("factory run: exit %d", code)
	}
	if len(*calls) != 1 {
		t.Fatalf("opener called %d times, want 1", len(*calls))
	}
	c := (*calls)[0]
	if c.repoPath != "/repo" || c.base != "origin/development" {
		t.Errorf("repo/base = %q, %q", c.repoPath, c.base)
	}
	if c.branch != "card-1-name-the-failing-tests" || c.agent != "card-1" {
		t.Errorf("branch/agent = %q, %q", c.branch, c.agent)
	}
	if c.opts.PermissionMode != "acceptEdits" || c.opts.Focus {
		t.Errorf("opts = %+v, want acceptEdits and no focus", c.opts)
	}
	if !strings.Contains(c.prompt, "the spec") || !strings.Contains(c.prompt, "kanban move 1 in_review") {
		t.Errorf("prompt lacks the spec or the handoff steps:\n%s", c.prompt)
	}
	settings, err := os.ReadFile(c.opts.SettingsFile)
	if err != nil || !strings.Contains(string(settings), "Bash(make check)") ||
		!strings.Contains(string(settings), "Bash(git commit:*)") || !strings.HasPrefix(c.opts.SettingsFile, dir) {
		t.Errorf("settings file %q: %v\n%s", c.opts.SettingsFile, err, settings)
	}
	if got := cardOf(t, path, 1).Status; got != "ongoing" {
		t.Errorf("status = %q, want ongoing", got)
	}
}

func TestFactoryRunRollsBackWhenOpenFails(t *testing.T) {
	path := testDBPath(t)
	stubFactory(t, oneRepo, errors.New("herdr exploded"))
	Execute([]string{"add", "Card"})
	if code := Execute([]string{"factory", "run", "1"}); code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	if got := cardOf(t, path, 1).Status; got != "todo" {
		t.Errorf("status after a failed dispatch = %q, want todo", got)
	}
}

func TestFactoryRunRefusesNonRunnableCards(t *testing.T) {
	path := testDBPath(t)
	calls, _ := stubFactory(t, oneRepo, nil)
	Execute([]string{"add", "Card"})
	for _, status := range []string{"in_review", "completed", "blocked"} {
		Execute([]string{"move", "1", status})
		if code := Execute([]string{"factory", "run", "1"}); code != 1 {
			t.Errorf("run on a %s card: exit %d, want 1", status, code)
		}
	}
	if code := Execute([]string{"factory", "run", "99"}); code != 1 {
		t.Errorf("run on a missing card: exit %d, want 1", code)
	}
	if len(*calls) != 0 {
		t.Errorf("opener called %d times, want 0", len(*calls))
	}
	if got := cardOf(t, path, 1).Status; got != "blocked" {
		t.Errorf("a refused run must not move the card, status = %q", got)
	}
}

func TestPickRepo(t *testing.T) {
	two := []config.RepoEntry{{Name: "Data Platform"}, {Name: "Easy Data"}}
	cases := []struct {
		repos   []config.RepoEntry
		name    string
		want    string
		wantErr bool
	}{
		{oneRepo, "", "Data Platform", false},
		{two, "easy", "Easy Data", false},
		{two, "data platform", "Data Platform", false},
		{two, "", "", true},     // ambiguous without --repo
		{two, "data", "", true}, // substring matches both
		{two, "nope", "", true},
		{nil, "", "", true},
	}
	for _, c := range cases {
		got, err := pickRepo(c.repos, c.name)
		if (err != nil) != c.wantErr || got.Name != c.want {
			t.Errorf("pickRepo(%v, %q) = %q, %v; want %q (err=%v)", c.repos, c.name, got.Name, err, c.want, c.wantErr)
		}
	}
}
