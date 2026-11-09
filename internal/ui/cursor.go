package ui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/muesli/termenv"

	"lazyfmess/internal/store"
)

type editorFinishedMsg struct {
	text string
	err  error
}

func (m *Model) refreshViewport() {
	cursor := ""
	if m.focus == focusMessages {
		cursor = m.cursorID
	}
	chat := renderMessages(m.messages, m.viewport.Width, m.selfAccountID(), cursor)
	m.viewport.SetContent(chat.content)
	m.spans = chat.spans
	m.order = chat.order
}

func (m *Model) placeCursor(followEnd bool) {
	if len(m.order) == 0 {
		m.cursorID = ""
		return
	}
	if _, ok := m.spans[m.cursorID]; !ok || followEnd {
		m.cursorID = m.order[len(m.order)-1]
	}
}

func (m *Model) moveCursor(delta int) {
	if len(m.order) == 0 {
		return
	}
	idx := m.cursorIndex()
	m.setCursor(m.order[min(max(idx+delta, 0), len(m.order)-1)])
}

func (m *Model) jumpMedia(delta int) {
	for i := m.cursorIndex() + delta; i >= 0 && i < len(m.order); i += delta {
		if msg, ok := m.messageByID(m.order[i]); ok && hasMedia(msg) {
			m.setCursor(msg.ID)
			return
		}
	}
	m.status = "no more media in this chat"
}

func (m *Model) setCursor(id string) {
	m.cursorID = id
	m.refreshViewport()
	span, ok := m.spans[id]
	if !ok {
		return
	}
	top, height := m.viewport.YOffset, m.viewport.Height
	switch {
	case span.start < top || span.end-span.start >= height:
		m.viewport.SetYOffset(span.start)
	case span.end >= top+height:
		m.viewport.SetYOffset(span.end - height + 1)
	}
}

func (m Model) cursorIndex() int {
	for i, id := range m.order {
		if id == m.cursorID {
			return i
		}
	}
	return len(m.order) - 1
}

func (m Model) cursorMessage() (store.Message, bool) {
	return m.messageByID(m.cursorID)
}

func (m Model) messageByID(id string) (store.Message, bool) {
	for _, msg := range m.messages {
		if msg.ID == id {
			return msg, true
		}
	}
	return store.Message{}, false
}

func hasMedia(msg store.Message) bool {
	return msg.MediaKind != "" && msg.MediaURL != ""
}

func (m *Model) startReply(msg store.Message) {
	if msg.Pending || msg.Error != "" {
		return
	}
	m.replyTo = msg
	m.setFocus(focusComposer)
}

func (m Model) messageActions(msg store.Message) []menuItem {
	var items []menuItem
	if !msg.Pending && msg.Error == "" {
		items = append(items,
			menuItem{"Reply", func(m *Model) tea.Cmd { m.startReply(msg); return nil }},
			menuItem{"React", func(m *Model) tea.Cmd { m.openReactionMenu(msg); return nil }},
		)
	}
	if hasMedia(msg) {
		items = append(items, menuItem{"Open " + mediaPlaceholder(msg), func(m *Model) tea.Cmd { return m.track(m.openMedia(msg)) }})
	}
	if strings.TrimSpace(msg.Text) != "" {
		items = append(items, menuItem{"Copy text", func(*Model) tea.Cmd { return copyText(msg.Text) }})
	}
	return items
}

func (m Model) openMedia(msg store.Message) tea.Cmd {
	if !hasMedia(msg) {
		return func() tea.Msg { return mediaOpenedMsg{err: fmt.Errorf("this message has no media")} }
	}
	return func() tea.Msg {
		if m.media == nil {
			return mediaOpenedMsg{err: fmt.Errorf("media open is unavailable")}
		}
		return mediaOpenedMsg{err: m.media.OpenMedia(m.ctx, msg)}
	}
}

func copyText(text string) tea.Cmd {
	return func() tea.Msg {
		termenv.Copy(text)
		return toastMsg{message: "copied to clipboard"}
	}
}

func editDraft(draft string) tea.Cmd {
	f, err := os.CreateTemp("", "lazyfmess-*.txt")
	if err != nil {
		return func() tea.Msg { return editorFinishedMsg{err: fmt.Errorf("create draft file: %w", err)} }
	}
	path := f.Name()
	_, err = f.WriteString(draft)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		os.Remove(path)
		return func() tea.Msg { return editorFinishedMsg{err: fmt.Errorf("write draft file: %w", err)} }
	}
	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		editor = "vi"
	}
	args := append(strings.Fields(editor), path)
	return tea.ExecProcess(exec.Command(args[0], args[1:]...), func(err error) tea.Msg {
		defer os.Remove(path)
		if err != nil {
			return editorFinishedMsg{err: fmt.Errorf("run editor: %w", err)}
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return editorFinishedMsg{err: fmt.Errorf("read draft file: %w", err)}
		}
		return editorFinishedMsg{text: strings.TrimRight(string(data), "\n")}
	})
}

func (m Model) handleMouse(msg tea.MouseMsg) (Model, tea.Cmd) {
	inChats := msg.X < m.leftWidth
	inComposer := !inChats && msg.Y >= m.height-1-composerFrameHeight && msg.Y < m.height-1
	switch {
	case msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown:
		up := msg.Button == tea.MouseButtonWheelUp
		switch {
		case inChats && up:
			m.list.CursorUp()
			return m.updateList(nil)
		case inChats:
			m.list.CursorDown()
			return m.updateList(nil)
		case up:
			m.viewport.ScrollUp(3)
		default:
			m.viewport.ScrollDown(3)
		}
	case msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress:
		switch {
		case inChats:
			m.setFocus(focusList)
		case inComposer && m.selectedID != 0:
			m.setFocus(focusComposer)
		case !inComposer:
			m.setFocus(focusMessages)
		}
	}
	return m, nil
}
