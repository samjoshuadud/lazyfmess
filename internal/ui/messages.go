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
	threadID int64
	err      error
}

type RefreshMsg struct{}

type toastExpiredMsg struct {
	message string
}

type toastMsg struct {
	message string
}

type mediaOpenedMsg struct {
	err error
}

const chatSidePad = 3

func renderMessages(messages []store.Message, width int, selfID int64, mediaFocusID string) string {
	if width <= 0 {
		width = 40
	}
	inner := width - chatSidePad*2
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
	now := time.Now()

	for _, msg := range messages {
		body := messageBody(msg, msg.ID == mediaFocusID)
		if body == "" {
			continue
		}

		mine := selfID != 0 && msg.SenderID == selfID
		newDay := first || !sameDay(msg.Timestamp.Local(), prevTime.Local())
		sameSender := !first && msg.SenderID == prevSender
		closeInTime := sameSender && !newDay && msg.Timestamp.Sub(prevTime) < 5*time.Minute
		showHeader := !closeInTime

		if !first {
			if showHeader {
				b.WriteString("\n\n")
			} else {
				b.WriteByte('\n')
			}
		}
		if newDay {
			b.WriteString(daySeparator(dayLabel(msg.Timestamp, now), width))
			b.WriteString("\n\n")
		}

		block := renderMessageBlock(msg, body, mine, showHeader, bubbleMax, width)
		b.WriteString(block)
		prevSender = msg.SenderID
		prevTime = msg.Timestamp
		first = false
	}
	if first {
		return lipgloss.NewStyle().
			Width(width).
			PaddingLeft(chatSidePad).
			PaddingRight(chatSidePad).
			Render(statusStyle.Render("No messages yet"))
	}
	return b.String()
}

func daySeparator(label string, width int) string {
	label = " " + label + " "
	side := max((width-lipgloss.Width(label))/2, 1)
	rule := strings.Repeat("─", side)
	return timeStyle.Render(lipgloss.PlaceHorizontal(width, lipgloss.Center, rule+label+rule))
}

func messageBody(msg store.Message, mediaFocused bool) string {
	body := strings.TrimSpace(msg.Text)
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
	if msg.MediaKind != "" {
		placeholder := mediaPlaceholder(msg, mediaFocused)
		if body == "" {
			body = placeholder
		} else {
			body += "\n" + placeholder
		}
	}
	return body
}

func mediaPlaceholder(msg store.Message, focused bool) string {
	label := "[" + msg.MediaKind + "]"
	if msg.MediaKind == "video" && msg.MediaDurationMs > 0 {
		label = fmt.Sprintf("[video %s]", formatMediaDuration(msg.MediaDurationMs))
	}
	if focused {
		return "▸ " + label
	}
	return label
}

func formatMediaDuration(ms int64) string {
	if ms < 0 {
		ms = 0
	}
	sec := ms / 1000
	return fmt.Sprintf("%d:%02d", sec/60, sec%60)
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
	return lipgloss.NewStyle().
		Width(paneWidth).
		PaddingLeft(chatSidePad).
		PaddingRight(chatSidePad).
		Align(align).
		Render(block)
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
