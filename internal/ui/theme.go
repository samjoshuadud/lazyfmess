package ui

import (
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"
)

var (
	text   = lipgloss.Color("252")
	muted  = lipgloss.Color("245")
	accent = lipgloss.Color("81")
	danger = lipgloss.Color("203")
	line   = lipgloss.Color("238")

	titleStyle = lipgloss.NewStyle().
			Foreground(accent).
			Bold(true)

	panelTitleStyle = lipgloss.NewStyle().
			Foreground(text).
			Bold(true).
			Padding(0, 1).
			Border(lipgloss.NormalBorder(), false, false, true, false).
			BorderForeground(line)

	sidebarStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder(), false, true, false, false).
			BorderForeground(line)

	statusStyle = lipgloss.NewStyle().
			Foreground(muted).
			Padding(0, 1)

	errorStatusStyle = lipgloss.NewStyle().
				Foreground(danger).
				Padding(0, 1)

	helpStyle = lipgloss.NewStyle().
			Foreground(muted).
			Padding(0, 1)
)

func newThreadList() list.Model {
	delegate := list.NewDefaultDelegate()
	delegate.SetHeight(2)
	delegate.SetSpacing(0)
	delegate.Styles.NormalTitle = lipgloss.NewStyle().Foreground(text).Padding(0, 1)
	delegate.Styles.NormalDesc = lipgloss.NewStyle().Foreground(muted).Padding(0, 1)
	delegate.Styles.SelectedTitle = lipgloss.NewStyle().
		Foreground(accent).
		Bold(true).
		Padding(0, 1)
	delegate.Styles.SelectedDesc = lipgloss.NewStyle().
		Foreground(text).
		Padding(0, 1)
	delegate.Styles.DimmedTitle = lipgloss.NewStyle().Foreground(muted).Padding(0, 1)
	delegate.Styles.DimmedDesc = lipgloss.NewStyle().Foreground(muted).Padding(0, 1)

	threads := list.New(nil, delegate, 0, 0)
	threads.Title = "Inbox"
	threads.Styles.Title = titleStyle.Padding(0, 1)
	threads.Styles.FilterPrompt = lipgloss.NewStyle().Foreground(accent)
	threads.Styles.FilterCursor = lipgloss.NewStyle().Foreground(accent)
	threads.Styles.NoItems = lipgloss.NewStyle().Foreground(muted).Padding(0, 1)
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
