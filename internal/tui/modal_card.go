package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/ReptilianBrain/kanban-tui/internal/db"
)

// cardFormResult is the card form's result; the done callback gets nil on cancel.
type cardFormResult struct {
	Title       string
	Description string
	Priority    string
	DueDate     *string
	Label       *string
}

const (
	cardFocusTitle = iota
	cardFocusPriority
	cardFocusDue
	cardFocusLabel
	cardFocusDesc
	cardFocusCount
)

// cardModal is the create/edit card form. bubbles has no select widget, so
// priority and label are left/right cyclers.
type cardModal struct {
	heading string
	cardID  *int64

	title textinput.Model
	due   textinput.Model
	desc  textarea.Model

	labels      []db.Label
	priorityIdx int
	labelIdx    int // 0 = "(none)", else labels[labelIdx-1]

	focus  int
	errMsg string

	done func(m *Model, result *cardFormResult) tea.Cmd
}

// newCardModal builds the form. A non-empty label pre-selects it (label-filter
// pre-fill and note promotion); a non-nil cardID means edit rather than create.
func newCardModal(store *db.DB, title, description, priority, dueDate string, label *string, cardID *int64, done func(m *Model, result *cardFormResult) tea.Cmd) *cardModal {
	labels, _ := store.GetLabels()

	ti := textinput.New()
	ti.Placeholder = "Card title…"
	styleInput(&ti, colorSurface)
	ti.SetValue(title)

	di := textinput.New()
	di.Placeholder = "2026-05-26"
	styleInput(&di, colorSurface)
	di.SetValue(dueDate)

	ta := textarea.New()
	styleTextarea(&ta)
	ta.SetValue(description)

	priorityIdx := 1 // medium
	for i, p := range priorityOptions {
		if p == priority {
			priorityIdx = i
		}
	}

	labelIdx := 0
	if label != nil && *label != "" {
		for i, l := range labels {
			if l.Name == *label {
				labelIdx = i + 1
			}
		}
	}

	heading := "New Card"
	if cardID != nil {
		heading = "Edit Card"
	}

	c := &cardModal{
		heading:     heading,
		cardID:      cardID,
		title:       ti,
		due:         di,
		desc:        ta,
		labels:      labels,
		priorityIdx: priorityIdx,
		labelIdx:    labelIdx,
		focus:       cardFocusTitle,
		done:        done,
	}
	return c
}

func (c *cardModal) Init(m *Model) tea.Cmd {
	return c.title.Focus()
}

func (c *cardModal) setFocus(f int) tea.Cmd {
	c.title.Blur()
	c.due.Blur()
	c.desc.Blur()
	c.focus = f
	switch f {
	case cardFocusTitle:
		return c.title.Focus()
	case cardFocusDue:
		return c.due.Focus()
	case cardFocusDesc:
		return c.desc.Focus()
	}
	return nil
}

func (c *cardModal) cancel(m *Model) tea.Cmd {
	m.popModal()
	return c.done(m, nil)
}

func (c *cardModal) save(m *Model) tea.Cmd {
	title := strings.TrimSpace(c.title.Value())
	if title == "" {
		c.errMsg = "Title is required"
		return c.setFocus(cardFocusTitle)
	}
	dueRaw := strings.TrimSpace(c.due.Value())
	var dueDate *string
	if dueRaw != "" {
		if _, err := parseDate(dueRaw); err != nil {
			c.errMsg = "Due date must be YYYY-MM-DD"
			return c.setFocus(cardFocusDue)
		}
		dueDate = &dueRaw
	}

	var label *string
	if c.labelIdx > 0 && c.labelIdx-1 < len(c.labels) {
		name := c.labels[c.labelIdx-1].Name
		label = &name
	}

	result := &cardFormResult{
		Title:       title,
		Description: c.desc.Value(),
		Priority:    priorityOptions[c.priorityIdx],
		DueDate:     dueDate,
		Label:       label,
	}
	m.popModal()
	return c.done(m, result)
}

