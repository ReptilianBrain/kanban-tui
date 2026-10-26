package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newLabelsCmd(ctx *appCtx) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "labels",
		Short: "List defined labels.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			rows, err := ctx.db.GetLabels()
			if err != nil {
				return fail(cmd, "%s", err.Error())
			}
			if asJSON {
				return printJSON(cmd.OutOrStdout(), rows)
			}
			if len(rows) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "(no labels)")
				return nil
			}
			for _, l := range rows {
				fmt.Fprintln(cmd.OutOrStdout(), l.Name)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Print labels as JSON.")
	return cmd
}
