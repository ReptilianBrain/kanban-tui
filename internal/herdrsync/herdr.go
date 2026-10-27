// Package herdrsync shells out to the `herdr` CLI (https://herdr.dev) to open
// one kanban card's work in an isolated git-worktree pane running a fresh
// Claude Code session.
//
// herdr is a JSON-over-CLI client to a running herdr server. Successful
// mutating subcommands return {"result": {...}} and ids are read from that
// JSON, never assumed. Failures are {"error": {"code": ..., "message": ...}}
// with exit code 1, but on no fixed stream; see herdrFailure.
package herdrsync

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

const (
	worktreeTimeout = 30 * time.Second
	agentTimeout    = 35 * time.Second // just above herdr's own 30s agent-start timeout
	promptTimeout   = 15 * time.Second
)

// ConfigError means the herdr CLI itself is unusable (not installed), an
// environment problem rather than a per-card failure.
type ConfigError struct{ Message string }

func (e *ConfigError) Error() string { return e.Message }

type OpenResult struct {
	Branch    string
	PaneID    string
	AgentName string
	// Reused is true when OpenCard reconnected to a worktree/agent left
	// over from an earlier call instead of creating a fresh one.
	Reused bool
}

// herdrResult is one herdr invocation's outcome. ExitCode is meaningful only
// when the runner's error is nil; a non-nil error means herdr could not be
// run at all.
type herdrResult struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}

// runHerdr is a seam over process execution so tests can simulate herdr's
// stdout, stderr and exit code without a running server.
var runHerdr = defaultRunHerdr

func defaultRunHerdr(ctx context.Context, args ...string) (herdrResult, error) {
	cmd := exec.CommandContext(ctx, "herdr", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return herdrResult{Stdout: stdout.Bytes(), Stderr: stderr.Bytes(), ExitCode: exitErr.ExitCode()}, nil
		}
		return herdrResult{}, err
	}
	return herdrResult{Stdout: stdout.Bytes(), Stderr: stderr.Bytes(), ExitCode: 0}, nil
}

// run executes one herdr subcommand and returns the `.result` object of its
// JSON response. step prefixes any error so a mid-sequence failure says
// which step it was.
func run(ctx context.Context, step string, args ...string) (map[string]any, error) {
	res, err := runHerdr(ctx, args...)
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, &ConfigError{Message: "herdr CLI not found — install it or check PATH"}
		}
		return nil, fmt.Errorf("%s: %w", step, err)
	}
	if res.ExitCode != 0 {
		_, msg := herdrFailure(res)
		return nil, fmt.Errorf("%s: %s", step, msg)
	}

	var envelope struct {
		Result map[string]any `json:"result"`
	}
	if err := json.Unmarshal(res.Stdout, &envelope); err != nil {
		return nil, fmt.Errorf("%s: unparseable response: %s", step, strings.TrimSpace(string(res.Stdout)))
	}
	return envelope.Result, nil
}

// herdrFailure extracts herdr's error code and message from a non-zero
// result. The {"error": ...} envelope is not on a consistent stream (`agent
// get` on a missing agent writes it to stderr, other subcommands to stdout),
// so both are tried, then raw stderr, raw stdout, and a generic message.
func herdrFailure(res herdrResult) (code, message string) {
	if code, msg, ok := parseHerdrErrorEnvelope(res.Stdout); ok {
		return code, msg
	}
	if code, msg, ok := parseHerdrErrorEnvelope(res.Stderr); ok {
		return code, msg
	}
	msg := strings.TrimSpace(string(res.Stderr))
	if msg == "" {
		msg = strings.TrimSpace(string(res.Stdout))
	}
	if msg == "" {
		msg = fmt.Sprintf("herdr exited %d", res.ExitCode)
	}
	return "", msg
}

// parseHerdrErrorEnvelope parses herdr's error envelope from one stream.
func parseHerdrErrorEnvelope(data []byte) (code, message string, ok bool) {
	var envelope struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(data, &envelope); err == nil && envelope.Error.Message != "" {
		return envelope.Error.Code, envelope.Error.Message, true
	}
	return "", "", false
}

// existingAgentPane returns the pane id of a live agent with this name, or ""
// when there is none (agent_not_found is not an error). The name is derived
// from the card id, so this is what lets reopening a card reconnect instead
// of colliding with its own worktree.
func existingAgentPane(ctx context.Context, agentName string) (string, error) {
	res, err := runHerdr(ctx, "agent", "get", agentName)
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return "", &ConfigError{Message: "herdr CLI not found — install it or check PATH"}
		}
		return "", fmt.Errorf("checking for existing agent: %w", err)
	}
	if res.ExitCode != 0 {
		if code, msg := herdrFailure(res); code != "agent_not_found" {
			return "", fmt.Errorf("checking for existing agent: %s", msg)
		}
		return "", nil
	}

	var envelope struct {
		Result struct {
			Agent struct {
				PaneID string `json:"pane_id"`
			} `json:"agent"`
		} `json:"result"`
	}
	if err := json.Unmarshal(res.Stdout, &envelope); err != nil {
		return "", fmt.Errorf("checking for existing agent: unparseable response: %s", strings.TrimSpace(string(res.Stdout)))
	}
	return envelope.Result.Agent.PaneID, nil
}

