# kanban-tui

A terminal kanban board written in Go ([Bubble Tea](https://github.com/charmbracelet/bubbletea) + SQLite),
with a scriptable CLI that shares the same database. Nord Polar Night theme.

Columns: Backlog, In Progress, Blocked, In Review, Completed.

## Build and run

```bash
go install github.com/ReptilianBrain/kanban-tui/cmd/kanban@latest   # or build from a clone:
make build        # go build -o kanban ./cmd/kanban
./kanban          # no arguments: launch the TUI
./kanban --help   # with arguments: CLI
```

```bash
kanban add "Fix login bug" -p high --label backend --due 2026-07-01 --jira PRJ-123
kanban list --json
kanban move 12 in_review
kanban next       # suggest the best card to pick up
```

The board lives in `~/.kanbantui/kanban.db`. CLI edits show up in a running TUI within about a second.

Optional: `H` on a card opens it in a [herdr](https://herdr.dev) worktree. List your repos in
`~/.kanbantui/repos.json`, e.g. `[{"name": "my-repo", "path": "/path/to/my-repo", "base": "main"}]`
(`base` is optional).

## Keys

| Key | Action |
|---|---|
| `n` / `e` / `d` | New, edit, delete card |
| `Enter` | Card details (`H` there also opens it in herdr) |
| `h` `j` `k` `l` or arrows | Move between columns and cards |
| `[` `]` or `Shift+Left/Right` | Move the card to the previous or next column |
| `/` | Fuzzy search, live as you type (`Esc` clears) |
| `f` | Filter by label (`Esc` clears) |
| `t` | Label manager |
| `o` | Quick notes (`p` promotes a note into a card) |
| `c` | Show or hide Backlog and Completed |
| `H` | Open the card in a herdr worktree and move it to In Progress |
| `r` | Refresh (the board also refreshes itself every second) |
| `Ctrl+S` | Save inside the card form |
| `q` | Quit |

Architecture and conventions for contributors and coding agents are in `AGENTS.md`.

## License

MIT, see `LICENSE`.
