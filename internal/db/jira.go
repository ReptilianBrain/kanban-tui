package db

import (
	"fmt"
	"regexp"
	"strings"
)

var jiraKeyRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]*-[0-9]+$`)

// NormalizeJiraKey returns the upper-cased ticket key from s, which is either
// a bare key ("prj-123") or a browse URL ending in one. Empty input yields "".
func NormalizeJiraKey(s string) (string, error) {
	k := strings.TrimSpace(s)
	if k == "" {
		return "", nil
	}
	if i := strings.IndexAny(k, "?#"); i != -1 {
		k = k[:i]
	}
	k = k[strings.LastIndex(k, "/")+1:]
	if !jiraKeyRe.MatchString(k) {
		return "", fmt.Errorf("Jira key must look like ABC-123, got %q", strings.TrimSpace(s))
	}
	return strings.ToUpper(k), nil
}
