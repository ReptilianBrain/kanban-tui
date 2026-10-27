# kanban-tui

Terminal kanban board in Go: [Bubble Tea](https://github.com/charmbracelet/bubbletea) TUI and a Cobra CLI over one SQLite database (`modernc.org/sqlite`, pure Go).

## Commands

```bash
make build     # go build -o kanban ./cmd/kanban
make run       # build, then launch the TUI
make test      # go test ./...
make vet       # go vet ./...
make fmt       # gofmt -l -w .
make backup    # sqlite3 .backup of the live DB into ~/.kanbantui/backups/
```

`./kanban` with no arguments launches the TUI; with arguments it runs the CLI (`kanban --help`). Run `make test` and `make vet` before finishing a change.

## Layout

| Path | Role |
|---|---|
| `cmd/kanban` | Entry point: no args = `tui.Run()`, args = `cli.Execute()` and exit with its code. |
| `internal/db` | SQLite layer, schema and idempotent `migrate()`, card/label/note CRUD, fuzzy search. |
| `internal/cli` | Cobra commands, one file per group. Imports `db` only, never `tui`. |
| `internal/tui` | Bubble Tea model, board rendering, modals, Nord Polar Night styles. |
| `internal/herdrsync` | Shells out to the optional [herdr](https://herdr.dev) CLI (`H` key). |
| `internal/cost` | Reads the per-card spend files the Claude Code status line writes (`~/.kanbantui/cost/card-<id>.json`, `KANBANTUI_COST_DIR` overrides). |
| `internal/config` | `~/.kanbantui/repos.json` loader for the herdr repo picker. |

The DB is `~/.kanbantui/kanban.db`; `KANBANTUI_DB_PATH` overrides it and is meant for tests. Tests must never touch the real DB: use `t.TempDir()` (the TUI tests build a synthetic board in `testStore`).

## Behaviors worth knowing

- **Statuses vs labels.** Status values are `todo`, `ongoing`, `blocked`, `in_review`, `completed`. The UI shows them as Backlog, In Progress, Blocked, In Review, Completed under `KANBANTUI_SKIN=plain`, and as Fixer, Jacked In, Flatline, Dead Drop, Jacked Out under the default `sprawl` skin (`internal/tui/lexicon.go`). The skin renames only what the TUI displays; the CLI, JSON and DB keep the plain values. Use the value in the CLI and DB, the label in the UI.
- **Agent state and the watcher.** The TUI polls `herdr agent list` every 3s (`herdrsync.ListAgents`) and matches agents to cards by a `card-<id>-` path segment in their cwd; the card shows working / waiting / idle / done. Polling stops for good if the herdr binary is missing. `internal/tui/watcher.go` moves a card from `ongoing` to `blocked` once its agent has waited for input for 90s, and back to `ongoing` when the agent resumes, but only for cards the watcher itself blocked. It never touches `todo`, `in_review` or `completed`, and does nothing while a modal is open. It runs only while the TUI is open.
- **Matrix and Cycle.** `m` opens the Matrix, a read-only live monitor of cards in flight (In Progress or Blocked, or with an agent): agent state, a Flatline countdown for a waiting agent, and the card's Cycle. Unlike other modals it does not pause the DB poll or the watcher (`blockingModal`). Cycle is the sum of every session's last-known cost for the card, recorded by `~/.claude/hooks/statusline.sh` from Claude Code's status JSON; it also shows in the card detail. No file means no amount, never `$0.00`. Tests set `m.costDir` to a temp dir.
- **Live refresh.** The TUI polls `db.GetRevision()` every second and reloads when it changes, so CLI edits show up within about a second. The revision is `(card count, max updated_at, label count, label names)`; notes are deliberately excluded. The TUI skips the reload while a modal is open and keeps the cursor on the same card id. The DB runs in WAL mode with a 5s busy timeout for multi-process access.
- **Column order.** Every column except Completed is sorted by `created_at` descending, newest on top. Completed sorts by completion time. The stored `position` column no longer drives display, so `Shift+Up/Down` only toast. `list` and `export` still use position order.
- **Aging.** `status_changed_at` is set only on create and on a real status change; `UpdateCardStatus` is a no-op when the status is unchanged, so a repeated `move` cannot reset the clock. Rows with NULL fall back to `created_at`. Colors: 3-6 days dim, 7-13 yellow, 14+ bold red; not shown for Backlog or Completed.
- **WIP limits.** `wipLimits` in `internal/tui/wip.go` (In Progress 6, Blocked 2) drive the lane header counts and turn them red once exceeded. Other lanes except Backlog and Completed turn red above 5 cards.
- **Labels.** One label per card, stored by name. Renaming a label rewrites the cards that use it; deleting sets their label to NULL. The label filter is an exact match and combines with the `/` search.
- **Search.** `db.FuzzyScore` tries a substring match first, then a bounded in-order subsequence match. `CardMatchesSearch` also matches a plain substring of the card id.
- **`kanban next`.** Ranks `todo` and `ongoing` cards by priority, then oldest. No due-date weighting, on purpose.
- **Duplicate nudge.** `kanban add` warns on stderr when the title is a substring (either way, 4+ characters) of a non-completed card's title.
- **Quick notes.** A separate `notes` table for scratch ideas. They never appear on the board and are excluded from the revision, so a CLI `note` does not trigger a reload toast. `p` in the notes modal promotes a note into a pre-filled card.
- **Migrations.** `migrate()` creates the schema and adds missing columns; it also drops legacy `gitlab_*` columns and folds the old `planned` status into `todo`. It runs on every start and must stay idempotent.

## Theme

Nord Polar Night, dark only, defined in `internal/tui/styles.go`.

- `View()` paints nord0 under every cell (`paintCanvas`) so the look does not depend on the terminal theme. A Lip Gloss `Background()` is cancelled by the `\x1b[0m` that each inner `Render` ends with, so `paintBg` re-selects the background after every reset. Only the outermost frame pads to width; nested surfaces (card content, modal bodies) pass width 0 or they wrap.
- Colors go through `nord(trueColor, ansi256, ansi)` with explicit fallbacks, because automatic downsampling turns nord0 into teal on 256-color terminals.
- Selection is a Frost left-edge bar, never a background fill. Muted text is strict nord3 and is faint by choice.
- `canvas_test.go` forces the TrueColor profile; without a TTY the escape-sequence assertions would pass vacuously.

## Conventions

- Match the surrounding code: comments only where the why is not obvious, no emojis, gofmt.
- CLI exit codes: 0 success, 1 business error (missing card, bad date), 2 usage error. Classification is by error type (`businessError`), not a mutable flag.
- TUI tests send key presses through `Update` and do not execute the returned `Cmd`s; state changes happen synchronously, and running the timer Cmds would block on real ticks.
