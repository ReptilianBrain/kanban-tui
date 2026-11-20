// Package config loads user-maintained JSON config files that live outside
// the SQLite board.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// RepoEntry is one known repository, offered by the herdr-open picker in file
// order. Base is the git ref new worktrees are cut from; empty uses herdr's
// default.
type RepoEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Base string `json:"base,omitempty"`
}

// ReposPath returns ~/.kanbantui/repos.json, next to the board database.
func ReposPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("config: resolving home directory: %w", err)
	}
	return filepath.Join(home, ".kanbantui", "repos.json"), nil
}

// LoadRepos reads the repo list. A missing file yields an empty list, not an
// error: the file is hand-edited and a fresh install has none.
func LoadRepos() ([]RepoEntry, error) {
	path, err := ReposPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("config: reading %s: %w", path, err)
	}
	var repos []RepoEntry
	if err := json.Unmarshal(data, &repos); err != nil {
		return nil, fmt.Errorf("config: parsing %s: %w", path, err)
	}
	return repos, nil
}
