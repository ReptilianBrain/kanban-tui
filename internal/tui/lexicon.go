package tui

import (
	"os"
	"strings"
)

// A skin renames what the board shows without touching what it stores:
// status values, CLI arguments and JSON output keep their plain names. The
// skin is picked once at startup from KANBANTUI_SKIN ("sprawl", the default,
// or "plain" for the original column names).
type skin struct {
	status map[string]string
	card   string // singular noun for a card, lower case
}

var skins = map[string]skin{
	"plain": {card: "card", status: map[string]string{
		"todo":      "Backlog",
		"ongoing":   "In Progress",
		"blocked":   "Blocked",
		"in_review": "In Review",
		"completed": "Completed",
	}},
	"sprawl": {card: "contract", status: map[string]string{
		"todo":      "Fixer",
		"ongoing":   "Jacked In",
		"blocked":   "Flatline",
		"in_review": "Dead Drop",
		"completed": "Jacked Out",
	}},
}

func activeSkin() skin {
	if s, ok := skins[os.Getenv("KANBANTUI_SKIN")]; ok {
		return s
	}
	return skins["sprawl"]
}

// cardNoun is the skin's word for a card in running text; cardNouns is its
// plural and cardTitle its capitalised form for headings.
func cardNoun() string  { return activeSkin().card }
func cardNouns() string { return activeSkin().card + "s" }
func cardTitle() string { return strings.ToUpper(activeSkin().card[:1]) + activeSkin().card[1:] }
