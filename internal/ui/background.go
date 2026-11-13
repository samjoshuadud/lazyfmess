package ui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) showToast(message string) tea.Cmd {
	m.toast = message
	return tea.Tick(toastDuration, func(time.Time) tea.Msg {
		return toastExpiredMsg{message: message}
	})
}

func (m *Model) kickHistorySync() tea.Cmd {
	if m.syncStarted {
		return nil
	}
	return m.runBackgroundSync()
}

func (m *Model) runBackgroundSync() tea.Cmd {
	sync := m.backgroundSync
	if sync == nil || m.syncing {
		return nil
	}
	m.syncStarted = true
	m.syncing = true
	m.refreshViewport()
	return m.track(func() tea.Msg { return backgroundSyncMsg{err: sync(m.ctx)} })
}

func (m *Model) checkUnlock() tea.Cmd {
	if m.unlocker == nil {
		return nil
	}
	m.status = "checking encrypted history"
	return m.track(func() tea.Msg {
		needs, err := m.unlocker.NeedsPIN(m.ctx)
		return unlockCheckMsg{needs: needs, err: err}
	})
}
