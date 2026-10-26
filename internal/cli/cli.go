// Package cli is the Cobra command-line interface for KanbanTUI. It reads and
// mutates the same SQLite database as the TUI and never imports it.
//
// Exit codes:
//   - A business-logic failure (missing card, bad due date, ...) is reported
//     as "Error: <message>" on stderr with exit code 1; see businessError.
//   - Anything else (bad flag value, wrong arg count/type) is a plain
//     unwrapped error: Cobra prints its message and usage, and Execute maps
//     it to exit code 2.
package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"kanbantui/internal/db"
)

// allStatuses is the board's column order; keep in sync with the TUI.
var allStatuses = []string{"todo", "ongoing", "blocked", "in_review", "completed"}

// priorities is priority rank order, lowest to highest.
var priorities = []string{"low", "medium", "high", "critical"}

// statusLabels maps a stored status value to its display label. The values
// ("todo", "ongoing") are what the DB and flags use; only the labels differ.
var statusLabels = map[string]string{
	"todo":      "Backlog",
	"ongoing":   "In Progress",
	"blocked":   "Blocked",
	"in_review": "In Review",
	"completed": "Completed",
}

// businessError is a failure reported as "Error: <msg>" with exit code 1.
// Cobra's default error printer already adds the "Error: " prefix, so RunE
// only needs to return one of these with SilenceUsage set (see fail).
type businessError struct{ msg string }

func (e *businessError) Error() string { return e.msg }

func businessErrorf(format string, a ...any) error {
	return &businessError{msg: fmt.Sprintf(format, a...)}
}

// fail suppresses the usage block and returns a businessError.
func fail(cmd *cobra.Command, format string, a ...any) error {
	cmd.SilenceUsage = true
	return businessErrorf(format, a...)
}

// Execute runs the CLI with args (typically os.Args[1:]) and returns the
// process exit code. Classification is by error type rather than a mutable
// Cobra field, so a Silence* flag set for output reasons can't change it.
func Execute(args []string) int {
	root := newRootCmd()
	root.SetArgs(args)
	err := root.Execute()
	if err == nil {
		return 0
	}
	var be *businessError
	if errors.As(err, &be) {
		return 1
	}
	return 2
}

// appCtx carries the open database handle to every subcommand. It is set by
// the root's PersistentPreRunE; no leaf command may define its own, or the
// handle would never be populated.
type appCtx struct {
	db *db.DB
}

func newRootCmd() *cobra.Command {
	ctx := &appCtx{}
	root := &cobra.Command{
		Use:   "kanban",
		Short: "KanbanTUI CLI — read and mutate the board.",
		Long: "KanbanTUI CLI — read and mutate the board.\n\n" +
			"The board is the shared SQLite DB at ~/.kanbantui/kanban.db, so changes\n" +
			"made here appear live in a running TUI within ~1 second.",
		SilenceUsage:  false,
		SilenceErrors: false,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			d, err := db.Open()
			if err != nil {
				return fail(cmd, "%s", err.Error())
			}
			ctx.db = d
			return nil
		},
	}
	root.CompletionOptions.DisableDefaultCmd = true

	root.AddCommand(
		newAddCmd(ctx),
		newUpdateCmd(ctx),
		newMoveCmd(ctx),
		newDeleteCmd(ctx),
		newShowCmd(ctx),
		newListCmd(ctx),
		newNextCmd(ctx),
		newExportCmd(ctx),
		newLabelsCmd(ctx),
		newNoteCmd(ctx),
		newNotesCmd(ctx),
		newNoteDeleteCmd(ctx),
	)
	return root
}

// parseIntArg converts a positional argument to an id. A failure is a plain
// (unwrapped) error so it maps to exit code 2, like any other usage error.
func parseIntArg(kind, s string) (int64, error) {
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid %s %q", kind, s)
	}
	return n, nil
}

func parseCardID(s string) (int64, error) {
	return parseIntArg("card id", s)
}

func parseNoteID(s string) (int64, error) {
	return parseIntArg("note id", s)
}

// printJSON writes v as 2-space-indented JSON with HTML escaping disabled, so
// "<", ">" and "&" in card text stay readable.
func printJSON(w interface{ Write([]byte) (int, error) }, v any) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return err
	}
	_, err := w.Write(buf.Bytes())
	return err
}

// emit prints v as JSON when asJSON is set, otherwise the plain message.
func emit(cmd *cobra.Command, v any, asJSON bool, message string) error {
	if asJSON {
		return printJSON(cmd.OutOrStdout(), v)
	}
	fmt.Fprintln(cmd.OutOrStdout(), message)
	return nil
}

// choiceValue is a pflag.Value that only accepts one of a fixed set of
// strings. An invalid value returns a plain error, which Cobra reports as a
// usage error (exit code 2).
type choiceValue struct {
	value   *string
	allowed []string
}

func newChoiceFlag(dest *string, allowed []string, def string) *choiceValue {
	*dest = def
	return &choiceValue{value: dest, allowed: allowed}
}

func (c *choiceValue) String() string { return *c.value }
func (c *choiceValue) Type() string   { return "string" }
func (c *choiceValue) Set(s string) error {
	for _, a := range c.allowed {
		if a == s {
			*c.value = s
			return nil
		}
	}
	return fmt.Errorf("invalid value %q: must be one of %s", s, strings.Join(c.allowed, ", "))
}

