package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/ReptilianBrain/kanban-tui/internal/db"
)

// exportCard is the reduced per-card shape used by export --json.
type exportCard struct {
	ID          int64   `json:"id"`
	Title       string  `json:"title"`
	Description string  `json:"description"`
	Priority    string  `json:"priority"`
	DueDate     *string `json:"due_date"`
	DueStatus   *string `json:"due_status"`
	AgingStatus *string `json:"aging_status"`
	Label       *string `json:"label"`
	JiraKey     *string `json:"jira_key"`
}

type exportColumn struct {
	Status string       `json:"status"`
	Label  string       `json:"label"`
	Count  int          `json:"count"`
	Cards  []exportCard `json:"cards"`
}

type exportSnapshot struct {
	GeneratedAt      string         `json:"generated_at"`
	IncludeCompleted bool           `json:"include_completed"`
	OpenCardCount    int            `json:"open_card_count"`
	Columns          []exportColumn `json:"columns"`
}

// nilIfEmpty returns nil for an empty string, otherwise a pointer to it —
// used so due_status/aging_status serialize as JSON null rather than "".
func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func newExportCmd(ctx *appCtx) *cobra.Command {
	var (
		includeCompleted bool
		asJSON           bool
	)

	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export a board snapshot grouped by column, for sharing outside the TUI.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cards, err := ctx.db.GetAllCards()
			if err != nil {
				return fail(cmd, "%s", err.Error())
			}

			statuses := allStatuses
			if !includeCompleted {
				statuses = make([]string, 0, len(allStatuses))
				for _, s := range allStatuses {
					if s != "completed" {
						statuses = append(statuses, s)
					}
				}
			}
			statusSet := map[string]bool{}
			for _, s := range statuses {
				statusSet[s] = true
			}

			byStatus := map[string][]db.Card{}
			for _, s := range statuses {
				byStatus[s] = []db.Card{}
			}
			for _, c := range cards {
				if statusSet[c.Status] {
					byStatus[c.Status] = append(byStatus[c.Status], c)
				}
			}

			if asJSON {
				columns := make([]exportColumn, 0, len(statuses))
				total := 0
				for _, s := range statuses {
					group := byStatus[s]
					total += len(group)
					ecs := make([]exportCard, 0, len(group))
					for _, c := range group {
						ecs = append(ecs, exportCard{
							ID:          c.ID,
							Title:       c.Title,
							Description: c.Description,
							Priority:    c.Priority,
							DueDate:     c.DueDate,
							DueStatus:   nilIfEmpty(dueStatus(c.DueDate)),
							AgingStatus: nilIfEmpty(agingStatus(c.Status, c.StatusChangedAt, c.CreatedAt)),
							Label:       c.Label,
							JiraKey:     c.JiraKey,
						})
					}
					columns = append(columns, exportColumn{
						Status: s, Label: statusLabels[s], Count: len(group), Cards: ecs,
					})
				}
				snapshot := exportSnapshot{
					GeneratedAt:      time.Now().Format("2006-01-02"),
					IncludeCompleted: includeCompleted,
					OpenCardCount:    total,
					Columns:          columns,
				}
				return printJSON(cmd.OutOrStdout(), snapshot)
			}

			out := cmd.OutOrStdout()
			total := 0
			for _, s := range statuses {
				total += len(byStatus[s])
			}
			fmt.Fprintf(out, "# Board Snapshot — %s\n\n", time.Now().Format("2006-01-02"))
			note := ""
			if !includeCompleted {
				note = " (Completed excluded — pass --include-completed to include it)"
			}
			fmt.Fprintf(out, "_%d open cards across %d columns%s_\n", total, len(statuses), note)
			for _, s := range statuses {
				group := byStatus[s]
				fmt.Fprintf(out, "\n## %s (%d)\n", statusLabels[s], len(group))
				if len(group) == 0 {
					fmt.Fprintln(out, "_(no cards)_")
					continue
				}
				for _, c := range group {
					bits := []string{fmt.Sprintf("**%s**", c.Title)}
					var meta []string
					if w := priorityWord(c.Priority); w != "" {
						meta = append(meta, w)
					}
					if p := duePhrase(c.DueDate); p != "" {
						meta = append(meta, p)
					}
					if p := agingPhrase(c.Status, c.StatusChangedAt, c.CreatedAt); p != "" {
						meta = append(meta, p)
					}
					if c.Label != nil && *c.Label != "" {
						meta = append(meta, fmt.Sprintf("[%s]", *c.Label))
					}
					if c.JiraKey != nil && *c.JiraKey != "" {
						meta = append(meta, *c.JiraKey)
					}
					if len(meta) > 0 {
						bits = append(bits, "— "+strings.Join(meta, " · "))
					}
					fmt.Fprintln(out, "- "+strings.Join(bits, " "))
					if c.Description != "" {
						for _, line := range splitLines(c.Description) {
							fmt.Fprintf(out, "  %s\n", line)
						}
					}
				}
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&includeCompleted, "include-completed", false, "Include the Completed column (excluded by default).")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Print a structured JSON snapshot instead of Markdown.")
	return cmd
}
