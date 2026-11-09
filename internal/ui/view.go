package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"lazyfmess/internal/store"
)

const composerFrameHeight = 4

func (m Model) View() string {
	if !m.ready {
		return statusStyle.Render("loading")
	}
	base := m.renderBase()
	switch {
	case m.showPIN:
		return overlay(base, m.renderPINModal(), m.width, m.height)
	case m.showSearch:
		return overlay(base, m.renderSearchModal(), m.width, m.height)
	case m.showHelp:
		return overlay(base, m.renderHelpModal(), m.width, m.height)
	}
	return base
}

func (m Model) renderBase() string {
	bodyH := m.height - 1
	chatsTitle := "[1] Chats"
	if filter := m.list.FilterValue(); filter != "" {
		chatsTitle += " /" + filter
	}
	left := frame(chatsTitle, m.focus == focusList, m.leftWidth, bodyH, m.list.View())
	messages := frame("[2] "+m.conversationTitle(), m.focus == focusMessages, m.rightWidth, bodyH-composerFrameHeight, m.viewport.View())
	composer := frame("[3] Compose", m.focus == focusComposer, m.rightWidth, composerFrameHeight, m.composer.View())
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, messages+"\n"+composer)
	return body + "\n" + m.renderStatusBar()
}

func (m Model) conversationTitle() string {
	if t, ok := m.currentThread(); ok {
		return t.Title
	}
	if m.selectedID != 0 {
		return threadTitle(store.Thread{ID: m.selectedID})
	}
	return "Messages"
}

func (m Model) renderStatusBar() string {
	text := m.status
	style := statusStyle
	if m.toast != "" {
		text = m.toast
		style = toastStyle
	}
	if strings.Contains(strings.ToLower(text), "error") || strings.Contains(strings.ToLower(text), "fail") {
		style = errorStatusStyle
	}
	right := ansi.Truncate(text, m.width/2, "…")
	if m.inFlight > 0 {
		right = m.spinner.View() + " " + right
	}
	right = style.Render(right)
	hintsW := max(m.width-lipgloss.Width(right)-2, 0)
	hints := renderHints(m.contextBindings(), hintsW)
	gap := max(m.width-1-lipgloss.Width(hints)-lipgloss.Width(right), 1)
	return " " + hints + strings.Repeat(" ", gap) + right
}

func (m Model) renderHelpModal() string {
	return helpModalStyle.Render(
		lipgloss.JoinVertical(
			lipgloss.Left,
			titleStyle.Render("lazyfmess keys"),
			"",
			renderKeySections(m.keys.sections()),
		),
	)
}

func (m *Model) layout() {
	m.width = max(m.width, 40)
	m.height = max(m.height, 12)

	m.leftWidth = min(max(m.width/3, 24), 36)
	if m.leftWidth >= m.width-20 {
		m.leftWidth = m.width / 2
	}
	m.rightWidth = m.width - m.leftWidth

	bodyH := m.height - 1
	m.list.SetSize(m.leftWidth-2, bodyH-2)
	m.composer.SetWidth(m.rightWidth - 2)
	m.viewport.Width = m.rightWidth - 2
	m.viewport.Height = max(bodyH-composerFrameHeight-2, 1)
}
