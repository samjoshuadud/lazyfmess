package ui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"lazyfmess/internal/store"
)

type Muter interface {
	SetMuted(ctx context.Context, thread store.Thread, muted bool) error
}

type mutedMsg struct {
	muted bool
	err   error
}

func (m *Model) toggleMute() tea.Cmd {
	thread, ok := m.currentThread()
	if m.muter == nil || !ok {
		return nil
	}
	muted := !thread.Muted(time.Now())
	return m.track(func() tea.Msg {
		return mutedMsg{muted: muted, err: m.muter.SetMuted(m.ctx, thread, muted)}
	})
}
