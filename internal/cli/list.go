package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"kanbantui/internal/db"
)

func newListCmd(ctx *appCtx) *cobra.Command {
	var (
		status string
		label  string
		search string
		asJSON bool
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
				cards = append(cards, c)
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
					fmt.Fprintln(out, strings.Join(bits, " "))
				}
			}
			return nil
		},
	}

	cmd.Flags().VarP(&choiceValue{value: &status, allowed: allStatuses}, "status", "s", "Only cards in this column.")
	cmd.Flags().StringVar(&label, "label", "", "Only cards with this label.")
	cmd.Flags().StringVar(&search, "search", "", "Fuzzy-search title & description.")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Print cards as a JSON array.")
	return cmd
}
