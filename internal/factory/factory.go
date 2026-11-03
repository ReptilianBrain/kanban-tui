// Package factory builds the pieces of an unattended agent run on one card:
// the brief the agent starts from and the permission settings that keep it
// inside "work in the worktree, stop at in_review". The dispatch itself
// (worktree, session, card move) lives in internal/cli and internal/herdrsync.
package factory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// PermissionMode is the claude permission mode of a factory session: edits are
// accepted, everything else not on the allowlist stalls for a human, which the
// board's watcher surfaces as a waiting agent.
const PermissionMode = "acceptEdits"

// baseAllow lets the agent drive the board and read the repo without a human.
// Repo-specific gate commands (make check, uv run pytest, ...) come from the
// repo's entry in repos.json.
var baseAllow = []string{
	"Bash(kanban:*)",
	"Bash(git status:*)",
	"Bash(git diff:*)",
	"Bash(git log:*)",
	"Bash(git show:*)",
	"Bash(git branch --show-current)",
}

// deny is the hard stop behind the brief's "never commit, push or ship".
// A deny rule beats every allow rule and every user-level setting, so it holds
// even when the agent ignores the brief. The kanban rules are best effort: a
// prefix pattern cannot cover every spelling of "move to completed".
var deny = []string{
	"Bash(git commit:*)",
	"Bash(git push:*)",
	"Bash(git merge:*)",
	"Bash(git rebase:*)",
	"Bash(git reset:*)",
	"Bash(git stash:*)",
	"Bash(glab:*)",
	"Bash(gh:*)",
	"Skill(ship)",
	"Bash(kanban delete:*)",
	"Bash(kanban move * completed*)",
	"Bash(kanban move * blocked*)",
	"Bash(kanban update * completed*)",
	"Bash(kanban update * blocked*)",
}

type permissions struct {
	Allow []string `json:"allow"`
	Deny  []string `json:"deny"`
}

type settings struct {
	Permissions permissions `json:"permissions"`
}

// SettingsJSON returns the claude --settings document for a factory session:
// the base allowlist plus extraAllow, and the fixed deny list.
func SettingsJSON(extraAllow []string) ([]byte, error) {
	allow := append(append([]string{}, baseAllow...), extraAllow...)
	return json.MarshalIndent(settings{Permissions: permissions{
		Allow: allow,
		Deny:  append([]string{}, deny...),
	}}, "", "  ")
}

// WriteSettings writes the session's settings file into dir and returns its
// path. A file keeps the JSON out of the pane's shell quoting.
func WriteSettings(dir string, cardID int64, extraAllow []string) (string, error) {
	data, err := SettingsJSON(extraAllow)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("factory: creating %s: %w", dir, err)
	}
	path := filepath.Join(dir, fmt.Sprintf("card-%d.settings.json", cardID))
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return "", fmt.Errorf("factory: writing %s: %w", path, err)
	}
	return path, nil
}

// Brief is the first message of a factory session. Unlike the interactive
// seed prompt it forbids committing and completing, and tells the agent how to
// hand the work back.
func Brief(cardID int64, title, description, branch string) string {
	desc := ""
	if description != "" {
		desc = "\n" + description + "\n"
	}
	return fmt.Sprintf(`You're an unattended factory agent working kanban card #%[1]d: %[2]s.
%[3]s
Rules for this run:
- Work only in this worktree (branch %[4]q). The card is already "ongoing".
- Do not commit, push, open or merge MRs, or invoke the ship skill. Leave every change uncommitted in the working tree: a human reviews the diff and ships it.
- Never move the card to completed or blocked and never delete it. Your last card update is moving it to in_review.
- Development targets only. Production is read-only (REST/CLI APIs, no writes, no SSH).
- If the card is ambiguous, needs a human decision, or needs access you do not have, stop. Append a handoff stating exactly what is missing, leave the card in ongoing, and end your turn. Do not guess.

Steps:
1. Read the repo rules (AGENTS.md / CLAUDE.md) and the code the card touches, then make the smallest change that satisfies the card.
2. Run the repo's checks for what you touched and fix what fails. Report the real result; never claim a check passed that you did not run.
3. Run the code-review skill on your diff and fix the confirmed findings.
4. Append a handoff to the card with `+"`kanban update %[1]d --append \"<text>\"`"+`, in four sections: Changed (files, one line each), Verified (commands you ran and their result), Not verified, Open questions.
5. Run `+"`kanban move %[1]d in_review`"+`, then stop.`, cardID, title, desc, branch)
}
