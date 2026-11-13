package ui

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"lazyfmess/internal/store"
)

type threadsLoadedMsg struct {
	threads []store.Thread
	err     error
}

type messagesLoadedMsg struct {
	seq       int64
	threadID  int64
	messages  []store.Message
	readMarks []time.Time
	err       error
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

type lineSpan struct {
	start, end int
}

type chatRender struct {
	content string
	spans   map[string]lineSpan
	order   []string
}

func renderMessages(messages []store.Message, width int, selfID int64, cursorID string, readMarks []time.Time, cache map[string]string) chatRender {
	if width <= 0 {
		width = 40
	}
	inner := max(width-chatSidePad*2, 16)
	bubbleMax := min(max(inner*3/4, 12), inner)

	out := chatRender{spans: make(map[string]lineSpan)}
	var rows []string
	var prevSender int64
	var prevTime time.Time
	now := time.Now()
	seenID, seen := seenStatus(messages, selfID, readMarks)

	for _, msg := range messages {
		body := messageBody(msg)
		if body == "" {
			continue
		}
		first := len(out.order) == 0
		mine := selfID != 0 && msg.SenderID == selfID
		newDay := first || !sameDay(msg.Timestamp.Local(), prevTime.Local())
		closeInTime := !first && msg.SenderID == prevSender && !newDay && msg.Timestamp.Sub(prevTime) < 5*time.Minute
		showHeader := !closeInTime

		if !first && showHeader {
			rows = append(rows, "")
		}
		if newDay {
			rows = append(rows, daySeparator(dayLabel(msg.Timestamp, now), width), "")
		}
		status := ""
		if msg.ID == seenID {
			status = seen
		}
		if msg.System && !first && !showHeader {
			rows = append(rows, "")
		}
		quote := replyQuote(msg, selfID)
		key := blockKey(msg, body, quote, status, mine, showHeader, width)
		rendered, ok := cache[key]
		if !ok {
			if msg.System {
				rendered = systemLine(msg, selfID, width)
			} else {
				rendered = renderMessageBlock(msg, body, quote, status, mine, showHeader, bubbleMax, width)
			}
			if cache != nil {
				cache[key] = rendered
			}
		}
		block := strings.Split(rendered, "\n")
		gutter := " "
		if msg.ID == cursorID {
			gutter = focusMark.Render("▌")
		}
		for k := range block {
			block[k] = gutter + block[k]
		}
		out.spans[msg.ID] = lineSpan{start: len(rows), end: len(rows) + len(block) - 1}
		out.order = append(out.order, msg.ID)
		rows = append(rows, block...)
		prevSender = msg.SenderID
		if msg.System {
			prevSender = 0
		}
		prevTime = msg.Timestamp
	}
	if len(out.order) == 0 {
		out.content = chatPlaceholder("No messages yet", width)
		return out
	}
	out.content = strings.Join(rows, "\n")
	return out
}

func seenStatus(messages []store.Message, selfID int64, readMarks []time.Time) (string, string) {
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if msg.SenderID != selfID || msg.Pending || msg.Error != "" || msg.Unsent {
			continue
		}
		readers := 0
		for _, mark := range readMarks {
			if !mark.Before(msg.Timestamp) {
				readers++
			}
		}
		switch readers {
		case 0:
			return "", ""
		case 1:
			return msg.ID, "Seen"
		default:
			return msg.ID, fmt.Sprintf("Seen by %d", readers)
		}
	}
	return "", ""
}

func blockKey(msg store.Message, body, quote, status string, mine, header bool, width int) string {
	return fmt.Sprintf("%s\x00%d\x00%t%t%t%t%t%t\x00%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%d",
		msg.ID, width, mine, header, msg.Pending, msg.Edited, msg.Unsent, msg.System,
		body, quote, status, msg.SenderName, msg.Error, reactionSummary(msg.Reactions), msg.Timestamp.UnixMilli())
}

func messageOrder(messages []store.Message) []string {
	order := make([]string, 0, len(messages))
	for _, msg := range messages {
		if messageBody(msg) != "" {
			order = append(order, msg.ID)
		}
	}
	return order
}

