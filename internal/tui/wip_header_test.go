package tui

import (
	"regexp"
	"testing"
)

func TestColumnHeadersShowWIPLimits(t *testing.T) {
	m := newTestModel(t)

	cases := map[string]string{
		"ongoing": `\(\d+/6\)`,
		"blocked": `\(\d+/2\)`,
	}
	for status, pattern := range cases {
		if got := m.renderColumn(status, 40, 10); !regexp.MustCompile(pattern).MatchString(got) {
			t.Errorf("%s header = %q, want it to match %s", status, got, pattern)
		}
	}
	// Lanes without a WIP limit keep the plain count.
	if got := m.renderColumn("in_review", 40, 10); regexp.MustCompile(`\(\d+/\d+\)`).MatchString(got) {
		t.Errorf("in_review header = %q, want a plain count without a limit", got)
	}
}
