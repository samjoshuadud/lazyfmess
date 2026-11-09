package ui

import (
	"context"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"lazyfmess/internal/store"
)

const (
	typingTimeout = 6 * time.Second
	typingResend  = 4 * time.Second
)

type TypingNotifier interface {
	SetTyping(ctx context.Context, thread store.Thread, active bool) error
}

type typingMsg struct {
	threadID int64
	senderID int64
	active   bool
}

type typingExpiredMsg struct{}

type typingSentMsg struct {
	err error
}

func (m *Model) applyTyping(msg typingMsg) tea.Cmd {
	if msg.senderID == m.selfAccountID() {
		return nil
	}
	senders := m.typing[msg.threadID]
	if !msg.active {
		delete(senders, msg.senderID)
		return nil
	}
	if senders == nil {
		senders = make(map[int64]time.Time)
		m.typing[msg.threadID] = senders
	}
	senders[msg.senderID] = time.Now().Add(typingTimeout)
	return tea.Tick(typingTimeout, func(time.Time) tea.Msg { return typingExpiredMsg{} })
}

func (m *Model) expireTyping(now time.Time) {
	for threadID, senders := range m.typing {
		for senderID, until := range senders {
			if now.After(until) {
				delete(senders, senderID)
			}
		}
		if len(senders) == 0 {
			delete(m.typing, threadID)
		}
	}
}

func (m Model) typingLabel() string {
	senders := m.typing[m.selectedID]
	if len(senders) == 0 {
		return ""
	}
	names := make([]string, 0, len(senders))
	for senderID := range senders {
		names = append(names, m.senderName(senderID))
	}
	if len(names) > 2 {
		return "several people are typing…"
	}
	verb := " is typing…"
	if len(names) == 2 {
		verb = " are typing…"
	}
	return strings.Join(names, " and ") + verb
}

func (m Model) senderName(senderID int64) string {
	for i := len(m.messages) - 1; i >= 0; i-- {
		if m.messages[i].SenderID == senderID && m.messages[i].SenderName != "" {
			return m.messages[i].SenderName
		}
	}
	if senderID == m.selectedID {
		if t, ok := m.currentThread(); ok {
			return t.Title
		}
	}
	return "someone"
}

func (m *Model) updateOwnTyping() tea.Cmd {
	if m.typingNotifier == nil || m.selectedID == 0 {
		return nil
	}
	composing := m.focus == focusComposer && strings.TrimSpace(m.composer.Value()) != ""
	switch {
	case composing && (m.typingThread != m.selectedID || time.Since(m.typingSentAt) > typingResend):
		return m.sendTyping(m.selectedID, true)
	case !composing && m.typingThread != 0:
		return m.sendTyping(m.typingThread, false)
	}
	return nil
}

func (m *Model) sendTyping(threadID int64, active bool) tea.Cmd {
	m.typingThread, m.typingSentAt = 0, time.Time{}
	if active {
		m.typingThread, m.typingSentAt = threadID, time.Now()
	}
	thread, ok := m.threadByID(threadID)
	if !ok {
		return nil
	}
	return func() tea.Msg {
		return typingSentMsg{err: m.typingNotifier.SetTyping(m.ctx, thread, active)}
	}
}

func (m Model) threadByID(id int64) (store.Thread, bool) {
	for _, item := range m.list.Items() {
		if t, ok := item.(threadItem); ok && t.thread.ID == id {
			return t.thread, true
		}
	}
	return store.Thread{}, false
}
