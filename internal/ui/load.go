package ui

import (
	"lazyfmess/internal/store"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) loadThreads() tea.Msg {
	threads, err := m.store.ListThreads(m.ctx)
	return threadsLoadedMsg{threads: threads, err: err}
}

const (
	toastDuration = 4 * time.Second
	messagePage   = 300
	resizeDelay   = 50 * time.Millisecond
)

func (m *Model) loadOlderAtTop() tea.Cmd {
	if !m.moreAbove || m.anchorID != "" || len(m.order) == 0 || m.cursorID != m.order[0] {
		return nil
	}
	span := m.spans[m.cursorID]
	m.anchorID = m.cursorID
	m.anchorOffset = span.start - m.viewport.YOffset
	m.messageLimit += messagePage
	m.status = "loading older messages"
	return m.loadMessages(m.selectedID)
}

func (m Model) loadMessages(threadID int64) tea.Cmd {
	limit := m.messageLimit
	if threadID != m.loadedID || limit == 0 {
		limit = messagePage
	}
	var seq int64
	if m.loads != nil {
		seq = m.loads.Add(1)
	}
	return func() tea.Msg {
		msgs, err := m.store.ListRecentMessages(m.ctx, threadID, limit)
		if err != nil {
			return messagesLoadedMsg{threadID: threadID, err: err, seq: seq}
		}
		marks, err := m.store.ReadMarks(m.ctx, threadID, m.selfAccountID())
		return messagesLoadedMsg{threadID: threadID, messages: msgs, readMarks: marks, err: err, seq: seq}
	}
}

func (m *Model) fetchHistory(thread store.Thread) tea.Cmd {
	if m.history == nil || thread.ID == 0 {
		return nil
	}
	m.historyLoading[thread.ID] = true
	m.refreshViewport()
	return m.track(func() tea.Msg {
		err := m.history.LoadHistory(m.ctx, thread)
		return historyFinishedMsg{threadID: thread.ID, err: err}
	})
}

func (m Model) reload() tea.Cmd {
	if m.selectedID != 0 {
		return tea.Batch(m.loadThreads, m.loadMessages(m.selectedID))
	}
	return m.loadThreads
}

func (m Model) loading() bool {
	return m.connecting || m.syncing
}
