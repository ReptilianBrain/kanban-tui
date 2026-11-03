package factory

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSettingsJSON(t *testing.T) {
	data, err := SettingsJSON([]string{"Bash(make check)"})
	if err != nil {
		t.Fatal(err)
	}
	var got settings
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("settings are not valid JSON: %v", err)
	}
	has := func(list []string, want string) bool {
		for _, s := range list {
			if s == want {
				return true
			}
		}
		return false
	}
	if !has(got.Permissions.Allow, "Bash(make check)") || !has(got.Permissions.Allow, "Bash(kanban:*)") {
		t.Errorf("allow = %v, want the repo entry and the base entries", got.Permissions.Allow)
	}
	for _, want := range []string{"Bash(git commit:*)", "Bash(git push:*)", "Skill(ship)"} {
		if !has(got.Permissions.Deny, want) {
			t.Errorf("deny = %v, missing %q", got.Permissions.Deny, want)
		}
	}
	// A repo entry must never be able to punch through the deny list by
	// being listed in allow too: deny wins in claude, but keep the lists apart.
	for _, d := range got.Permissions.Deny {
		if has(got.Permissions.Allow, d) {
			t.Errorf("%q is in both allow and deny", d)
		}
	}
}

func TestSettingsJSONDoesNotMutateBaseLists(t *testing.T) {
	before := len(baseAllow)
	if _, err := SettingsJSON([]string{"Bash(make check)"}); err != nil {
		t.Fatal(err)
	}
	if len(baseAllow) != before {
		t.Fatalf("baseAllow grew from %d to %d", before, len(baseAllow))
	}
}

func TestWriteSettings(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "factory")
	path, err := WriteSettings(dir, 42, nil)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != "card-42.settings.json" {
		t.Errorf("path = %q", path)
	}
	data, err := os.ReadFile(path)
	if err != nil || !json.Valid(data) {
		t.Fatalf("file unreadable or invalid JSON: %v", err)
	}
}

func TestBrief(t *testing.T) {
	b := Brief(7, "Fix it", "Do the thing.", "card-7-fix-it")
	for _, want := range []string{
		"card #7: Fix it",
		"Do the thing.",
		`"card-7-fix-it"`,
		"kanban update 7 --append",
		"kanban move 7 in_review",
		"Do not commit",
	} {
		if !strings.Contains(b, want) {
			t.Errorf("brief missing %q:\n%s", want, b)
		}
	}
	if strings.Contains(b, "%!") {
		t.Errorf("brief has a broken format verb:\n%s", b)
	}
	if empty := Brief(7, "T", "", "b"); strings.Contains(empty, "\n\n\n") {
		t.Errorf("empty description leaves a gap:\n%s", empty)
	}
}
