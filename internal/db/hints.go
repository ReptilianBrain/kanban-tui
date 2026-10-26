package db

import (
	"fmt"
	"strings"
)

// unfinishedPhrases flag a description as reading unfinished.
var unfinishedPhrases = []string{
	"awaiting", "not yet", "still needs", "still in progress",
	"pending", "not merged", "not pushed",
}

// CompletionHint is a soft nudge, not a block: when status is "completed" or
// "in_review" and the description contains an unfinished phrase, it returns a
// warning; otherwise nil.
func CompletionHint(description, status string) *string {
	if status != "completed" && status != "in_review" {
		return nil
	}
	if description == "" {
		return nil
	}
	lower := strings.ToLower(description)
	for _, phrase := range unfinishedPhrases {
		if strings.Contains(lower, phrase) {
			hint := fmt.Sprintf(`description mentions %q — double-check this status is right`, phrase)
			return &hint
		}
	}
	return nil
}