func (c *cardModal) Update(msg tea.Msg, m *Model) tea.Cmd {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		// Non-key messages (cursor blink, paste) go to the focused widget.
		var cmd tea.Cmd
		switch c.focus {
		case cardFocusTitle:
			c.title, cmd = c.title.Update(msg)
		case cardFocusDue:
			c.due, cmd = c.due.Update(msg)
		case cardFocusDesc:
			c.desc, cmd = c.desc.Update(msg)
		}
		return cmd
	}

	switch keyMsg.String() {
	case "esc":
		// Escape cancels even while the textarea holds focus.
		return c.cancel(m)
	case "ctrl+s":
		return c.save(m)
	case "tab":
		return c.setFocus((c.focus + 1) % cardFocusCount)
	case "shift+tab":
		return c.setFocus((c.focus - 1 + cardFocusCount) % cardFocusCount)
	}

	switch c.focus {
	case cardFocusTitle:
		var cmd tea.Cmd
		c.title, cmd = c.title.Update(keyMsg)
		return cmd
	case cardFocusDue:
		var cmd tea.Cmd
		c.due, cmd = c.due.Update(keyMsg)
		return cmd
	case cardFocusDesc:
		var cmd tea.Cmd
		c.desc, cmd = c.desc.Update(keyMsg)
		return cmd
	case cardFocusPriority:
		switch keyMsg.String() {
		case "left", "h":
			c.priorityIdx = (c.priorityIdx - 1 + len(priorityOptions)) % len(priorityOptions)
		case "right", "l":
			c.priorityIdx = (c.priorityIdx + 1) % len(priorityOptions)
		}
	case cardFocusLabel:
		n := len(c.labels) + 1
		switch keyMsg.String() {
		case "left", "h":
			c.labelIdx = (c.labelIdx - 1 + n) % n
		case "right", "l":
			c.labelIdx = (c.labelIdx + 1) % n
		}
	}
	return nil
}

func fieldLine(fieldLabel string, focused bool, value string) string {
	lbl := textStyle.Render(fieldLabel)
	if focused {
		lbl = accentBold.Render("▸ " + fieldLabel)
	} else {
		lbl = textStyle.Render("  " + fieldLabel)
	}
	return lbl + "\n" + value
}

func (c *cardModal) labelName() string {
	if c.labelIdx == 0 {
		return "(none)"
	}
	if c.labelIdx-1 < len(c.labels) {
		return c.labels[c.labelIdx-1].Name
	}
	return "(none)"
}

func (c *cardModal) View(m *Model) string {
	var b strings.Builder
	b.WriteString(accentBold.Render(c.heading))
	b.WriteString("\n\n")

	b.WriteString(fieldLine("Title", c.focus == cardFocusTitle, c.title.View()))
	b.WriteString("\n\n")

	priorityVal := pickerValue(priorityOptions[c.priorityIdx], c.focus == cardFocusPriority)
	b.WriteString(fieldLine("Priority  (←/→ to change)", c.focus == cardFocusPriority, priorityVal))
	b.WriteString("\n\n")

	b.WriteString(fieldLine("Due date  (YYYY-MM-DD, optional)", c.focus == cardFocusDue, c.due.View()))
	b.WriteString("\n\n")

	labelVal := pickerValue(c.labelName(), c.focus == cardFocusLabel)
	b.WriteString(fieldLine("Label  (←/→ to change)", c.focus == cardFocusLabel, labelVal))
	b.WriteString("\n\n")

	b.WriteString(fieldLine("Description", c.focus == cardFocusDesc, c.desc.View()))

	if c.errMsg != "" {
		b.WriteString("\n\n")
		b.WriteString(errMsgStyle.Render(c.errMsg))
	}

	b.WriteString("\n\n")
	b.WriteString(modalHintStyle.Render("Tab: next field   Ctrl+S: save   Esc: cancel"))

	return renderModal(modalBoxStyle, 64, b.String())
}
