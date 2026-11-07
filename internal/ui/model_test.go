package ui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"lazyfmess/internal/store"
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

	m := New(ctx, s, stubSender{id: 1}, nil, nil, nil, Config{})
	msg := m.loadThreads()
	model, cmd := m.Update(msg)
	m = model.(Model)
	for cmd != nil {
		next := cmd()
		if next == nil {
			break
		}
		model, cmd = m.Update(next)
		m = model.(Model)
	}
	if len(m.list.Items()) != 1 {
		t.Fatalf("thread items: got %d", len(m.list.Items()))
	}
	rendered := renderMessages([]store.Message{{ID: "m7", SenderID: 1, SenderName: "Dana", Text: "ping", Timestamp: now}}, 40)
	if !strings.Contains(rendered, "ping") || !strings.Contains(rendered, "Dana") {
		t.Fatalf("renderMessages missing text: %q", rendered)
	}

	model, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
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
	m := New(ctx, s, stubSender{id: 1}, nil, nil, nil, Config{})
	model, cmd := m.Update(RefreshMsg{})
	m = model.(Model)
	if cmd == nil {
		t.Fatal("expected reload command")
	}
	msg := cmd()
	model, _ = m.Update(msg)
	m = model.(Model)
	if m.status != "0 chats" {
		t.Fatalf("status: %q", m.status)
	}
}

func TestHelpToggle(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()
	m := New(ctx, s, stubSender{id: 1}, nil, nil, nil, Config{})
	m.ready = true
	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	m = model.(Model)
	if !m.showHelp {
		t.Fatal("expected help open")
	}
	model, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = model.(Model)
	if m.showHelp {
		t.Fatal("expected help closed")
	}
}

type stubSearcher struct {
	results []PersonHit
	opened  PersonHit
}

func (s *stubSearcher) SearchPeople(context.Context, string) ([]PersonHit, error) {
	return s.results, nil
}

func (s *stubSearcher) OpenPerson(_ context.Context, person PersonHit) (int64, error) {
	s.opened = person
	return person.ID, nil
}

func TestPeopleSearchOpensChat(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()
	searcher := &stubSearcher{
		results: []PersonHit{{ID: 42, Name: "Dana", Context: "person", ThreadType: 1}},
	}
	m := New(ctx, s, stubSender{id: 1}, nil, nil, searcher, Config{})
	m.ready = true
	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	m = model.(Model)
	if !m.showSearch {
		t.Fatal("expected people search open")
	}
	m.searchInput.SetValue("Dana")
	model, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = model.(Model)
	if cmd == nil {
		t.Fatal("expected search command")
	}
	model, _ = m.Update(cmd())
	m = model.(Model)
	if len(m.searchResults) != 1 || m.searchResults[0].Name != "Dana" {
		t.Fatalf("search results: %+v", m.searchResults)
	}
	model, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = model.(Model)
	if cmd == nil {
		t.Fatal("expected open command")
	}
	model, cmd = m.Update(cmd())
	m = model.(Model)
	if m.showSearch {
		t.Fatal("expected people search closed")
	}
	if m.selectedID != 42 {
		t.Fatalf("selectedID: %d", m.selectedID)
	}
	if searcher.opened.ID != 42 {
		t.Fatalf("opened: %+v", searcher.opened)
	}
	_ = cmd
}
