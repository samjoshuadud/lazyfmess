package ui

import (
	"context"
	"fmt"
	"lazyfmess/internal/store"
	"strings"
	"sync/atomic"

	tea "github.com/charmbracelet/bubbletea"
)

type App struct {
	program *tea.Program
	queue   chan tea.Msg
	done    chan struct{}
	dirty   atomic.Bool
}

func NewApp(ctx context.Context, s *store.Store, svc Services, cfg Config) *App {
	p := tea.NewProgram(New(ctx, s, svc, cfg), tea.WithAltScreen(), tea.WithMouseCellMotion(), tea.WithContext(ctx))
	a := &App{program: p, queue: make(chan tea.Msg, 256), done: make(chan struct{})}
	go a.forward()
	return a
}

func (a *App) forward() {
	for {
		select {
		case msg := <-a.queue:
			if _, ok := msg.(RefreshMsg); ok {
				a.dirty.Store(false)
			}
			a.program.Send(msg)
		case <-a.done:
			return
		}
	}
}

func (a *App) send(msg tea.Msg, droppable bool) bool {
	if a == nil || a.program == nil {
		return false
	}
	if droppable {
		select {
		case a.queue <- msg:
			return true
		default:
			return false
		}
	}
	select {
	case a.queue <- msg:
		return true
	case <-a.done:
		return false
	}
}

func (a *App) Notify() {
	if a == nil || a.dirty.Swap(true) {
		return
	}
	if !a.send(RefreshMsg{}, true) {
		a.dirty.Store(false)
	}
}

func (a *App) Toast(message string) {
	if strings.TrimSpace(message) != "" {
		a.send(toastMsg{message: message}, true)
	}
}

func (a *App) NeedLogin(reason string) {
	a.send(needLoginMsg{reason: reason}, false)
}

func (a *App) SyncProgress(done, total int) {
	a.send(syncProgressMsg{done: done, total: total}, true)
}

func (a *App) Connected(err error) {
	a.send(connectedMsg{err: err}, false)
}

func (a *App) Typing(threadID, senderID int64, active bool) {
	a.send(typingMsg{threadID: threadID, senderID: senderID, active: active}, true)
}

func (a *App) Error(message string) {
	if strings.TrimSpace(message) != "" {
		a.send(errorMsg{text: message}, false)
	}
}

func (a *App) Run() error {
	defer close(a.done)
	_, err := a.program.Run()
	if err != nil {
		return fmt.Errorf("run tui: %w", err)
	}
	return nil
}
