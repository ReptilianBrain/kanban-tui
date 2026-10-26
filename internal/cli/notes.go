package cli

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/spf13/cobra"
)

func newNoteCmd(ctx *appCtx) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "note TEXT",
		Short: "Add a quick note (not a card — promote it from the TUI's 'o' modal).",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			text := args[0]
			noteID, err := ctx.db.CreateNote(text)
			if err != nil {
				return fail(cmd, "%s", err.Error())
			}
			if asJSON {
				n, err := ctx.db.GetNote(noteID)
				if err != nil {
					return fail(cmd, "%s", err.Error())
				}
				return printJSON(cmd.OutOrStdout(), n)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Added note #%d\n", noteID)
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Print the resulting note as JSON.")
	return cmd
}

func newNotesCmd(ctx *appCtx) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "notes",
		Short: "List quick notes (newest first).",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			rows, err := ctx.db.GetNotes()
			if err != nil {
				return fail(cmd, "%s", err.Error())
			}
			if asJSON {
				return printJSON(cmd.OutOrStdout(), rows)
			}
			if len(rows) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "(no notes)")
				return nil
			}
			out := cmd.OutOrStdout()
			for _, n := range rows {
				lines := splitLines(n.Text)
				firstLine := ""
				if len(lines) > 0 {
					firstLine = lines[0]
				}
				firstRunes := []rune(firstLine)
				if strings.Contains(n.Text, "\n") || len(firstRunes) > 60 {
					if len(firstRunes) > 60 {
						firstRunes = firstRunes[:60]
					}
					firstLine = trimTrailingSpace(string(firstRunes)) + "…"
				}
				createdAt := n.CreatedAt
				if len([]rune(createdAt)) > 16 {
					createdAt = string([]rune(createdAt)[:16])
				}
				fmt.Fprintf(out, "#%d  %s  %s\n", n.ID, createdAt, firstLine)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Print notes as a JSON array.")
	return cmd
}

func newNoteDeleteCmd(ctx *appCtx) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "note-delete NOTE_ID",
		Short: "Delete a quick note.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			noteID, err := parseNoteID(args[0])
			if err != nil {
				return err
			}
			n, err := ctx.db.GetNote(noteID)
			if err != nil {
				return fail(cmd, "%s", err.Error())
			}
			if n == nil {
				return fail(cmd, "no note with id %d", noteID)
			}
			if err := ctx.db.DeleteNote(noteID); err != nil {
				return fail(cmd, "%s", err.Error())
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Deleted note #%d\n", noteID)
			return nil
		},
	}
	return cmd
}

// trimTrailingSpace trims trailing Unicode whitespace, not just ASCII.
func trimTrailingSpace(s string) string {
	return strings.TrimRightFunc(s, unicode.IsSpace)
}
