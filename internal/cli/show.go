package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func newShowCmd(ctx *appCtx) *cobra.Command {
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "show CARD_ID",
		Short: "Show a single card by id.",
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
			if asJSON {
				return printJSON(cmd.OutOrStdout(), card)
			}

			bits := []string{
				fmt.Sprintf("#%d", card.ID),
				fmt.Sprintf("[%s]", card.Priority),
				card.Title,
				fmt.Sprintf("(%s)", statusLabels[card.Status]),
			}
			if card.DueDate != nil && *card.DueDate != "" {
				bits = append(bits, "due:"+*card.DueDate)
			}
			if aging := agingPhrase(card.Status, card.StatusChangedAt, card.CreatedAt); aging != "" {
				bits = append(bits, aging)
			}
			if card.Label != nil && *card.Label != "" {
				bits = append(bits, "#"+*card.Label)
			}
			fmt.Fprintln(cmd.OutOrStdout(), strings.Join(bits, " "))
			if card.Description != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "\n%s\n", card.Description)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "Print the card as JSON.")
	return cmd
}
