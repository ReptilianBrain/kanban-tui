// Command kanban is KanbanTUI's entry point. With no arguments it launches
// the Bubble Tea TUI (internal/tui); with arguments it runs the Cobra CLI
// (internal/cli) and exits with its return code.
package main

import (
	"fmt"
	"os"

	"kanbantui/internal/cli"
	"kanbantui/internal/tui"
)

func main() {
	if len(os.Args) <= 1 {
		if err := tui.Run(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	os.Exit(cli.Execute(os.Args[1:]))
}
