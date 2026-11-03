package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ReptilianBrain/kanban-tui/internal/config"
	"github.com/ReptilianBrain/kanban-tui/internal/factory"
	"github.com/ReptilianBrain/kanban-tui/internal/herdrsync"
)

// Seams so tests can run `factory run` without herdr, a repos.json or $HOME.
var (
	loadRepos    = config.LoadRepos
	openCardWith = herdrsync.OpenCardWith
	factoryDir   = defaultFactoryDir
)

func defaultFactoryDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home directory: %w", err)
	}
	return filepath.Join(home, ".kanbantui", "factory"), nil
}

func newFactoryCmd(ctx *appCtx) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "factory",
		Short: "Hand cards to unattended agents that stop at in_review.",
	}
	cmd.AddCommand(newFactoryRunCmd(ctx))
	return cmd
}

func newFactoryRunCmd(ctx *appCtx) *cobra.Command {
	var repoName string

	cmd := &cobra.Command{
		Use:   "run CARD_ID",
		Short: "Dispatch one card to an unattended agent in its own herdr worktree.",
		Long: "Moves the card to ongoing, opens a herdr worktree on card-<id>-<slug> and starts a\n" +
			"background Claude session with the factory brief: it works the card, leaves the\n" +
			"changes uncommitted, appends a handoff to the card and moves it to in_review.\n" +
			"The session may not commit, push or ship (deny rules); you review and run ship.\n\n" +
			"The repo comes from ~/.kanbantui/repos.json; pass --repo when there is more than one.",
		Args: cobra.ExactArgs(1),
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
			if card.Status != "todo" && card.Status != "ongoing" {
				return fail(cmd, "card #%d is in %s; only todo or ongoing cards can be dispatched", cardID, card.Status)
			}

			repos, err := loadRepos()
			if err != nil {
				return fail(cmd, "%s", err.Error())
			}
			repo, err := pickRepo(repos, repoName)
			if err != nil {
				return fail(cmd, "%s", err.Error())
			}

			dir, err := factoryDir()
			if err != nil {
				return fail(cmd, "%s", err.Error())
			}
			settingsPath, err := factory.WriteSettings(dir, cardID, repo.FactoryAllow)
			if err != nil {
				return fail(cmd, "%s", err.Error())
			}

			branch := herdrsync.BranchName(cardID, card.Title)
			agentName := fmt.Sprintf("card-%d", cardID)
			label := fmt.Sprintf("#%d %s", cardID, card.Title)
			prevStatus := card.Status

			// The brief says the card is already ongoing, so move it first.
			if err := ctx.db.UpdateCardStatus(cardID, "ongoing"); err != nil {
				return fail(cmd, "%s", err.Error())
			}
			res, err := openCardWith(repo.Path, repo.Base, branch, agentName, label,
				factory.Brief(cardID, card.Title, card.Description, branch),
				herdrsync.OpenOptions{PermissionMode: factory.PermissionMode, SettingsFile: settingsPath})
			if err != nil {
				// Do not leave a card in Jacked In with no agent behind it.
				if prevStatus != "ongoing" {
					_ = ctx.db.UpdateCardStatus(cardID, prevStatus)
				}
				return fail(cmd, "dispatching card #%d: %s", cardID, err.Error())
			}

			verb := "Dispatched"
			if res.Reused {
				verb = "Already running:"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s card #%d on %s (agent %s, repo %s)\n",
				verb, cardID, res.Branch, res.AgentName, repo.Name)
			return nil
		},
	}

	cmd.Flags().StringVar(&repoName, "repo", "", "Repo name from repos.json (case-insensitive substring); required when more than one is configured.")
	return cmd
}

// pickRepo resolves --repo against repos.json. An empty name is only valid
// with a single configured repo: guessing between two would open a worktree
// of the wrong project.
func pickRepo(repos []config.RepoEntry, name string) (config.RepoEntry, error) {
	if len(repos) == 0 {
		return config.RepoEntry{}, fmt.Errorf("no repos configured: add one to ~/.kanbantui/repos.json")
	}
	if name == "" {
		if len(repos) == 1 {
			return repos[0], nil
		}
		return config.RepoEntry{}, fmt.Errorf("several repos configured, pass --repo (%s)", repoNames(repos))
	}
	var matches []config.RepoEntry
	for _, r := range repos {
		if strings.EqualFold(r.Name, name) {
			return r, nil
		}
		if strings.Contains(strings.ToLower(r.Name), strings.ToLower(name)) {
			matches = append(matches, r)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return config.RepoEntry{}, fmt.Errorf("no repo matches %q (%s)", name, repoNames(repos))
	default:
		return config.RepoEntry{}, fmt.Errorf("%q matches several repos (%s)", name, repoNames(matches))
	}
}

func repoNames(repos []config.RepoEntry) string {
	names := make([]string, len(repos))
	for i, r := range repos {
		names[i] = r.Name
	}
	return strings.Join(names, ", ")
}
