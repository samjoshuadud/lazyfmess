package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type popupKind int

const (
	popupError popupKind = iota
	popupConfirm
	popupMenu
	popupPrompt
)

type menuItem struct {
	label  string
	action func(*Model) tea.Cmd
}

type popup struct {
	kind      popupKind
	title     string
	body      string
	items     []menuItem
	index     int
	onConfirm func(*Model) tea.Cmd
	input     textinput.Model
	onSubmit  func(*Model, string) tea.Cmd
}

type errorMsg struct {
	text string
}

func (m *Model) showError(text string) {
	m.popup = &popup{kind: popupError, title: "Error", body: text}
}

func (m *Model) showConfirm(title, body string, onConfirm func(*Model) tea.Cmd) {
	m.popup = &popup{kind: popupConfirm, title: title, body: body, onConfirm: onConfirm}
}

func (m *Model) showMenu(title string, items []menuItem) {
	m.popup = &popup{kind: popupMenu, title: title, items: items}
}

func (m *Model) showPrompt(title, placeholder string, onSubmit func(*Model, string) tea.Cmd) tea.Cmd {
	input := textinput.New()
	input.Placeholder = placeholder
	input.Prompt = "> "
	input.PromptStyle = lipgloss.NewStyle().Foreground(accent).Bold(true)
	input.TextStyle = lipgloss.NewStyle().Foreground(text)
	input.PlaceholderStyle = lipgloss.NewStyle().Foreground(muted)
	input.Width = min(56, m.width-12)
	input.Focus()
	m.popup = &popup{kind: popupPrompt, title: title, input: input, onSubmit: onSubmit}
	return textinput.Blink
}

func (m Model) updatePopup(msg tea.Msg) (Model, tea.Cmd) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		if m.popup.kind == popupPrompt {
			var cmd tea.Cmd
			m.popup.input, cmd = m.popup.input.Update(msg)
			return m, cmd
		}
		return m, nil
	}
	keys := m.keys
	p := m.popup
	if key.Matches(k, keys.ForceQuit) {
		return m, tea.Quit
	}
	switch p.kind {
	case popupError:
		if key.Matches(k, keys.Dismiss) {
			m.popup = nil
		}
	case popupConfirm:
		switch {
		case key.Matches(k, keys.Confirm):
			m.popup = nil
			return m, p.onConfirm(&m)
		case key.Matches(k, keys.Cancel):
			m.popup = nil
		}
	case popupPrompt:
		switch {
		case key.Matches(k, keys.Select):
			m.popup = nil
			return m, p.onSubmit(&m, strings.TrimSpace(p.input.Value()))
		case key.Matches(k, keys.Skip):
			m.popup = nil
			return m, nil
		}
		var cmd tea.Cmd
		p.input, cmd = p.input.Update(k)
		return m, cmd
	case popupMenu:
		switch {
		case key.Matches(k, keys.Up):
			p.index = max(p.index-1, 0)
		case key.Matches(k, keys.Down):
			p.index = min(p.index+1, len(p.items)-1)
		case key.Matches(k, keys.Select):
			m.popup = nil
			return m, p.items[p.index].action(&m)
		case key.Matches(k, keys.Cancel):
			m.popup = nil
		}
	}
	return m, nil
}

func (m Model) renderPopup() string {
	p := m.popup
	width := min(64, m.width-4)
	border := accent
	if p.kind == popupError {
		border = danger
	}
	var body string
	switch p.kind {
	case popupPrompt:
		body = p.input.View()
	case popupMenu:
		rows := make([]string, len(p.items))
		for i, item := range p.items {
			label := ansi.Truncate(item.label, width-6, "…")
			if i == p.index {
				rows[i] = focusMark.Render("> " + label)
			} else {
				rows[i] = "  " + label
			}
		}
		body = strings.Join(rows, "\n")
	default:
		body = lipgloss.NewStyle().Width(width - 4).Foreground(text).Render(p.body)
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(border).
		Background(panel).
		Padding(1, 2).
		Width(width).
		Render(lipgloss.JoinVertical(
			lipgloss.Left,
			lipgloss.NewStyle().Foreground(border).Bold(true).Render(p.title),
			"",
			body,
		))
}
