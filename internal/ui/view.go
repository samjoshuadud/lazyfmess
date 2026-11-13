package ui

import (
	"fmt"
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
	if m.login != nil {
		return m.renderLogin()
	}
	view := m.renderBase()
	switch {
	case m.showPIN:
		view = overlay(view, m.renderPINModal(), m.width, m.height)
	case m.showSearch:
		view = overlay(view, m.renderSearchModal(), m.width, m.height)
	case m.showHelp:
		view = overlay(view, m.renderHelpModal(), m.width, m.height)
	}
	if m.popup != nil {
		view = overlay(view, m.renderPopup(), m.width, m.height)
	}
	return view
}

func (m Model) renderBase() string {
	bodyH := m.height - 1
	chatsTitle := "[1] Chats"
	if filter := m.list.FilterValue(); filter != "" {
		chatsTitle += " /" + filter
	}
	chats := m.list.View()
	if len(m.list.Items()) == 0 {
		placeholder := "No chats yet"
		if !m.threadsLoaded || m.loading() {
			placeholder = "Loading chats…"
		}
		chats = "\n" + statusStyle.Render(placeholder)
	}
	left := frame(chatsTitle, m.focus == focusList, m.leftWidth, bodyH, chats)
	messages := frame("[2] "+m.conversationTitle(), m.focus == focusMessages, m.rightWidth, bodyH-composerFrameHeight, m.viewport.View())
	composeTitle := "[3] Compose"
	switch {
	case m.editing.ID != "":
		composeTitle = "[3] Edit message (esc cancels)"
	case m.replyTo.ID != "":
		composeTitle = "[3] Reply to " + replyName(m.replyTo, m.selfAccountID()) + " (esc cancels)"
	}
	if label := m.typingLabel(); label != "" {
		composeTitle += " · " + label
	}
	composer := frame(composeTitle, m.focus == focusComposer, m.rightWidth, composerFrameHeight, m.composer.View())
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
	switch {
	case m.connecting:
		text = "connecting to Messenger… · " + text
	case m.syncing && m.syncTotal > 0:
		text = fmt.Sprintf("syncing history %d/%d · %s", m.syncDone, m.syncTotal, text)
	case m.syncing:
		text = "syncing history… · " + text
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
