package tui

// wipLimits are the per-lane work-in-progress limits. Lanes absent from the
// map have no limit and keep the generic ">5 cards" overload warning.
var wipLimits = map[string]int{
	"ongoing": 6,
	"blocked": 2,
}
