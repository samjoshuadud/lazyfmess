package ui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"fmesstui/internal/store"
)

type stubSender struct {
	id int64
}

func (s stubSender) AccountID() int64 { return s.id }
func (s stubSender) SendText(context.Context, store.Thread, string) (SendResult, error) {
	return SendResult{MessageID: "sent-1", Timestamp: time.UnixMilli(1_700_000_000_500)}, nil
}

func TestModelLoadsThreadsAndMessages(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()
	now := time.UnixMilli(1_700_000_000_000)
	if err := s.UpsertThread(ctx, store.Thread{ID: 7, Title: "Dana", LastPreview: "ping", UpdatedAt: now}); err != nil {
		t.Fatalf("UpsertThread: %v", err)
	}
	if err := s.UpsertMessage(ctx, store.Message{ID: "m7", ThreadID: 7, SenderID: 1, Text: "ping", Timestamp: now}); err != nil {
		t.Fatalf("UpsertMessage: %v", err)
	}

	m := New(ctx, s, stubSender{id: 1})
	msg := m.Init()()
	model, cmd := m.Update(msg)
	m = model.(Model)
	if cmd != nil {
		msg = cmd()
		model, _ = m.Update(msg)
		m = model.(Model)
	}
	if len(m.list.Items()) != 1 {
		t.Fatalf("thread items: got %d", len(m.list.Items()))
	}
	rendered := renderMessages([]store.Message{{ID: "m7", SenderID: 1, SenderName: "Dana", Text: "ping", Timestamp: now}})
	if !strings.Contains(rendered, "ping") || !strings.Contains(rendered, "Dana") {
		t.Fatalf("renderMessages missing text: %q", rendered)
	}

	model, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("expected quit command")
	}
}

func TestRefreshReloadsThreads(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()
	m := New(ctx, s, stubSender{id: 1})
	model, cmd := m.Update(RefreshMsg{})
	m = model.(Model)
	if cmd == nil {
		t.Fatal("expected reload command")
	}
	msg := cmd()
	model, _ = m.Update(msg)
	m = model.(Model)
	if m.status != "0 threads" {
		t.Fatalf("status: %q", m.status)
	}
}
