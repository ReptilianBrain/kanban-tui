package db

import "time"

// nowStamp returns the current local time as ISO 8601 with no zone suffix and
// a six-digit fraction (omitted when zero). Local, not UTC, to stay consistent
// with timestamps already stored in existing databases.
func nowStamp() string {
	now := time.Now()
	if now.Nanosecond() != 0 {
		return now.Format("2006-01-02T15:04:05.000000")
	}
	return now.Format("2006-01-02T15:04:05")
}
