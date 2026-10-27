package tui

import "testing"

func TestCardNouns(t *testing.T) {
	cases := []struct {
		skin, noun, nouns, title string
	}{
		{"sprawl", "contract", "contracts", "Contract"},
		{"plain", "card", "cards", "Card"},
		{"nonsense", "contract", "contracts", "Contract"}, // unknown skin falls back to sprawl
	}
	for _, c := range cases {
		t.Setenv("KANBANTUI_SKIN", c.skin)
		if got := cardNoun(); got != c.noun {
			t.Errorf("%s: cardNoun = %q, want %q", c.skin, got, c.noun)
		}
		if got := cardNouns(); got != c.nouns {
			t.Errorf("%s: cardNouns = %q, want %q", c.skin, got, c.nouns)
		}
		if got := cardTitle(); got != c.title {
			t.Errorf("%s: cardTitle = %q, want %q", c.skin, got, c.title)
		}
	}
}
