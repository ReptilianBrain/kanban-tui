package cli

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"

	"kanbantui/internal/db"
)

func newNextCmd(ctx *appCtx) *cobra.Command {
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "next",
		Short: "Suggest the single best card to pick up next.",
		Long: "Suggest the single best card to pick up next: highest priority, then\n" +
			"oldest, among cards in Backlog or In Progress. Excludes Blocked (nothing\n" +
			"actionable there) and In Review (already sent onward).",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cards, err := ctx.db.GetAllCards()
			if err != nil {
				return fail(cmd, "%s", err.Error())
			}

			var candidates []db.Card
			for _, c := range cards {
				if c.Status == "todo" || c.Status == "ongoing" {
					candidates = append(candidates, c)
				}
			}

			if len(candidates) == 0 {
				blocked := 0
				for _, c := range cards {
					if c.Status == "blocked" {
						blocked++
					}
				}
				if asJSON {
					fmt.Fprintln(cmd.OutOrStdout(), "null")
				} else if blocked > 0 {
					fmt.Fprintf(cmd.OutOrStdout(), "(nothing actionable — %d blocked)\n", blocked)
				} else {
					fmt.Fprintln(cmd.OutOrStdout(), "(no cards)")
				}
				return nil
			}

			weight := map[string]int{}
			for i, p := range priorities {
				weight[p] = i
			}
			sort.SliceStable(candidates, func(i, j int) bool {
				wi, wj := weight[candidates[i].Priority], weight[candidates[j].Priority]
				if wi != wj {
					return wi > wj
				}
				return candidates[i].CreatedAt < candidates[j].CreatedAt
			})
			best := candidates[0]
			msg := fmt.Sprintf("#%d [%s] %s (%s)", best.ID, best.Priority, best.Title, statusLabels[best.Status])
			return emit(cmd, best, asJSON, msg)
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "Print the resulting card as JSON.")
	return cmd
}