func isValidChoice(s string, allowed []string) bool {
	for _, a := range allowed {
		if a == s {
			return true
		}
	}
	return false
}

// isoNow formats the current local time the way internal/db's nowStamp does:
// no timezone suffix, and no fractional part when it would be all zero.
func isoNow() string {
	now := time.Now()
	if now.Nanosecond() != 0 {
		return now.Format("2006-01-02T15:04:05.000000")
	}
	return now.Format("2006-01-02T15:04:05")
}

// parseNaiveDateTime parses a local, timezone-less timestamp as stored by the
// DB, with or without fractional seconds.
func parseNaiveDateTime(ts string) (time.Time, bool) {
	for _, layout := range []string{"2006-01-02T15:04:05.999999999", "2006-01-02T15:04:05"} {
		if t, err := time.ParseInLocation(layout, ts, time.Local); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// validateDue rejects a non-empty due date that isn't YYYY-MM-DD. The empty
// string means "clear" and is always valid.
func validateDue(cmd *cobra.Command, due string) error {
	if due == "" {
		return nil
	}
	if _, err := time.ParseInLocation("2006-01-02", due, time.Local); err != nil {
		return fail(cmd, "due date '%s' must be in YYYY-MM-DD format", due)
	}
	return nil
}

// dueStatus buckets a due date as overdue, today, soon (within 3 days),
// future, or "" when unset or unparseable.
func dueStatus(due *string) string {
	if due == nil || *due == "" {
		return ""
	}
	dueDate, err := time.ParseInLocation("2006-01-02", *due, time.Local)
	if err != nil {
		return ""
	}
	y, m, d := time.Now().Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, time.Local)
	diffDays := int(dueDate.Sub(today).Hours() / 24)
	switch {
	case diffDays < 0:
		return "overdue"
	case diffDays == 0:
		return "today"
	case diffDays <= 3:
		return "soon"
	default:
		return "future"
	}
}

func duePhrase(due *string) string {
	if due == nil || *due == "" {
		return ""
	}
	switch dueStatus(due) {
	case "":
		return "due " + *due
	case "overdue":
		return "OVERDUE " + *due
	case "today":
		return "due today"
	case "soon":
		return fmt.Sprintf("due %s (soon)", *due)
	default:
		return "due " + *due
	}
}

// agingStatus buckets how long a card has sat in its column: 3-6 days aging,
// 7-13 stale, 14+ stuck. Backlog and Completed cards never age. The clock is
// status_changed_at, falling back to created_at for rows that predate it.
func agingStatus(cardStatus string, statusChangedAt *string, createdAt string) string {
	if cardStatus == "todo" || cardStatus == "completed" {
		return ""
	}
	ref := createdAt
	if statusChangedAt != nil && *statusChangedAt != "" {
		ref = *statusChangedAt
	}
	changed, ok := parseNaiveDateTime(ref)
	if !ok {
		return ""
	}
	days := int(time.Since(changed).Hours() / 24)
	switch {
	case days < 3:
		return ""
	case days < 7:
		return "aging"
	case days < 14:
		return "stale"
	default:
		return "stuck"
	}
}

func agingPhrase(cardStatus string, statusChangedAt *string, createdAt string) string {
	bucket := agingStatus(cardStatus, statusChangedAt, createdAt)
	if bucket == "" {
		return ""
	}
	ref := createdAt
	if statusChangedAt != nil && *statusChangedAt != "" {
		ref = *statusChangedAt
	}
	changed, _ := parseNaiveDateTime(ref)
	days := int(time.Since(changed).Hours() / 24)
	return fmt.Sprintf("%dd in %s", days, statusLabels[cardStatus])
}

// priorityWord is the plain-text priority marker; medium is the unmarked
// baseline (matches the TUI).
func priorityWord(priority string) string {
	if priority == "medium" {
		return ""
	}
	if priority == "" {
		return ""
	}
	return strings.ToUpper(priority[:1]) + priority[1:]
}

// possibleDuplicate returns an open card whose title contains, or is
// contained in, the new title (both at least 4 runes). It matches on title
// only: a title+description match would false-positive against long
// descriptions.
func possibleDuplicate(title string, cards []db.Card) *db.Card {
	query := strings.ToLower(strings.TrimSpace(title))
	for i := range cards {
		c := &cards[i]
		if c.Status == "completed" {
			continue
		}
		existing := strings.ToLower(strings.TrimSpace(c.Title))
		if minInt(len([]rune(query)), len([]rune(existing))) < 4 {
			continue
		}
		if strings.Contains(existing, query) || strings.Contains(query, existing) {
			return c
		}
	}
	return nil
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// runeTruncate truncates s to at most n runes (not bytes) and reports whether
// it cut anything.
func runeTruncate(s string, n int) (truncated string, wasTruncated bool) {
	r := []rune(s)
	if len(r) <= n {
		return s, false
	}
	return string(r[:n]), true
}

var newlineSplitRE = regexp.MustCompile(`\r\n|\r|\n`)

// splitLines splits on \n, \r\n and \r; an empty string yields no lines.
func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	return newlineSplitRE.Split(s, -1)
}
