package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newDeleteCmd(ctx *appCtx) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete CARD_ID",
		Short: "Delete a card.",
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
			if err := ctx.db.DeleteCard(cardID); err != nil {
				return fail(cmd, "%s", err.Error())
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Deleted card #%d \"%s\"\n", cardID, card.Title)
			return nil
		},
	}
	return cmd
}
