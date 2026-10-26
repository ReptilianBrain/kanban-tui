package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"kanbantui/internal/db"
)

func newMoveCmd(ctx *appCtx) *cobra.Command {
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "move CARD_ID STATUS",
		Short: "Move a card to another column.",
		Args: func(cmd *cobra.Command, args []string) error {
			if err := cobra.ExactArgs(2)(cmd, args); err != nil {
				return err
			}
			if !isValidChoice(args[1], allStatuses) {
				return fmt.Errorf("invalid status %q: must be one of %s", args[1], strings.Join(allStatuses, ", "))
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			cardID, err := parseCardID(args[0])
			if err != nil {
				return err
			}
			status := args[1]

			card, err := ctx.db.GetCard(cardID)
			if err != nil {
				return fail(cmd, "%s", err.Error())
			}
			if card == nil {
				return fail(cmd, "no card with id %d", cardID)
			}

			if err := ctx.db.UpdateCardStatus(cardID, status); err != nil {
				return fail(cmd, "%s", err.Error())
			}
			if hint := db.CompletionHint(card.Description, status); hint != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "Note: %s\n", *hint)
			}

			updated, err := ctx.db.GetCard(cardID)
			if err != nil {
				return fail(cmd, "%s", err.Error())
			}
			return emit(cmd, updated, asJSON, fmt.Sprintf(`Moved card #%d "%s" to %s`, cardID, card.Title, status))
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "Print the resulting card as JSON.")
	return cmd
}
