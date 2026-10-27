package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ReptilianBrain/kanban-tui/internal/db"
)

func newUpdateCmd(ctx *appCtx) *cobra.Command {
	var (
		title       string
		description string
		status      string
		priority    string
		due         string
		label       string
		jira        string
		asJSON      bool
	)

	cmd := &cobra.Command{
		Use:   "update CARD_ID",
		Short: "Modify an existing card. Only the options you pass are changed.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cardID, err := parseCardID(args[0])
			if err != nil {
				return err
			}
			card, err := ctx.db.GetCard(cardID)
			if err != nil {
				return fail(cmd, "%s", err.Error())
			}
			if card == nil {
				return fail(cmd, "no card with id %d", cardID)
			}

			newTitle := card.Title
			if cmd.Flags().Changed("title") {
				newTitle = title
			}
			newDesc := card.Description
			if cmd.Flags().Changed("description") {
				newDesc = description
			}
			newPriority := card.Priority
			if cmd.Flags().Changed("priority") {
				newPriority = priority
			}

			newDue := card.DueDate
			if cmd.Flags().Changed("due") {
				if err := validateDue(cmd, due); err != nil {
					return err
				}
				if due == "" {
					newDue = nil
				} else {
					d := due
					newDue = &d
				}
			}

			newLabel := card.Label
			if cmd.Flags().Changed("label") {
				if label == "" {
					newLabel = nil
				} else {
					l := label
					newLabel = &l
					if err := ctx.db.EnsureLabel(label); err != nil {
						return fail(cmd, "%s", err.Error())
					}
				}
			}

			newJira := card.JiraKey
			if cmd.Flags().Changed("jira") {
				key, err := db.NormalizeJiraKey(jira)
				if err != nil {
					return fail(cmd, "%s", err.Error())
				}
				if key == "" {
					newJira = nil
				} else {
					newJira = &key
				}
			}

			if err := ctx.db.UpdateCard(cardID, newTitle, newDesc, newPriority, newDue, newLabel, newJira); err != nil {
				return fail(cmd, "%s", err.Error())
			}

			if cmd.Flags().Changed("status") && status != card.Status {
				if err := ctx.db.UpdateCardStatus(cardID, status); err != nil {
					return fail(cmd, "%s", err.Error())
				}
				if hint := db.CompletionHint(newDesc, status); hint != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "Note: %s\n", *hint)
				}
			}

			updated, err := ctx.db.GetCard(cardID)
			if err != nil {
				return fail(cmd, "%s", err.Error())
			}
			return emit(cmd, updated, asJSON, fmt.Sprintf(`Updated card #%d "%s"`, cardID, newTitle))
		},
	}

	cmd.Flags().StringVarP(&title, "title", "t", "", "New title.")
	cmd.Flags().StringVarP(&description, "description", "m", "", "New description.")
	cmd.Flags().VarP(&choiceValue{value: &status, allowed: allStatuses}, "status", "s", "Move to a different column.")
	cmd.Flags().VarP(&choiceValue{value: &priority, allowed: priorities}, "priority", "p", "")
	cmd.Flags().StringVar(&due, "due", "", `Due date YYYY-MM-DD, or "" to clear.`)
	cmd.Flags().StringVar(&label, "label", "", `Label name, or "" to clear.`)
	cmd.Flags().StringVar(&jira, "jira", "", `Jira ticket key (ABC-123) or browse URL, or "" to clear.`)
	cmd.Flags().BoolVar(&asJSON, "json", false, "Print the resulting card as JSON.")

	return cmd
}
