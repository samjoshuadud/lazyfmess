package ui

import (
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/lipgloss"
)

var (
	text   = lipgloss.AdaptiveColor{Light: "#111827", Dark: "#E5E7EB"}
	muted  = lipgloss.AdaptiveColor{Light: "#6B7280", Dark: "#9CA3AF"}
	cyan   = lipgloss.AdaptiveColor{Light: "#0E7490", Dark: "#67E8F9"}
	coral  = lipgloss.AdaptiveColor{Light: "#B91C1C", Dark: "#FCA5A5"}
	line   = lipgloss.AdaptiveColor{Light: "#D1D5DB", Dark: "#374151"}
	strong = lipgloss.AdaptiveColor{Light: "#0F172A", Dark: "#F9FAFB"}

	appStyle = lipgloss.NewStyle().
			Foreground(text)
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Padding(0, 1).
			Border(lipgloss.NormalBorder(), false, false, true, false).
			BorderForeground(line)
	eyebrowStyle = lipgloss.NewStyle().
			Foreground(cyan).
			Bold(true)
	sidebarStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder(), false, true, false, false).
			BorderForeground(line)
	conversationStyle = lipgloss.NewStyle().
				PaddingLeft(1)
	conversationHeaderStyle = lipgloss.NewStyle().
				Foreground(strong).
				Bold(true).
				Padding(0, 1).
				Border(lipgloss.NormalBorder(), false, false, true, false).
				BorderForeground(line)
	statusStyle = lipgloss.NewStyle().
			Foreground(muted).
			PaddingLeft(1)
	statusActiveStyle = lipgloss.NewStyle().
				Foreground(cyan).
				Bold(true)
	errorStyle = lipgloss.NewStyle().
			Foreground(coral)
	toastStyle = lipgloss.NewStyle().
			Foreground(coral).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(coral).
			Padding(0, 1)
)

func newThreadList() list.Model {
	delegate := list.NewDefaultDelegate()
	delegate.SetHeight(2)
	delegate.SetSpacing(0)
	delegate.Styles.NormalTitle = lipgloss.NewStyle().
		Foreground(strong).
		Padding(0, 1)
	delegate.Styles.NormalDesc = lipgloss.NewStyle().
		Foreground(muted).
		Padding(0, 1)
	delegate.Styles.SelectedTitle = lipgloss.NewStyle().
		Foreground(strong).
		Border(lipgloss.NormalBorder(), false, false, false, true).
		BorderForeground(cyan).
		Bold(true).
		Padding(0, 1)
	delegate.Styles.SelectedDesc = lipgloss.NewStyle().
		Foreground(text).
		Border(lipgloss.NormalBorder(), false, false, false, true).
		BorderForeground(cyan).
		Padding(0, 1)
	delegate.Styles.DimmedTitle = lipgloss.NewStyle().
		Foreground(muted).
		Padding(0, 1)
	delegate.Styles.DimmedDesc = lipgloss.NewStyle().
		Foreground(muted).
		Padding(0, 1)
	threads := list.New(nil, delegate, 0, 0)
	threads.Title = "INBOX"
	threads.Styles.Title = lipgloss.NewStyle().
		Foreground(cyan).
		Bold(true).
		Padding(0, 1)
	threads.Styles.FilterPrompt = lipgloss.NewStyle().Foreground(cyan)
	threads.Styles.FilterCursor = lipgloss.NewStyle().Foreground(cyan)
	threads.Styles.StatusBar = lipgloss.NewStyle().Foreground(muted).Padding(0, 1)
	threads.Styles.HelpStyle = lipgloss.NewStyle().Foreground(muted)
	threads.Styles.NoItems = lipgloss.NewStyle().Foreground(muted).Padding(0, 1)
	threads.SetShowStatusBar(false)
	threads.SetFilteringEnabled(true)
	threads.DisableQuitKeybindings()
	return threads
}

func newComposer() textarea.Model {
	composer := textarea.New()
	composer.Placeholder = "message"
	composer.ShowLineNumbers = false
	composer.Prompt = "› "
	composer.FocusedStyle.Prompt = lipgloss.NewStyle().Foreground(cyan).Bold(true)
	composer.FocusedStyle.Text = lipgloss.NewStyle().Foreground(text)
	composer.FocusedStyle.Placeholder = lipgloss.NewStyle().Foreground(muted)
	composer.BlurredStyle.Prompt = lipgloss.NewStyle().Foreground(muted)
	composer.BlurredStyle.Text = lipgloss.NewStyle().Foreground(text)
	composer.BlurredStyle.Placeholder = lipgloss.NewStyle().Foreground(muted)
	composer.SetHeight(2)
	composer.Focus()
	return composer
}
