package ui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"lazyfmess/internal/store"
)

type blockingHistory struct {
	release chan struct{}
}

func (h blockingHistory) LoadHistory(ctx context.Context, _ store.Thread) error {
	<-h.release
	return nil
}

func TestChatsPanelLoadingState(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()
	m := New(ctx, s, Services{Sender: stubSender{id: 1}}, Config{Connecting: true})
	model, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = model.(Model)
	if !strings.Contains(ansi.Strip(m.View()), "Loading chats…") {
		t.Fatal("expected loading placeholder while connecting")
	}
	model, _ = m.Update(connectedMsg{})
	m = model.(Model)
	model, _ = m.Update(m.loadThreads())
	m = model.(Model)
	if view := ansi.Strip(m.View()); !strings.Contains(view, "No chats yet") || strings.Contains(view, "Loading chats") {
		t.Fatal("expected empty placeholder once loaded")
	}
}

func TestChatLoadingWhileHistoryFetches(t *testing.T) {
	t.Parallel()
	m := newTestModel(t, 1, 0)
	history := blockingHistory{release: make(chan struct{})}
	defer close(history.release)
	m.history = history
	model, _ := m.Update(keyPress("enter"))
	m = model.(Model)
	if !strings.Contains(ansi.Strip(m.viewport.View()), "Loading messages…") {
		t.Fatalf("expected loading placeholder:\n%s", ansi.Strip(m.viewport.View()))
	}
	model, _ = m.Update(historyFinishedMsg{threadID: 1})
	m = model.(Model)
	m.refreshViewport()
	if !strings.Contains(ansi.Strip(m.viewport.View()), "No messages yet") {
		t.Fatal("expected empty placeholder after history finished")
	}
}

func TestBackgroundSyncShowsActivity(t *testing.T) {
	t.Parallel()
	m := newTestModel(t, 1, 1)
	release := make(chan struct{})
	m.backgroundSync = func(context.Context) error { <-release; return nil }
	cmd := m.kickHistorySync()
	if cmd == nil || !m.syncing || m.inFlight != 1 {
		t.Fatalf("sync not tracked: syncing %v inFlight %d", m.syncing, m.inFlight)
	}
	if !strings.Contains(ansi.Strip(m.renderStatusBar()), "syncing history") {
		t.Fatal("status bar missing sync activity")
	}
	if m.kickHistorySync() != nil {
		t.Fatal("background sync must only start once")
	}
	close(release)
	m = drain(t, m, workDoneMsg{msg: backgroundSyncMsg{}})
	if m.syncing || m.inFlight != 0 {
		t.Fatalf("after sync: syncing %v inFlight %d", m.syncing, m.inFlight)
	}
}
