package ui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"lazyfmess/internal/store"
)

type MessageEditor interface {
	Edit(ctx context.Context, thread store.Thread, msg store.Message, text string) error
	Unsend(ctx context.Context, thread store.Thread, msg store.Message) error
}

type editedMsg struct {
	threadID int64
	err      error
}

func (m Model) canModify(msg store.Message) bool {
	return msg.SenderID == m.selfAccountID() && !msg.Pending && msg.Error == "" && !msg.Unsent
}

func (m *Model) startEdit(msg store.Message) {
	if !m.canModify(msg) || strings.TrimSpace(msg.Text) == "" {
		return
	}
	m.editing = msg
	m.replyTo = store.Message{}
	m.composer.SetValue(msg.Text)
	m.setFocus(focusComposer)
}

func (m *Model) cancelEdit() {
	m.editing = store.Message{}
	m.composer.Reset()
}

func (m *Model) submitEdit() tea.Cmd {
	msg := m.editing
	text := strings.TrimSpace(m.composer.Value())
	m.cancelEdit()
	if text == "" || text == strings.TrimSpace(msg.Text) {
		return nil
	}
	return m.runEdit(msg, func(ctx context.Context, thread store.Thread) error {
		return m.messageEditor.Edit(ctx, thread, msg, text)
	})
}

func (m *Model) confirmUnsend(msg store.Message) {
	if !m.canModify(msg) {
		return
	}
	m.showConfirm("Unsend message", "Remove this message for everyone in the chat?", func(m *Model) tea.Cmd {
		return m.runEdit(msg, func(ctx context.Context, thread store.Thread) error {
			return m.messageEditor.Unsend(ctx, thread, msg)
		})
	})
}

func (m *Model) runEdit(msg store.Message, run func(context.Context, store.Thread) error) tea.Cmd {
	thread, ok := m.threadByID(msg.ThreadID)
	if m.messageEditor == nil || !ok {
		return func() tea.Msg { return editedMsg{err: fmt.Errorf("editing is unavailable")} }
	}
	return m.track(func() tea.Msg {
		return editedMsg{threadID: thread.ID, err: run(m.ctx, thread)}
	})
}
