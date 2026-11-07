package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"

	"fmesstui/internal/store"
)

func (m Model) View() string {
	if !m.ready {
		return appStyle.Render("loading")
	}
	header := headerStyle.Width(m.width).Render(
		lipgloss.JoinHorizontal(
			lipgloss.Left,
			eyebrowStyle.Render("FMESSTUI"),
			"  ",
			lipgloss.NewStyle().Foreground(muted).Render("MESSAGES"),
		),
	)
	leftWidth := m.width / 3
	if leftWidth < 20 {
		leftWidth = 20
	}
	if leftWidth > m.width-20 {
		leftWidth = m.width / 2
	}
	rightWidth := m.width - leftWidth
	left := sidebarStyle.Width(leftWidth).Height(m.height - 1).Render(m.list.View())
	right := m.renderConversation(rightWidth, m.height-1)
	return appStyle.Width(m.width).Height(m.height).Render(
		lipgloss.JoinVertical(lipgloss.Left, header, lipgloss.JoinHorizontal(lipgloss.Top, left, right)),
	)
}

func (m Model) renderConversation(width, height int) string {
	title := "NO THREAD SELECTED"
	if item, ok := m.list.SelectedItem().(threadItem); ok {
		title = item.thread.Title
		if title == "" {
			title = fmt.Sprintf("THREAD %d", item.thread.ID)
		}
	}
	header := conversationHeaderStyle.Width(width).Render(
		lipgloss.JoinHorizontal(
			lipgloss.Left,
			lipgloss.NewStyle().Foreground(cyan).Render("●"),
			"  ",
			title,
		),
	)
	status := m.status
	if strings.Contains(status, "error") || strings.Contains(status, "failed") {
		status = errorStyle.Render(status)
	} else if status == "sending" || status == "sent" {
		status = statusActiveStyle.Render(status)
	} else {
		status = statusStyle.Render(status)
	}
	composer := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), true, false, false, false).
		BorderForeground(line).
		PaddingTop(1).
		Render(m.composer.View())
	footer := status
	if m.toast != "" {
		toastWidth := lipgloss.Width(m.toast) + 6
		if toastWidth > width-2 {
			toastWidth = width - 2
		}
		if toastWidth < 24 && width >= 24 {
			toastWidth = 24
		}
		footer = lipgloss.JoinVertical(lipgloss.Right, toastStyle.Width(toastWidth).Render(m.toast), status)
	}
	body := lipgloss.JoinVertical(lipgloss.Left, m.viewport.View(), composer, footer)
	return conversationStyle.Width(width).Height(height).Render(
		lipgloss.JoinVertical(lipgloss.Left, header, body),
	)
}

func (m *Model) layout() {
	leftWidth := m.width / 3
	if leftWidth < 20 {
		leftWidth = 20
	}
	if leftWidth > m.width-20 {
		leftWidth = m.width / 2
	}
	rightWidth := m.width - leftWidth
	if rightWidth < 1 {
		rightWidth = 1
	}
	composerHeight := 4
	footerHeight := 1
	if m.toast != "" {
		footerHeight += 3
	}
	listHeight := m.height - 1
	viewHeight := m.height - 1 - composerHeight - footerHeight - 2
	if viewHeight < 1 {
		viewHeight = 1
	}
	m.list.SetSize(leftWidth, listHeight)
	m.viewport = viewport.New(rightWidth, viewHeight)
	m.composer.SetWidth(rightWidth)
}

func renderMessages(messages []store.Message) string {
	if len(messages) == 0 {
		return "no messages"
	}
	var b strings.Builder
	for _, msg := range messages {
		ts := msg.Timestamp.Local().Format("3:04PM")
		who := msg.SenderName
		if who == "" {
			who = fmt.Sprintf("%d", msg.SenderID)
		}
		line := fmt.Sprintf("[%s] %s: %s", ts, who, msg.Text)
		if msg.Pending {
			line += " (pending)"
		}
		if msg.Error != "" {
			line += " error=" + msg.Error
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}
