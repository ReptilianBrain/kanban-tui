package db

import "testing"

func TestFuzzyScore(t *testing.T) {
	tests := []struct {
		name        string
		query       string
		text        string
		wantMatched bool
	}{
		{
			name:        "exact substring wins",
			query:       "login",
			text:        "fix login bug",
			wantMatched: true,
		},
		{
			name:        "typo-tolerant subsequence",
			query:       "lgn",
			text:        "login",
			wantMatched: true,
		},
		{
			name:        "tighter cluster over early false start",
			query:       "strp",
			text:        "our payment service now talks to stripe directly",
			wantMatched: true,
		},
		{
			name:        "span too wide to match",
			query:       "abcd",
			text:        "a....................b....................c....................d",
			wantMatched: false,
		},
		{
			name:        "empty query never matches",
			query:       "",
			text:        "anything",
			wantMatched: false,
		},
		{
			name:        "case insensitive substring",
			query:       "LOGIN",
			text:        "fix login bug",
			wantMatched: true,
		},
		{
			name:        "missing character breaks subsequence",
			query:       "xyz",
			text:        "kanban",
			wantMatched: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, matched := FuzzyScore(tt.query, tt.text)
			if matched != tt.wantMatched {
				t.Errorf("FuzzyScore(%q, %q) matched=%v, want %v", tt.query, tt.text, matched, tt.wantMatched)
			}
		})
	}
}

// "strp" must score the tight cluster in "stripe", not the early false start
// at the "s" of "service"; assert the exact score, not just a match.
func TestFuzzyScorePrefersTighterCluster(t *testing.T) {
	text := "our payment service now talks to stripe directly"
	scoreTight, matched := FuzzyScore("strp", text)
	if !matched {
		t.Fatalf("expected a match")
	}

	// Locate the expected "stripe" placement and score it by hand.
	idx := indexOf(text, "stripe")
	if idx == -1 {
		t.Fatalf("missing expected substring")
	}
	// s, t, r are consecutive; p follows the gap at the i.
	positions := []int{idx, idx + 1, idx + 2, idx + 4}
	want := 0
	consecutive := 0
	prev := -1
	for i, p := range positions {
		if i > 0 && p == prev+1 {
			consecutive++
			want += 2 + consecutive
		} else {
			consecutive = 0
			want++
		}
		prev = p
	}
	if scoreTight != want {
		t.Errorf("expected score %d for the tight stripe-cluster placement, got %d", want, scoreTight)
	}
}

func indexOf(text, substr string) int {
	for i := 0; i+len(substr) <= len(text); i++ {
		if text[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func TestCardMatchesSearch(t *testing.T) {
	card := Card{Title: "Fix login bug", Description: "users can't sign in"}
	if !CardMatchesSearch(card, "lgn") {
		t.Errorf("expected fuzzy match against title")
	}
	if !CardMatchesSearch(card, "sign") {
		t.Errorf("expected match against description")
	}
	if CardMatchesSearch(card, "xyz123") {
		t.Errorf("expected no match for unrelated query")
	}

	idCard := Card{ID: 286, Title: "Refactor auth", Description: "unrelated"}
	if !CardMatchesSearch(idCard, "286") {
		t.Errorf("expected match against card id")
	}
	if !CardMatchesSearch(idCard, "28") {
		t.Errorf("expected substring match against card id")
	}
	if CardMatchesSearch(idCard, "999") {
		t.Errorf("expected no match for unrelated id query")
	}
}

func TestCompletionHint(t *testing.T) {
	tests := []struct {
		name        string
		description string
		status      string
		wantHint    bool
	}{
		{"todo status never hints", "still needs review", "todo", false},
		{"ongoing status never hints", "still needs review", "ongoing", false},
		{"completed with trigger phrase hints", "still needs review", "completed", true},
		{"in_review with trigger phrase hints", "not yet merged", "in_review", true},
		{"completed without trigger phrase", "all done, shipped", "completed", false},
		{"empty description never hints", "", "completed", false},
		{"case insensitive match", "Still Needs testing", "completed", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CompletionHint(tt.description, tt.status)
			if (got != nil) != tt.wantHint {
				t.Errorf("CompletionHint(%q, %q) = %v, want non-nil=%v", tt.description, tt.status, got, tt.wantHint)
			}
		})
	}
}
