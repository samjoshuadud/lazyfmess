package ui

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"
)

var (
	text   = lipgloss.AdaptiveColor{Light: "236", Dark: "252"}
	muted  = lipgloss.AdaptiveColor{Light: "243", Dark: "245"}
	accent = lipgloss.AdaptiveColor{Light: "31", Dark: "81"}
	danger = lipgloss.AdaptiveColor{Light: "160", Dark: "203"}
	line   = lipgloss.AdaptiveColor{Light: "250", Dark: "238"}

	titleStyle = lipgloss.NewStyle().
			Foreground(accent).
			Bold(true)

	statusStyle = lipgloss.NewStyle().
			Foreground(muted).
			Padding(0, 1)

	toastStyle = lipgloss.NewStyle().
			Foreground(text).
			Padding(0, 1)

	errorStatusStyle = lipgloss.NewStyle().
				Foreground(danger).
				Padding(0, 1)

	helpStyle = lipgloss.NewStyle().
			Foreground(muted).
			Padding(0, 1)

	modalStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(accent).
			Padding(1, 2)

	helpModalStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(line).
			Padding(1, 2)

	focusMark = lipgloss.NewStyle().Foreground(accent).Bold(true)

	timeStyle = lipgloss.NewStyle().Foreground(muted)

	pendingBodyStyle = lipgloss.NewStyle().Foreground(muted)

	errorBodyStyle = lipgloss.NewStyle().Foreground(danger)

	quoteStyle = lipgloss.NewStyle().Foreground(muted).Italic(true)
)

func senderPalette() []lipgloss.AdaptiveColor {
	return []lipgloss.AdaptiveColor{
		{Light: "29", Dark: "114"},
		{Light: "130", Dark: "180"},
		{Light: "127", Dark: "176"},
		{Light: "25", Dark: "110"},
		{Light: "166", Dark: "216"},
		{Light: "30", Dark: "151"},
	}
}

func newThreadList() list.Model {
	threads := list.New(nil, threadDelegate{}, 0, 0)
	threads.Title = "Inbox"
	threads.Styles.Title = titleStyle.Padding(0, 1)
	threads.Styles.FilterPrompt = lipgloss.NewStyle().Foreground(accent)
	threads.Styles.FilterCursor = lipgloss.NewStyle().Foreground(accent)
	threads.Styles.NoItems = lipgloss.NewStyle().Foreground(muted).Padding(0, 1)
	threads.SetShowTitle(false)
	threads.SetShowStatusBar(false)
	threads.SetShowHelp(false)
	threads.SetShowPagination(false)
	threads.SetFilteringEnabled(true)
	threads.DisableQuitKeybindings()
	return threads
}

func newComposer() textarea.Model {
	composer := textarea.New()
	composer.Placeholder = "Write a message"
	composer.ShowLineNumbers = false
	composer.Prompt = "> "
	composer.FocusedStyle.Prompt = lipgloss.NewStyle().Foreground(accent).Bold(true)
	composer.FocusedStyle.Text = lipgloss.NewStyle().Foreground(text)
	composer.FocusedStyle.Placeholder = lipgloss.NewStyle().Foreground(muted)
	composer.BlurredStyle.Prompt = lipgloss.NewStyle().Foreground(muted)
	composer.BlurredStyle.Text = lipgloss.NewStyle().Foreground(text)
	composer.BlurredStyle.Placeholder = lipgloss.NewStyle().Foreground(muted)
	composer.SetHeight(2)
	composer.CharLimit = 4000
	composer.KeyMap.InsertNewline = key.NewBinding(key.WithKeys("alt+enter", "ctrl+j"))
	return composer
}

func newPINInput() textinput.Model {
	input := textinput.New()
	input.Placeholder = "******"
	input.CharLimit = 6
	input.Width = 12
	input.EchoMode = textinput.EchoPassword
	input.EchoCharacter = '*'
	input.Prompt = "PIN> "
	input.PromptStyle = lipgloss.NewStyle().Foreground(accent).Bold(true)
	input.TextStyle = lipgloss.NewStyle().Foreground(text)
	input.PlaceholderStyle = lipgloss.NewStyle().Foreground(muted)
	input.Focus()
	return input
}
