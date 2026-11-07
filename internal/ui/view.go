package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"lazyfmess/internal/store"
)

func (m Model) View() string {
	if !m.ready {
		return statusStyle.Render("loading")
	}
	base := m.renderBase()
	if m.showPIN {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, m.renderPINModal())
	}
	if m.showSearch {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, m.renderSearchModal())
	}
	if m.showHelp {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, m.renderHelpModal())
	}
	return base
}

func (m Model) renderBase() string {
	leftTitle := "Inbox"
	if m.focus == focusList {
		leftTitle = focusMark.Render("●") + " Inbox"
	}
	m.list.Title = leftTitle

	left := sidebarStyle.
		Width(m.leftWidth).
		Height(m.height).
		Render(m.list.View())

	right := lipgloss.NewStyle().
		Width(m.rightWidth).
		Height(m.height).
		Render(m.renderConversation())

	return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
}

func (m Model) renderConversation() string {
	title := "No chat selected"
	if item, ok := m.list.SelectedItem().(threadItem); ok {
		title = item.thread.Title
		if title == "" {
			title = fmt.Sprintf("Chat %d", item.thread.ID)
		}
	}
	if m.focus == focusMessages {
		title = focusMark.Render("●") + " " + title
	}

	header := panelTitleStyle.Width(m.rightWidth).Render(truncate(title, m.rightWidth-2))
	composerLabel := "message"
	if m.focus == focusComposer {
		composerLabel = focusMark.Render("●") + " message"
	}
	composer := lipgloss.NewStyle().
		Width(m.rightWidth).
		Border(lipgloss.NormalBorder(), true, false, false, false).
		BorderForeground(line).
		Padding(0, 1).
		Render(composerLabel + "\n" + m.composer.View())

	statusText := m.status
	if m.toast != "" {
		statusText = m.toast
	}
	status := statusStyle.Width(m.rightWidth).Render(truncate(statusText, m.rightWidth-2))
	if strings.Contains(strings.ToLower(statusText), "error") || strings.Contains(strings.ToLower(statusText), "fail") {
		status = errorStatusStyle.Width(m.rightWidth).Render(truncate(statusText, m.rightWidth-2))
	}

	help := helpStyle.Width(m.rightWidth).Render(m.helpLine())

	return lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		m.viewport.View(),
		composer,
		status,
		help,
	)
}

func (m Model) helpLine() string {
	switch m.focus {
	case focusList:
		return "j/k move · enter open · / filter · s people · tab pane · i compose · ? help · q quit"
	case focusMessages:
		return "j/k scroll · i compose · h back · tab pane · ? help · q quit"
	default:
		return "enter send · esc back · tab pane · ? help"
	}
}

func (m Model) renderHelpModal() string {
	width := 56
	if m.width > 0 && m.width < width+4 {
		width = m.width - 4
	}
	body := strings.TrimSpace(`
Navigation
  j/k  ↑/↓     move or scroll
  h/l  ←/→     move between panes
  tab          cycle inbox → chat → compose
  enter        open chat / send message
  i a c        focus composer
  esc          back to inbox
  /            filter inbox
  s            search people
  ?            toggle this help
  q            quit
`)
	return helpModalStyle.Width(width).Render(
		lipgloss.JoinVertical(
			lipgloss.Left,
			titleStyle.Render("lazyfmess keys"),
			"",
			helpStyle.Render(body),
			"",
			helpStyle.Render("press ? or esc to close"),
		),
	)
}

func (m *Model) layout() {
	if m.width < 40 {
		m.width = 40
	}
	if m.height < 10 {
		m.height = 10
	}

	m.leftWidth = m.width / 3
	if m.leftWidth < 24 {
		m.leftWidth = 24
	}
	if m.leftWidth > 36 {
		m.leftWidth = 36
	}
	if m.leftWidth >= m.width-20 {
		m.leftWidth = m.width / 2
	}
	m.rightWidth = m.width - m.leftWidth
	if m.rightWidth < 20 {
		m.rightWidth = 20
	}

	const (
		headerLines   = 2
		composerLines = 4
		statusLines   = 1
		helpLines     = 1
	)
	viewHeight := m.height - headerLines - composerLines - statusLines - helpLines
	if viewHeight < 3 {
		viewHeight = 3
	}

	m.list.SetSize(m.leftWidth-1, m.height)
	m.composer.SetWidth(m.rightWidth - 4)
	m.viewport.Width = m.rightWidth
	m.viewport.Height = viewHeight
}

func truncate(s string, width int) string {
	if width <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= width {
		return s
	}
	if width <= 1 {
		return string(runes[:width])
	}
	return string(runes[:width-1]) + "…"
}

func renderMessages(messages []store.Message, width int) string {
	if len(messages) == 0 {
		return statusStyle.Render("No messages yet")
	}
	var b strings.Builder
	for _, msg := range messages {
		ts := msg.Timestamp.Local().Format("15:04")
		who := msg.SenderName
		if who == "" {
			who = fmt.Sprintf("%d", msg.SenderID)
		}
		prefix := fmt.Sprintf("%s %s  ", ts, who)
		body := msg.Text
		if msg.Pending {
			body += " …"
		}
		if msg.Error != "" {
			body += " [" + msg.Error + "]"
		}
		lineWidth := width - 2
		if lineWidth < 10 {
			lineWidth = 10
		}
		wrapped := wrapText(prefix, body, lineWidth)
		b.WriteString(wrapped)
		b.WriteByte('\n')
	}
	return b.String()
}

func wrapText(prefix, body string, width int) string {
	avail := width - lipgloss.Width(prefix)
	if avail < 8 {
		return truncate(prefix+body, width)
	}
	runes := []rune(body)
	if len(runes) == 0 {
		return strings.TrimRight(prefix, " ")
	}
	var lines []string
	for len(runes) > 0 {
		n := avail
		if n > len(runes) {
			n = len(runes)
		}
		chunk := string(runes[:n])
		if len(lines) == 0 {
			lines = append(lines, prefix+chunk)
		} else {
			pad := strings.Repeat(" ", lipgloss.Width(prefix))
			lines = append(lines, pad+chunk)
		}
		runes = runes[n:]
	}
	return strings.Join(lines, "\n")
}
