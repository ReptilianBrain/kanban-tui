package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ReptilianBrain/kanban-tui/internal/db"
)

func newAddCmd(ctx *appCtx) *cobra.Command {
	var (
		description string
		status      string
		priority    string
		due         string
		label       string
		jira        string
		asJSON      bool
	)

	cmd := &cobra.Command{
		Use:   "add TITLE",
		Short: "Create a new card.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			title := args[0]

			if err := validateDue(cmd, due); err != nil {
				return err
			}
			jiraKey, err := db.NormalizeJiraKey(jira)
			if err != nil {
				return fail(cmd, "%s", err.Error())
			}
			if label != "" {
				if err := ctx.db.EnsureLabel(label); err != nil {
					return fail(cmd, "%s", err.Error())
				}
			}

			allCards, err := ctx.db.GetAllCards()
			if err != nil {
				return fail(cmd, "%s", err.Error())
			}
			if dup := possibleDuplicate(title, allCards); dup != nil {
				fmt.Fprintf(cmd.ErrOrStderr(),
					"Note: this looks similar to existing card #%d \"%s\" — check it's not a duplicate\n",
					dup.ID, dup.Title)
			}

			var dueDate, lbl, jiraPtr *string
			if due != "" {
				dueDate = &due
			}
			if label != "" {
				lbl = &label
			}
			if jiraKey != "" {
				jiraPtr = &jiraKey
			}
			cardID, err := ctx.db.CreateCard(title, description, status, priority, dueDate, lbl, jiraPtr)
			if err != nil {
				return fail(cmd, "%s", err.Error())
			}
			card, err := ctx.db.GetCard(cardID)
			if err != nil {
				return fail(cmd, "%s", err.Error())
			}
			return emit(cmd, card, asJSON, fmt.Sprintf(`Created card #%d "%s" in %s`, cardID, title, status))
		},
	}

	cmd.Flags().StringVarP(&description, "description", "m", "", "Card description.")
	cmd.Flags().VarP(newChoiceFlag(&status, allStatuses, "todo"), "status", "s", "Column to create the card in.")
	cmd.Flags().VarP(newChoiceFlag(&priority, priorities, "medium"), "priority", "p", "")
	cmd.Flags().StringVar(&due, "due", "", "Due date (YYYY-MM-DD).")
	cmd.Flags().StringVar(&label, "label", "", "Label name (created if it's new).")
	cmd.Flags().StringVar(&jira, "jira", "", "Jira ticket key (ABC-123) or browse URL.")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Print the resulting card as JSON.")

	return cmd
}
