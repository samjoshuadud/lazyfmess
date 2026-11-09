package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type keyMap struct {
	Quit      key.Binding
	ForceQuit key.Binding
	Help      key.Binding
	NextPane  key.Binding
	PrevPane  key.Binding
	PaneLeft  key.Binding
	PaneRight key.Binding
	Back      key.Binding
	Compose   key.Binding
	People    key.Binding
	Filter    key.Binding
	JumpChats key.Binding
	JumpChat  key.Binding
	JumpInput key.Binding

	Up       key.Binding
	Down     key.Binding
	OpenChat key.Binding

	Top       key.Binding
	Bottom    key.Binding
	HalfDown  key.Binding
	HalfUp    key.Binding
	NextMedia key.Binding
	PrevMedia key.Binding
	OpenMedia key.Binding
	Actions   key.Binding
	Copy      key.Binding
	Reply     key.Binding
	React     key.Binding
	Edit      key.Binding
	Unsend    key.Binding

	Send    key.Binding
	Newline key.Binding
	Editor  key.Binding

	Dismiss key.Binding
	Confirm key.Binding
	Cancel  key.Binding
	Select  key.Binding

	Close        key.Binding
	Unlock       key.Binding
	Skip         key.Binding
	SearchSubmit key.Binding
	SearchUp     key.Binding
	SearchDown   key.Binding
}

func newKeyMap() keyMap {
	return keyMap{
		Quit:      key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit")),
		ForceQuit: key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "quit")),
		Help:      key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "keys")),
		NextPane:  key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next pane")),
		PrevPane:  key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "previous pane")),
		PaneLeft:  key.NewBinding(key.WithKeys("h", "left"), key.WithHelp("h/←", "pane left")),
		PaneRight: key.NewBinding(key.WithKeys("l", "right"), key.WithHelp("l/→", "pane right")),
		Back:      key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		Compose:   key.NewBinding(key.WithKeys("i", "a", "c"), key.WithHelp("i", "compose")),
		People:    key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "people")),
		Filter:    key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
		JumpChats: key.NewBinding(key.WithKeys("1"), key.WithHelp("1", "chats")),
		JumpChat:  key.NewBinding(key.WithKeys("2"), key.WithHelp("2", "messages")),
		JumpInput: key.NewBinding(key.WithKeys("3"), key.WithHelp("3", "compose")),

		Up:       key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("k/↑", "up")),
		Down:     key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("j/↓", "down")),
		OpenChat: key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open")),

		Top:       key.NewBinding(key.WithKeys("g", "home"), key.WithHelp("g", "top")),
		Bottom:    key.NewBinding(key.WithKeys("G", "end"), key.WithHelp("G", "bottom")),
		HalfDown:  key.NewBinding(key.WithKeys("ctrl+d"), key.WithHelp("ctrl+d", "half page down")),
		HalfUp:    key.NewBinding(key.WithKeys("ctrl+u"), key.WithHelp("ctrl+u", "half page up")),
		NextMedia: key.NewBinding(key.WithKeys("n", "]", "}"), key.WithHelp("n", "next media")),
		PrevMedia: key.NewBinding(key.WithKeys("p", "[", "{"), key.WithHelp("p", "previous media")),
		OpenMedia: key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "open media")),
		Actions:   key.NewBinding(key.WithKeys("enter", "x"), key.WithHelp("enter/x", "actions")),
		Copy:      key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "copy text")),
		Reply:     key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "reply")),
		React:     key.NewBinding(key.WithKeys("+"), key.WithHelp("+", "react")),
		Edit:      key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "edit own")),
		Unsend:    key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "unsend own")),

		Send:    key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "send")),
		Newline: key.NewBinding(key.WithKeys("alt+enter", "ctrl+j"), key.WithHelp("alt+enter", "newline")),
		Editor:  key.NewBinding(key.WithKeys("ctrl+e"), key.WithHelp("ctrl+e", "edit in $EDITOR")),

		Dismiss: key.NewBinding(key.WithKeys("enter", "esc"), key.WithHelp("enter", "dismiss")),
		Confirm: key.NewBinding(key.WithKeys("enter", "y"), key.WithHelp("enter/y", "confirm")),
		Cancel:  key.NewBinding(key.WithKeys("esc", "n"), key.WithHelp("esc", "cancel")),
		Select:  key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "select")),

		Close:        key.NewBinding(key.WithKeys("esc", "?", "q", "enter", " "), key.WithHelp("esc", "close")),
		Unlock:       key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "unlock")),
		Skip:         key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "skip")),
		SearchSubmit: key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "search/open")),
		SearchUp:     key.NewBinding(key.WithKeys("up", "ctrl+p"), key.WithHelp("↑", "previous")),
		SearchDown:   key.NewBinding(key.WithKeys("down", "ctrl+n"), key.WithHelp("↓", "next")),
	}
}