// paneID extracts the pane id from a worktree create/open result, shaped
// {"root_pane": {"pane_id": "..."}}.
func paneID(result map[string]any) (string, error) {
	rootPane, _ := result["root_pane"].(map[string]any)
	if rootPane == nil {
		return "", errors.New("no root_pane in response")
	}
	id, _ := rootPane["pane_id"].(string)
	if id == "" {
		return "", errors.New("no pane_id in response")
	}
	return id, nil
}

var slugRE = regexp.MustCompile(`[^a-z0-9]+`)

// slugify lowercases title into a branch-safe slug of at most 40 characters.
func slugify(title string) string {
	s := slugRE.ReplaceAllString(strings.ToLower(title), "-")
	s = strings.Trim(s, "-")
	if len(s) > 40 {
		s = strings.Trim(s[:40], "-")
	}
	if s == "" {
		s = "card"
	}
	return s
}

// BranchName returns "card-<id>-<slug>"; the id prefix keeps it unique per
// card even when titles collide.
func BranchName(cardID int64, title string) string {
	slug := slugify(title)
	return fmt.Sprintf("card-%d-%s", cardID, slug)
}

// SeedPrompt builds the first message for the new Claude session: the card's
// context and the state the board has already put it in.
func SeedPrompt(cardID int64, title, description, branch string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "You're picking up kanban card #%d: %s.\n", cardID, title)
	if description != "" {
		fmt.Fprintf(&b, "\n%s\n", description)
	}
	fmt.Fprintf(&b, "\nYou're already on branch %q with the card moved to \"ongoing\". "+
		"Work normally and keep the card's status up to date with the `kanban` CLI.", branch)
	return b.String()
}

// OpenCard creates a worktree-backed pane for repoPath on branch, starts
// Claude Code in it under agentName, and seeds it with prompt. label names
// both the herdr sidebar entry and the Claude session.
//
// branch and agentName are derived from the card, so a second call for the
// same card would collide with its own worktree and agent. Two checks make
// it reentrant instead:
//   - an agent already alive under agentName is focused and reused as-is,
//     skipping worktree/start/prompt entirely;
//   - failing that, a worktree/branch that already exists on disk (no live
//     agent, e.g. a previous call died between the worktree and agent-start
//     steps) is reopened via `worktree open` rather than recreated. herdr
//     reports a pre-existing worktree under the generic worktree_create_failed
//     code, so that case is detected by matching "already exists" in the
//     message.
//
// base, when non-empty, is the ref a freshly created worktree branches from
// (`--base`); empty leaves herdr's default. It has no effect on a reused
// agent or a reopened worktree, which already have their branch.
func OpenCard(repoPath, base, branch, agentName, label, prompt string) (*OpenResult, error) {
	checkCtx, checkCancel := context.WithTimeout(context.Background(), worktreeTimeout)
	pane, err := existingAgentPane(checkCtx, agentName)
	checkCancel()
	if err != nil {
		return nil, err
	}
	if pane != "" {
		if err := FocusAgent(agentName); err != nil {
			return nil, err
		}
		return &OpenResult{Branch: branch, PaneID: pane, AgentName: agentName, Reused: true}, nil
	}

	wtCtx, wtCancel := context.WithTimeout(context.Background(), worktreeTimeout)
	createArgs := []string{"worktree", "create", "--cwd", repoPath, "--branch", branch}
	if base != "" {
		createArgs = append(createArgs, "--base", base)
	}
	createArgs = append(createArgs, "--label", label, "--focus")
	wtResult, err := run(wtCtx, "creating worktree", createArgs...)
	wtCancel()
	if err != nil {
		if !strings.Contains(err.Error(), "already exists") {
			return nil, err
		}
		openCtx, openCancel := context.WithTimeout(context.Background(), worktreeTimeout)
		wtResult, err = run(openCtx, "opening existing worktree", "worktree", "open",
			"--cwd", repoPath, "--branch", branch, "--focus")
		openCancel()
		if err != nil {
			return nil, err
		}
	}
	wtPane, err := paneID(wtResult)
	if err != nil {
		return nil, fmt.Errorf("creating worktree: %w", err)
	}

	startCtx, startCancel := context.WithTimeout(context.Background(), agentTimeout)
	defer startCancel()
	if _, err := run(startCtx, "starting Claude", "agent", "start", agentName,
		"--kind", "claude", "--pane", wtPane, "--", "--permission-mode", "plan", "--name", label); err != nil {
		return nil, err
	}

	promptCtx, promptCancel := context.WithTimeout(context.Background(), promptTimeout)
	defer promptCancel()
	if _, err := run(promptCtx, "sending prompt", "agent", "prompt", agentName, prompt); err != nil {
		return nil, err
	}

	return &OpenResult{Branch: branch, PaneID: wtPane, AgentName: agentName}, nil
}
