package ui

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/charmbracelet/lipgloss"

	"lazyfmess/internal/store"
)

type threadsLoadedMsg struct {
	threads []store.Thread
	err     error
}

type messagesLoadedMsg struct {
	threadID int64
	messages []store.Message
	err      error
}

type sendFinishedMsg struct {
	pendingID string
	threadID  int64
	result    SendResult
	text      string
	err       error
}

type RefreshMsg struct{}

type toastExpiredMsg struct {
	message string
}

type toastMsg struct {
	message string
}

func renderMessages(messages []store.Message, width int, selfID int64) string {
	if width <= 0 {
		width = 40
	}
	if len(messages) == 0 {
		return statusStyle.Render("No messages yet")
	}

	inner := width - 2
	if inner < 16 {
		inner = 16
	}
	bubbleMax := inner * 3 / 4
	if bubbleMax < 12 {
		bubbleMax = 12
	}
	if bubbleMax > inner {
		bubbleMax = inner
	}

	var b strings.Builder
	var prevSender int64
	var prevTime time.Time
	first := true

	for _, msg := range messages {
		body := strings.TrimSpace(msg.Text)
		if body == "" && !msg.Pending && msg.Error == "" {
			continue
		}
		if msg.Pending {
			body = strings.TrimSpace(body + " …")
		}
		if msg.Error != "" {
			if body == "" {
				body = msg.Error
			} else {
				body += "\n" + msg.Error
			}
		}

		mine := selfID != 0 && msg.SenderID == selfID
		sameSender := !first && msg.SenderID == prevSender
		closeInTime := sameSender && !prevTime.IsZero() && msg.Timestamp.Sub(prevTime) < 5*time.Minute
		showHeader := !closeInTime

		if !first {
			if showHeader {
				b.WriteString("\n\n")
			} else {
				b.WriteByte('\n')
			}
		}

		block := renderMessageBlock(msg, body, mine, showHeader, bubbleMax, inner)
		b.WriteString(block)
		prevSender = msg.SenderID
		prevTime = msg.Timestamp
		first = false
	}
	if first {
		return statusStyle.Render("No messages yet")
	}
	return b.String()
}

func renderMessageBlock(msg store.Message, body string, mine, showHeader bool, bubbleMax, paneWidth int) string {
	name := "you"
	nameStyle := selfNameStyle
	bodyStyle := selfBodyStyle
	align := lipgloss.Right
	if !mine {
		name = msg.SenderName
		if name == "" {
			name = fmt.Sprintf("%d", msg.SenderID)
		}
		nameStyle = otherNameStyle
		bodyStyle = otherBodyStyle
		align = lipgloss.Left
	}
	if msg.Error != "" {
		bodyStyle = errorBodyStyle
	} else if msg.Pending {
		bodyStyle = pendingBodyStyle
	}

	ts := timeStyle.Render(msg.Timestamp.Local().Format("15:04"))
	var parts []string
	if showHeader {
		header := nameStyle.Render(name) + "  " + ts
		parts = append(parts, header)
	}

	for _, line := range wrapWords(body, bubbleMax) {
		parts = append(parts, bodyStyle.Render(line))
	}
	block := strings.Join(parts, "\n")
	return lipgloss.NewStyle().Width(paneWidth).Align(align).Render(block)
}

func wrapWords(s string, width int) []string {
	if width < 8 {
		width = 8
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	var out []string
	for _, paragraph := range strings.Split(s, "\n") {
		if strings.TrimSpace(paragraph) == "" {
			out = append(out, "")
			continue
		}
		out = append(out, wrapParagraph(paragraph, width)...)
	}
	if len(out) == 0 {
		return []string{""}
	}
	return out
}

func wrapParagraph(s string, width int) []string {
	words := strings.FieldsFunc(s, func(r rune) bool {
		return unicode.IsSpace(r)
	})
	if len(words) == 0 {
		return []string{""}
	}
	var lines []string
	var line strings.Builder
	for _, word := range words {
		for lipgloss.Width(word) > width {
			if line.Len() > 0 {
				lines = append(lines, line.String())
				line.Reset()
			}
			runes := []rune(word)
			cut := width
			if cut > len(runes) {
				cut = len(runes)
			}
			for cut > 1 && lipgloss.Width(string(runes[:cut])) > width {
				cut--
			}
			lines = append(lines, string(runes[:cut]))
			word = string(runes[cut:])
		}
		if line.Len() == 0 {
			line.WriteString(word)
			continue
		}
		candidate := line.String() + " " + word
		if lipgloss.Width(candidate) <= width {
			line.WriteByte(' ')
			line.WriteString(word)
			continue
		}
		lines = append(lines, line.String())
		line.Reset()
		line.WriteString(word)
	}
	if line.Len() > 0 {
		lines = append(lines, line.String())
	}
	return lines
}
