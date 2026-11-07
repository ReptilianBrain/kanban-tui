package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every modal draws on the nord1 surface, so none may use the muted nord3
// styles. A source scan keeps a future modal from reintroducing them without
// building each modal's view.
func TestModals_UseNoMutedStylesOnTheSurface(t *testing.T) {
	files, err := filepath.Glob("modal*.go")
	if err != nil || len(files) == 0 {
		t.Fatalf("no modal sources found: %v", err)
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, banned := range []string{"hintStyle.Render", "dimStyle.Render"} {
			// modalHintStyle.Render contains "HintStyle.Render", not "hintStyle.Render"
			if strings.Contains(string(src), banned) {
				t.Errorf("%s uses %s on the modal surface; use modalHintStyle or textStyle", f, banned)
			}
		}
	}
}
