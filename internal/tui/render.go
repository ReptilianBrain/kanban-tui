package tui

import (
	"strings"
	"time"

	"kanbantui/internal/db"
)

// priorityBadge is a styled plain-word badge with a trailing space baked in,
// so it concatenates directly in front of the title. Medium has no badge.
func priorityBadge(priority string) string {
	switch priority {
	case "low":
		return dimStyle.Render("low") + " "
	case "high":
		return warnStyle.Render("high") + " "
	case "critical":
		return errorBoldStyle.Render("critical") + " "
	default: // "medium" and anything unrecognized
		return ""
	}
}

// parseDate parses a YYYY-MM-DD date in the local zone.
func parseDate(s string) (time.Time, error) {
	return time.ParseInLocation("2006-01-02", s, time.Local)
}

// dueMarkup: unparseable -> dim flag + raw string; overdue -> bold error;
// today -> bold warning "today" (not the date); within 3 days -> warning +
// date; further out -> dim + date; no due date -> "".
func dueMarkup(dueDate *string) string {
	if dueDate == nil || *dueDate == "" {
		return ""
	}
	due, err := parseDate(*dueDate)
	if err != nil {
		return dimStyle.Render("⚑ " + *dueDate)
	}
	// Diff calendar dates, not durations: both sides are rebuilt at UTC
	// midnight so a DST transition between today and the due date can't turn
	// a 24h day into 23h/25h and round into the wrong bucket.
	today := time.Now().Local()
	todayUTC := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)
	dueUTC := time.Date(due.Year(), due.Month(), due.Day(), 0, 0, 0, 0, time.UTC)
	diffDays := int(dueUTC.Sub(todayUTC).Hours() / 24)
	switch {
	case diffDays < 0:
		return errorBoldStyle.Render("⚑ " + *dueDate)
	case diffDays == 0:
		return warnBoldStyle.Render("⚑ today")
	case diffDays <= 3:
		return warnStyle.Render("⚑ " + *dueDate)
	default:
		return dimStyle.Render("⚑ " + *dueDate)
	}
}

func labelMarkup(label *string) string {
	if label == nil || *label == "" {
		return ""
	}
	return cyanStyle.Render("#" + *label)
}

// parseTimestamp tries the timestamp shapes the db writes: with
// microseconds, without, and a bare date. Anything else is an error.
func parseTimestamp(s string) (time.Time, error) {
	layouts := []string{
		"2006-01-02T15:04:05.000000",
		"2006-01-02T15:04:05",
		"2006-01-02",
	}
	var lastErr error
	for _, layout := range layouts {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return t, nil
		} else {
			lastErr = err
		}
	}
	return time.Time{}, lastErr
}

// agingMarkup: never shown for todo/completed; falls back to created_at when
// status_changed_at is unset; unparseable reference -> ""; <3 days -> "";
// 3-6 dim; 7-13 warning; 14+ bold error.
func agingMarkup(status string, statusChangedAt *string, createdAt string) string {
	if status == "todo" || status == "completed" {
		return ""
	}
	reference := createdAt
	if statusChangedAt != nil && *statusChangedAt != "" {
		reference = *statusChangedAt
	}
	changed, err := parseTimestamp(reference)
	if err != nil {
		return ""
	}
	// time.Since is elapsed time, not wall-clock, so around a DST transition a
	// card can cross a 3/7/14-day boundary up to an hour early or late. That is
	// cosmetic and self-corrects, so it isn't rebuilt like dueMarkup's day diff.
	days := int(time.Since(changed).Hours() / 24)
	switch {
	case days < 3:
		return ""
	case days < 7:
		return dimStyle.Render(hourglassAge(days))
	case days < 14:
		return warnStyle.Render(hourglassAge(days))
	default:
		return errorBoldStyle.Render(hourglassAge(days))
	}
}

func hourglassAge(days int) string {
	return "⏳ " + itoa(days) + "d"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}

// cardBlockLines returns a card's content lines, unbordered (the caller adds
// border and background): "#id badge title", then an optional meta line.
func cardBlockLines(card db.Card) []string {
	idPrefix := dimStyle.Render("#" + itoa64(card.ID))
	badge := priorityBadge(card.Priority)
	title := card.Title
	first := idPrefix + " " + badge + boldStyle.Render(title)
	lines := []string{first}

	var due *string
	due = card.DueDate
	parts := []string{}
	if p := dueMarkup(due); p != "" {
		parts = append(parts, p)
	}
	if p := agingMarkup(card.Status, card.StatusChangedAt, card.CreatedAt); p != "" {
		parts = append(parts, p)
	}
	if p := labelMarkup(card.Label); p != "" {
		parts = append(parts, p)
	}
	if len(parts) > 0 {
		lines = append(lines, strings.Join(parts, "  "))
	}
	return lines
}

func itoa64(n int64) string {
	return itoa(int(n))
}
