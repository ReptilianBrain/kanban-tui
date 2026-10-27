package cost

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestCardTotal(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "card-7.json", `{"sessA":2.00,"sessB":0.5}`)
	write(t, dir, "card-8.json", `not json`)
	write(t, dir, "card-9.json", `{}`)

	if got, ok := CardTotal(dir, 7); !ok || got != 2.5 {
		t.Errorf("card 7 = %v, %v; want 2.5, true", got, ok)
	}
	for _, id := range []int64{8, 9, 10} {
		if got, ok := CardTotal(dir, id); ok {
			t.Errorf("card %d = %v, true; want no data", id, got)
		}
	}
	if _, ok := CardTotal("", 7); ok {
		t.Error("empty dir should report no data")
	}
}

func TestDir_EnvOverride(t *testing.T) {
	t.Setenv("KANBANTUI_COST_DIR", "/tmp/x")
	if got := Dir(); got != "/tmp/x" {
		t.Errorf("Dir() = %q, want the override", got)
	}
}
