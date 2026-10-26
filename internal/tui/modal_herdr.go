package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"kanbantui/internal/config"
)

// herdrRepoModal is a left/right cycler over the known repo paths in
// ~/.kanbantui/repos.json, shown by the herdr pickup keybinding.
type herdrRepoModal struct {
	repos []config.RepoEntry
	idx   int
	done  func(m *Model, repo config.RepoEntry) tea.Cmd
}

func newHerdrRepoModal(repos []config.RepoEntry, done func(m *Model, repo config.RepoEntry) tea.Cmd) *herdrRepoModal {
	return &herdrRepoModal{repos: repos, done: done}
}

func (h *herdrRepoModal) Init(m *Model) tea.Cmd { return nil }

func (h *herdrRepoModal) Update(msg tea.Msg, m *Model) tea.Cmd {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil
	}
	switch keyMsg.String() {
	case "esc":
		m.popModal()
		return nil
	case "enter":
		if len(h.repos) == 0 {
			m.popModal()
			return nil
		}
		repo := h.repos[h.idx]
		m.popModal()
		return h.done(m, repo)
	case "left", "h":
		if len(h.repos) > 0 {
			h.idx = (h.idx - 1 + len(h.repos)) % len(h.repos)
		}
	case "right", "l":
		if len(h.repos) > 0 {
			h.idx = (h.idx + 1) % len(h.repos)
		}
	}
	return nil
}

func (h *herdrRepoModal) View(m *Model) string {
	body := accentBold.Render("Open in herdr") + "\n\n"
	if len(h.repos) == 0 {
		body += hintStyle.Render("No repos configured — add one to ~/.kanbantui/repos.json") + "\n\n"
	} else {
		entry := h.repos[h.idx]
		body += pickerValue(entry.Name, true) + "\n"
		body += hintStyle.Render(entry.Path) + "\n"
		if entry.Base != "" {
			body += hintStyle.Render("base: "+entry.Base) + "\n"
		}
		body += "\n"
	}
	body += hintStyle.Render("←/→: change   Enter: open   Esc: cancel")
	return renderModal(modalBoxStyle, 60, body)
}
