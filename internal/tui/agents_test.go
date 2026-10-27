package tui

import (
	"errors"
	"strings"
	"testing"

	"github.com/ReptilianBrain/kanban-tui/internal/db"
)

func TestAgentsMsg_UpdatesKeepsAndClears(t *testing.T) {
	m := newTestModel(t)

	m.Update(agentsMsg{states: map[int64]string{7: "working"}})
	if got := m.agents[7]; got != "working" {
		t.Fatalf("agents[7] = %q, want working", got)
	}

	// A failed poll must not wipe the last known states.
	m.Update(agentsMsg{err: errors.New("herdr hiccup")})
	if got := m.agents[7]; got != "working" {
		t.Errorf("after error agents[7] = %q, want working kept", got)
	}

	// herdr gone for good: states cleared.
	m.Update(agentsMsg{disabled: true})
	if len(m.agents) != 0 {
		t.Errorf("after disabled agents = %v, want empty", m.agents)
	}
}

func TestCardBlockLines_AgentState(t *testing.T) {
	card := db.Card{ID: 1, Title: "t", Status: "ongoing", Priority: "medium"}
	for state, want := range map[string]string{
		"working": "working",
		"blocked": "waiting",
		"idle":    "idle",
		"done":    "done",
	} {
		got := strings.Join(cardBlockLines(card, state), "\n")
		if !strings.Contains(got, want) {
			t.Errorf("state %q: card lines %q lack %q", state, got, want)
		}
	}
	for _, state := range []string{"", "unknown"} {
		if got := strings.Join(cardBlockLines(card, state), "\n"); len(cardBlockLines(card, state)) != 1 {
			t.Errorf("state %q should add no meta line, got %q", state, got)
		}
	}
}

func TestSkin(t *testing.T) {
	t.Setenv("KANBANTUI_SKIN", "plain")
	if got := activeSkin().status["ongoing"]; got != "In Progress" {
		t.Errorf("plain ongoing = %q, want In Progress", got)
	}
	t.Setenv("KANBANTUI_SKIN", "sprawl")
	if got := activeSkin().status["blocked"]; got != "Flatline" {
		t.Errorf("sprawl blocked = %q, want Flatline", got)
	}
	t.Setenv("KANBANTUI_SKIN", "nonsense")
	if got := activeSkin().status["todo"]; got != "Fixer" {
		t.Errorf("unknown skin todo = %q, want the sprawl default Fixer", got)
	}
	for name, s := range skins {
		for _, status := range statuses {
			if s.status[status] == "" {
				t.Errorf("skin %q has no label for %q", name, status)
			}
		}
	}
}

func TestPopup_EscQAndF15Close(t *testing.T) {
	quits := func(m *Model, k string) bool {
		_, cmd := m.Update(key(k))
		return m.quitting && cmd != nil
	}

	m := newTestModel(t)
	m.popup = true
	if !quits(m, "esc") {
		t.Error("popup: esc at the bare board should quit")
	}
	for _, k := range []string{"q", "f15"} {
		m = newTestModel(t)
		m.popup = true
		if !quits(m, k) {
			t.Errorf("popup: %s should quit", k)
		}
	}

	// esc still clears a filter first, and only the next esc closes
	m = newTestModel(t)
	m.popup = true
	m.labelFilter = "Data"
	if quits(m, "esc") {
		t.Error("popup: esc with a filter active should clear it, not quit")
	}
	if m.labelFilter != "" {
		t.Error("popup: esc should have cleared the filter")
	}
	if !quits(m, "esc") {
		t.Error("popup: second esc should quit")
	}

	// F15 closes from inside a modal too
	m = newTestModel(t)
	m.popup = true
	send(m, "m")
	if !quits(m, "f15") {
		t.Error("popup: f15 should quit from inside the Matrix")
	}

	// outside a popup: esc and f15 do nothing, q still quits
	m = newTestModel(t)
	if quits(m, "esc") || quits(m, "f15") {
		t.Error("not a popup: esc and f15 must not quit")
	}
	if !quits(m, "q") {
		t.Error("not a popup: q should still quit")
	}
}

func TestPopup_FooterAdvertisesTheCloseKeys(t *testing.T) {
	m := newTestModel(t)
	m.popup = true
	if v := m.View(); !strings.Contains(v, "Esc/q/F15:Close") {
		t.Error("popup footer should say Esc/q/F15:Close")
	}
	m.popup = false
	if v := m.View(); strings.Contains(v, "F15") {
		t.Error("non-popup footer should not mention F15")
	}
}
