package db

import (
	"strconv"
	"strings"
	"unicode/utf8"
)

// subsequencePositions greedily locates each rune of query in text, in order,
// returning rune offsets and whether every rune was found.
//
// Leftmost placement can lock onto an early false start ("strp" against
// "...service...stripe..." takes the "s" of "service"); rightmost (reverse)
// catches that, so FuzzyScore tries both and keeps the tighter one.
func subsequencePositions(query, text []rune, reverse bool) ([]int, bool) {
	positions := make([]int, 0, len(query))
	if !reverse {
		pos := 0
		for _, ch := range query {
			idx := -1
			for i := pos; i < len(text); i++ {
				if text[i] == ch {
					idx = i
					break
				}
			}
			if idx == -1 {
				return nil, false
			}
			positions = append(positions, idx)
			pos = idx + 1
		}
		return positions, true
	}

	pos := len(text) - 1
	for i := len(query) - 1; i >= 0; i-- {
		ch := query[i]
		idx := -1
		for j := pos; j >= 0; j-- {
			if text[j] == ch {
				idx = j
				break
			}
		}
		if idx == -1 {
			return nil, false
		}
		positions = append(positions, idx)
		pos = idx - 1
	}
	// Built back-to-front; reverse to ascending order.
	for i, j := 0, len(positions)-1; i < j; i, j = i+1, j-1 {
		positions[i], positions[j] = positions[j], positions[i]
	}
	return positions, true
}

// FuzzyScore scores a case-insensitive fuzzy match of query against text.
//
// A substring hit always wins, ranked by how early it appears. Otherwise
// query must be an in-order subsequence ("kbn" matches "kanban"), ranked by
// consecutive hits and bounded to a short span so a short query can't match
// stray letters scattered across an unrelated sentence.
func FuzzyScore(query, text string) (int, bool) {
	q := strings.ToLower(strings.TrimSpace(query))
	t := strings.ToLower(text)
	if q == "" {
		return 0, false
	}
	if idx := strings.Index(t, q); idx != -1 {
		runeIdx := utf8.RuneCountInString(t[:idx])
		return 10_000 - runeIdx, true
	}

	qRunes := []rune(q)
	tRunes := []rune(t)

	positions, ok := subsequencePositions(qRunes, tRunes, false)
	if !ok {
		return 0, false
	}
	if right, ok := subsequencePositions(qRunes, tRunes, true); ok {
		if (right[len(right)-1] - right[0]) < (positions[len(positions)-1] - positions[0]) {
			positions = right
		}
	}

	span := positions[len(positions)-1] - positions[0] + 1
	if span > len(qRunes)*3+2 {
		return 0, false
	}

	score := 0
	consecutive := 0
	prev := -1
	for i, idx := range positions {
		if i > 0 && idx == prev+1 {
			consecutive++
			score += 2 + consecutive
		} else {
			consecutive = 0
			score++
		}
		prev = idx
	}
	return score, true
}

// CardMatchesSearch fuzzy-matches query against title and description. A
// query that is a substring of the card's id also matches; ids are checked by
// plain substring because a short numeric query would otherwise
// subsequence-match unrelated ids.
func CardMatchesSearch(card Card, query string) bool {
	haystack := card.Title + " " + card.Description
	if _, matched := FuzzyScore(query, haystack); matched {
		return true
	}
	q := strings.ToLower(strings.TrimSpace(query))
	return q != "" && strings.Contains(strconv.FormatInt(card.ID, 10), q)
}
