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

	Send key.Binding

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

		Up:       key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("k/↑", "up")),
		Down:     key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("j/↓", "down")),
		OpenChat: key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open")),

		Top:       key.NewBinding(key.WithKeys("g", "home"), key.WithHelp("g", "top")),
		Bottom:    key.NewBinding(key.WithKeys("G", "end"), key.WithHelp("G", "bottom")),
		HalfDown:  key.NewBinding(key.WithKeys("ctrl+d"), key.WithHelp("ctrl+d", "half page down")),
		HalfUp:    key.NewBinding(key.WithKeys("ctrl+u"), key.WithHelp("ctrl+u", "half page up")),
		NextMedia: key.NewBinding(key.WithKeys("n", "]", "}"), key.WithHelp("n", "next media")),
		PrevMedia: key.NewBinding(key.WithKeys("p", "[", "{"), key.WithHelp("p", "previous media")),
		OpenMedia: key.NewBinding(key.WithKeys("o", "enter"), key.WithHelp("o", "open media")),

		Send: key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "send")),

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

func (k keyMap) sections() []keySection {
	return []keySection{
		{"Global", []key.Binding{k.NextPane, k.PrevPane, k.PaneLeft, k.PaneRight, k.Back, k.Compose, k.People, k.Filter, k.Help, k.Quit}},
		{"Chats", []key.Binding{k.Up, k.Down, k.OpenChat}},
		{"Messages", []key.Binding{k.Down, k.Up, k.Top, k.Bottom, k.HalfDown, k.HalfUp, k.NextMedia, k.PrevMedia, k.OpenMedia}},
		{"Compose", []key.Binding{k.Send, k.Back}},
	}
}

func (m Model) contextBindings() []key.Binding {
	k := m.keys
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
		return []key.Binding{k.Down, k.NextMedia, k.OpenMedia, k.Compose, k.PaneLeft, k.Help, k.Quit}
	default:
		return []key.Binding{k.Send, k.Back, k.NextPane}
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

func renderKeySections(sections []keySection) string {
	keyWidth := 0
	for _, section := range sections {
		for _, b := range section.bindings {
			keyWidth = max(keyWidth, lipgloss.Width(b.Help().Key))
		}
	}
	keyStyle := lipgloss.NewStyle().Foreground(accent).Width(keyWidth + 2)
	descStyle := lipgloss.NewStyle().Foreground(text)
	var out []string
	for i, section := range sections {
		if i > 0 {
			out = append(out, "")
		}
		out = append(out, titleStyle.Render(section.title))
		for _, b := range section.bindings {
			out = append(out, "  "+keyStyle.Render(b.Help().Key)+descStyle.Render(b.Help().Desc))
		}
	}
	return strings.Join(out, "\n")
}
