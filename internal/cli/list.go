package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ReptilianBrain/kanban-tui/internal/db"
)

func newListCmd(ctx *appCtx) *cobra.Command {
	var (
		status  string
		label   string
		search  string
		asJSON  bool
		fields  []string
		title   string
		overdue bool
		noDue   bool
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List cards (optionally filtered by column, label, or a fuzzy search).",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			all, err := ctx.db.GetAllCards()
			if err != nil {
				return fail(cmd, "%s", err.Error())
			}

			cards := []db.Card{}
			for _, c := range all {
				if cmd.Flags().Changed("status") && c.Status != status {
					continue
				}
				if label != "" && (c.Label == nil || *c.Label != label) {
					continue
				}
				if search != "" && !db.CardMatchesSearch(c, search) {
					continue
				}
				if title != "" && !strings.Contains(strings.ToLower(c.Title), strings.ToLower(title)) {
					continue
				}
				if overdue && (c.Status == "completed" || dueStatus(c.DueDate) != "overdue") {
					continue
				}
				if noDue && (c.Status == "completed" || (c.DueDate != nil && *c.DueDate != "")) {
					continue
				}
				cards = append(cards, c)
			}

			if len(fields) > 0 {
				projected, err := projectCards(cards, fields)
				if err != nil {
					return err
				}
				return printCompactArray(cmd.OutOrStdout(), projected)
			}
			if asJSON {
				return printJSON(cmd.OutOrStdout(), cards)
			}
			if len(cards) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "(no cards)")
				return nil
			}

			byStatus := map[string][]db.Card{}
			for _, c := range cards {
				byStatus[c.Status] = append(byStatus[c.Status], c)
			}
			out := cmd.OutOrStdout()
			for _, s := range allStatuses {
				group := byStatus[s]
				if len(group) == 0 {
					continue
				}
				fmt.Fprintf(out, "\n%s (%d)\n", s, len(group))
				for _, c := range group {
					bits := []string{fmt.Sprintf("  #%d", c.ID), fmt.Sprintf("[%s]", c.Priority), c.Title}
					if c.DueDate != nil && *c.DueDate != "" {
						bits = append(bits, "due:"+*c.DueDate)
					}
					if aging := agingPhrase(c.Status, c.StatusChangedAt, c.CreatedAt); aging != "" {
						bits = append(bits, aging)
					}
					if c.Label != nil && *c.Label != "" {
						bits = append(bits, "#"+*c.Label)
					}
					if c.JiraKey != nil && *c.JiraKey != "" {
						bits = append(bits, "jira:"+*c.JiraKey)
					}
					fmt.Fprintln(out, strings.Join(bits, " "))
				}
			}
			return nil
		},
	}

	cmd.Flags().VarP(&choiceValue{value: &status, allowed: allStatuses}, "status", "s", "Only cards in this column.")
	cmd.Flags().StringVar(&label, "label", "", "Only cards with this label.")
	cmd.Flags().StringVar(&search, "search", "", "Fuzzy-search title, description & Jira key.")
	cmd.Flags().StringVar(&title, "title", "", "Only cards whose title contains this text (case-insensitive; unlike --search, descriptions are ignored).")
	cmd.Flags().BoolVar(&overdue, "overdue", false, "Only open cards past their due date.")
	cmd.Flags().BoolVar(&noDue, "no-due", false, "Only open cards with no due date.")
	cmd.MarkFlagsMutuallyExclusive("overdue", "no-due")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Print cards as a JSON array.")
	cmd.Flags().StringSliceVar(&fields, "fields", nil, "Comma-separated card fields to print as JSON (implies --json), e.g. id,title,status. Descriptions make the full output large.")
	return cmd
}

// orderedFields marshals as a JSON object whose keys keep the order the user
// asked for, which a plain map would not.
type orderedFields struct {
	keys   []string
	values map[string]json.RawMessage
}

func (o orderedFields) MarshalJSON() ([]byte, error) {
	var b strings.Builder
	b.WriteByte('{')
	for i, k := range o.keys {
		if i > 0 {
			b.WriteByte(',')
		}
		name, _ := json.Marshal(k)
		b.Write(name)
		b.WriteByte(':')
		b.Write(o.values[k])
	}
	b.WriteByte('}')
	return []byte(b.String()), nil
}

// projectCards keeps only the named JSON fields of each card. Valid names are
// the Card JSON keys, read off a marshalled zero value so the list cannot
// drift from the struct.
func projectCards(cards []db.Card, fields []string) ([]orderedFields, error) {
	zero, err := json.Marshal(db.Card{})
	if err != nil {
		return nil, err
	}
	var valid map[string]json.RawMessage
	if err := json.Unmarshal(zero, &valid); err != nil {
		return nil, err
	}
	for _, f := range fields {
		if _, ok := valid[f]; !ok {
			names := make([]string, 0, len(valid))
			for k := range valid {
				names = append(names, k)
			}
			sort.Strings(names)
			return nil, fmt.Errorf("unknown field %q: must be one of %s", f, strings.Join(names, ", "))
		}
	}

	out := make([]orderedFields, 0, len(cards))
	for _, c := range cards {
		raw, err := json.Marshal(c)
		if err != nil {
			return nil, err
		}
		var all map[string]json.RawMessage
		if err := json.Unmarshal(raw, &all); err != nil {
			return nil, err
		}
		out = append(out, orderedFields{keys: fields, values: all})
	}
	return out, nil
}

// printCompactArray prints a JSON array with one compact element per line:
// still valid JSON for jq, but far smaller than the indented form when a
// caller only asked for a few fields.
func printCompactArray(w io.Writer, items []orderedFields) error {
	var b strings.Builder
	b.WriteString("[\n")
	for i, it := range items {
		raw, err := it.MarshalJSON()
		if err != nil {
			return err
		}
		b.Write(raw)
		if i < len(items)-1 {
			b.WriteByte(',')
		}
		b.WriteByte('\n')
	}
	b.WriteString("]\n")
	_, err := io.WriteString(w, b.String())
	return err
}