type keySection struct {
	title    string
	bindings []key.Binding
}

func (k keyMap) sections() [][]keySection {
	return [][]keySection{
		{
			{"Global", []key.Binding{k.JumpChats, k.JumpChat, k.JumpInput, k.NextPane, k.PrevPane, k.PaneLeft, k.PaneRight, k.Back, k.Compose, k.People, k.Filter, k.Help, k.Quit}},
		},
		{
			{"Chats", []key.Binding{k.Up, k.Down, k.OpenChat}},
			{"Compose", []key.Binding{k.Send, k.Newline, k.Editor, k.Back}},
		},
		{
			{"Messages", []key.Binding{k.Down, k.Up, k.Top, k.Bottom, k.HalfDown, k.HalfUp, k.Actions, k.Reply, k.React, k.Edit, k.Unsend, k.Copy, k.NextMedia, k.PrevMedia, k.OpenMedia}},
		},
	}
}

func (m Model) contextBindings() []key.Binding {
	k := m.keys
	if m.popup != nil {
		switch m.popup.kind {
		case popupError:
			return []key.Binding{k.Dismiss}
		case popupConfirm:
			return []key.Binding{k.Confirm, k.Cancel}
		default:
			return []key.Binding{k.Select, k.Down, k.Up, k.Cancel}
		}
	}
	switch {
	case m.showPIN:
		return []key.Binding{k.Unlock, k.Skip}
	case m.showSearch:
		return []key.Binding{k.SearchSubmit, k.SearchUp, k.SearchDown, k.Skip}
	case m.showHelp:
		return []key.Binding{k.Close}
	}
	switch m.focus {
	case focusList:
		return []key.Binding{k.OpenChat, k.Filter, k.People, k.Compose, k.NextPane, k.Help, k.Quit}
	case focusMessages:
		return []key.Binding{k.Actions, k.Reply, k.React, k.Copy, k.OpenMedia, k.Compose, k.Help, k.Quit}
	default:
		return []key.Binding{k.Send, k.Newline, k.Editor, k.Back}
	}
}

func renderHints(bindings []key.Binding, width int) string {
	h := help.New()
	h.Width = width
	h.ShortSeparator = " · "
	h.Styles.ShortKey = lipgloss.NewStyle().Foreground(accent)
	h.Styles.ShortDesc = lipgloss.NewStyle().Foreground(muted)
	h.Styles.ShortSeparator = lipgloss.NewStyle().Foreground(line)
	h.Styles.Ellipsis = lipgloss.NewStyle().Foreground(muted)
	return ansi.Truncate(h.ShortHelpView(bindings), width, "…")
}

func renderKeySections(columns [][]keySection) string {
	keyStyle := lipgloss.NewStyle().Foreground(accent).PaddingRight(2)
	descStyle := lipgloss.NewStyle().Foreground(text).PaddingRight(4)
	rendered := make([]string, 0, len(columns))
	for _, column := range columns {
		var keys, descs []string
		for i, section := range column {
			if i > 0 {
				keys, descs = append(keys, ""), append(descs, "")
			}
			keys, descs = append(keys, titleStyle.Render(section.title)), append(descs, "")
			for _, b := range section.bindings {
				keys = append(keys, "  "+b.Help().Key)
				descs = append(descs, b.Help().Desc)
			}
		}
		rendered = append(rendered, lipgloss.JoinHorizontal(
			lipgloss.Top,
			keyStyle.Render(strings.Join(keys, "\n")),
			descStyle.Render(strings.Join(descs, "\n")),
		))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, rendered...)
}