func systemLine(msg store.Message, selfID int64, width int) string {
	who := replyName(msg, selfID)
	if who == "you" {
		who = "You"
	}
	text := ansi.Truncate(who+" "+msg.Text+" · "+msg.Timestamp.Local().Format("15:04"), width-2, "…")
	return lipgloss.PlaceHorizontal(width-1, lipgloss.Center, quoteStyle.Render(text))
}

func chatPlaceholder(text string, width int) string {
	return lipgloss.NewStyle().
		Width(width).
		PaddingLeft(chatSidePad).
		PaddingRight(chatSidePad).
		Render(statusStyle.Render(text))
}

func messageFooter(msg store.Message, status string) string {
	var parts []string
	if msg.Edited && !msg.Unsent {
		parts = append(parts, "edited")
	}
	if summary := reactionSummary(msg.Reactions); summary != "" {
		parts = append(parts, summary)
	}
	if status != "" {
		parts = append(parts, status)
	}
	return strings.Join(parts, " · ")
}

func daySeparator(label string, width int) string {
	label = " " + label + " "
	side := max((width-lipgloss.Width(label))/2, 1)
	rule := strings.Repeat("─", side)
	return timeStyle.Render(lipgloss.PlaceHorizontal(width, lipgloss.Center, rule+label+rule))
}

func messageBody(msg store.Message) string {
	if msg.Unsent {
		return "message unsent"
	}
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
		placeholder := mediaPlaceholder(msg)
		if body == "" {
			body = placeholder
		} else {
			body += "\n" + placeholder
		}
	}
	return body
}

func mediaPlaceholder(msg store.Message) string {
	label := "[" + msg.MediaKind + "]"
	switch {
	case msg.MediaKind == "video" && msg.MediaDurationMs > 0:
		label = fmt.Sprintf("[video %s]", formatMediaDuration(msg.MediaDurationMs))
	case msg.MediaKind != "image" && msg.MediaKind != "video" && msg.MediaName != "":
		label = "[" + msg.MediaKind + " " + msg.MediaName + "]"
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

func replyQuote(msg store.Message, selfID int64) string {
	if msg.ReplyToID == "" {
		return ""
	}
	text := strings.Join(strings.Fields(msg.ReplyText), " ")
	if text == "" {
		text = "a message"
	}
	return "↪ " + replyName(store.Message{SenderID: msg.ReplySenderID, SenderName: msg.ReplySenderName}, selfID) + ": " + text
}

func replyName(msg store.Message, selfID int64) string {
	switch {
	case msg.SenderID != 0 && msg.SenderID == selfID:
		return "you"
	case msg.SenderName != "":
		return msg.SenderName
	case msg.SenderID != 0:
		return fmt.Sprintf("%d", msg.SenderID)
	default:
		return "message"
	}
}

func renderMessageBlock(msg store.Message, body, quote, status string, mine, showHeader bool, bubbleMax, paneWidth int) string {
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
	switch {
	case msg.Unsent:
		bodyStyle = quoteStyle
	case msg.Error != "":
		bodyStyle = errorBodyStyle
	case msg.Pending:
		bodyStyle = pendingBodyStyle
	}

	ts := timeStyle.Render(msg.Timestamp.Local().Format("15:04"))
	var parts []string
	if showHeader {
		header := nameStyle.Render(name) + "  " + ts
		parts = append(parts, header)
	}
	if quote != "" {
		parts = append(parts, quoteStyle.Render(ansi.Truncate(quote, bubbleMax, "…")))
	}

	for _, line := range wrapWords(body, bubbleMax) {
		parts = append(parts, bodyStyle.Render(line))
	}
	if footer := messageFooter(msg, status); footer != "" {
		parts = append(parts, timeStyle.Render(ansi.Truncate(footer, bubbleMax, "…")))
	}
	block := strings.Join(parts, "\n")
	return lipgloss.NewStyle().
		Width(paneWidth - 1).
		PaddingLeft(chatSidePad - 1).
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
