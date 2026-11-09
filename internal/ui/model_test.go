package ui

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

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

	m := New(ctx, s, stubSender{id: 1}, nil, nil, nil, nil, Config{})
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
	rendered := renderMessages([]store.Message{
		{ID: "m7", SenderID: 2, SenderName: "Dana", Text: "ping", Timestamp: now},
		{ID: "m8", SenderID: 1, SenderName: "Pat", Text: "pong", Timestamp: now.Add(time.Minute)},
		{ID: "m9", SenderID: 2, SenderName: "Dana", Text: "", Timestamp: now.Add(2 * time.Minute), MediaKind: "image", MediaURL: "https://example.test/a.jpg"},
	}, 40, 1, "m9").content
	if !strings.Contains(rendered, "ping") || !strings.Contains(rendered, "Dana") {
		t.Fatalf("renderMessages missing peer text: %q", rendered)
	}
	if !strings.Contains(rendered, "pong") || !strings.Contains(rendered, "you") {
		t.Fatalf("renderMessages missing self label: %q", rendered)
	}
	if strings.Contains(rendered, "Pat") {
		t.Fatalf("self message should use you, not contact name: %q", rendered)
	}
	if !strings.Contains(rendered, "[image]") || !strings.Contains(rendered, "▌") {
		t.Fatalf("renderMessages missing cursor or media placeholder: %q", rendered)
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
	m := New(ctx, s, stubSender{id: 1}, nil, nil, nil, nil, Config{})
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
	m := New(ctx, s, stubSender{id: 1}, nil, nil, nil, nil, Config{})
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
	m := New(ctx, s, stubSender{id: 1}, nil, nil, searcher, nil, Config{})
	m.ready = true
	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	m = model.(Model)
	if !m.showSearch {
		t.Fatal("expected people search open")
	}
	m.searchInput.SetValue("Dana")
	m = drain(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if len(m.searchResults) != 1 || m.searchResults[0].Name != "Dana" {
		t.Fatalf("search results: %+v", m.searchResults)
	}
	if m.inFlight != 0 {
		t.Fatalf("inFlight after search: %d", m.inFlight)
	}
	m = drain(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.showSearch {
		t.Fatal("expected people search closed")
	}
	if m.selectedID != 42 {
		t.Fatalf("selectedID: %d", m.selectedID)
	}
	if searcher.opened.ID != 42 {
		t.Fatalf("opened: %+v", searcher.opened)
	}
}

func newTestModel(t *testing.T, threads int, messagesPerThread int) Model {
	t.Helper()
	ctx := context.Background()
	s, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	base := time.UnixMilli(1_700_000_000_000)
	for i := 1; i <= threads; i++ {
		id := int64(i)
		if err := s.UpsertThread(ctx, store.Thread{ID: id, Title: fmt.Sprintf("Thread %d", i), UpdatedAt: base.Add(time.Duration(i) * time.Minute)}); err != nil {
			t.Fatalf("UpsertThread: %v", err)
		}
		for j := 0; j < messagesPerThread; j++ {
			if err := s.UpsertMessage(ctx, store.Message{ID: fmt.Sprintf("m%d-%d", i, j), ThreadID: id, SenderID: 2, Text: fmt.Sprintf("line %d", j), Timestamp: base.Add(time.Duration(j) * time.Second)}); err != nil {
				t.Fatalf("UpsertMessage: %v", err)
			}
		}
	}
	m := New(ctx, s, stubSender{id: 1}, nil, nil, nil, nil, Config{})
	m = drain(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	return drain(t, m, m.loadThreads())
}

func drain(t *testing.T, m Model, msg tea.Msg) Model {
	t.Helper()
	queue := []tea.Msg{msg}
	for len(queue) > 0 {
		next := queue[0]
		queue = queue[1:]
		model, cmd := m.Update(next)
		m = model.(Model)
		queue = append(queue, runCmd(cmd)...)
	}
	return m
}

func runCmd(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range batch {
			out = append(out, runCmd(c)...)
		}
		return out
	}
	if msg == nil {
		return nil
	}
	if _, ok := msg.(tea.QuitMsg); ok {
		return nil
	}
	if strings.HasPrefix(fmt.Sprintf("%T", msg), "cursor.") {
		return nil
	}
	return []tea.Msg{msg}
}

func TestEscClearsAppliedFilter(t *testing.T) {
	t.Parallel()
	m := newTestModel(t, 3, 0)
	m.list.SetFilterText("Thread 2")
	if m.list.FilterState() != list.FilterApplied {
		t.Fatalf("filter state: %v", m.list.FilterState())
	}
	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = model.(Model)
	if m.list.FilterState() != list.Unfiltered {
		t.Fatalf("filter state after esc: %v", m.list.FilterState())
	}
}

func TestRefreshKeepsScrollPosition(t *testing.T) {
	t.Parallel()
	m := newTestModel(t, 1, 80)
	if !m.viewport.AtBottom() {
		t.Fatal("expected first load at bottom")
	}
	m.viewport.GotoTop()
	m = drain(t, m, RefreshMsg{})
	if m.viewport.YOffset != 0 {
		t.Fatalf("refresh moved viewport to %d", m.viewport.YOffset)
	}
}

func TestHeaderUsesOpenThreadWhenFiltered(t *testing.T) {
	t.Parallel()
	m := newTestModel(t, 3, 0)
	m.selectedID = 1
	m.list.SetFilterText("Thread 3")
	if m.conversationTitle() != "Thread 1" {
		t.Fatal("header should show the open thread, not the filtered cursor")
	}
}

func TestViewFitsTerminal(t *testing.T) {
	t.Parallel()
	m := newTestModel(t, 5, 15)
	sizes := []struct{ w, h int }{{80, 24}, {100, 30}, {200, 50}, {60, 15}}
	for _, size := range sizes {
		model, _ := m.Update(tea.WindowSizeMsg{Width: size.w, Height: size.h})
		m = model.(Model)
		lines := strings.Split(m.View(), "\n")
		if len(lines) > size.h {
			t.Errorf("%dx%d: %d lines", size.w, size.h, len(lines))
		}
		for i, line := range lines {
			if w := lipgloss.Width(line); w > size.w {
				t.Errorf("%dx%d: line %d is %d wide", size.w, size.h, i, w)
				break
			}
		}
	}
}

func TestComposerReceivesNavigationLetters(t *testing.T) {
	t.Parallel()
	m := newTestModel(t, 1, 0)
	for _, r := range "ihlqs?" {
		model, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = model.(Model)
	}
	if got := m.composer.Value(); got != "hlqs?" {
		t.Fatalf("composer value: %q", got)
	}
	if m.focus != focusComposer || m.showHelp || m.showSearch {
		t.Fatal("letters in the composer must not trigger global keys")
	}
}

func TestHintsFollowFocus(t *testing.T) {
	t.Parallel()
	m := newTestModel(t, 1, 0)
	tests := []struct {
		focus focusPane
		want  string
	}{
		{focusList, "filter"},
		{focusMessages, "open media"},
		{focusComposer, "send"},
	}
	for _, tt := range tests {
		m.setFocus(tt.focus)
		if got := renderHints(m.contextBindings(), 200); !strings.Contains(got, tt.want) {
			t.Errorf("focus %v hints %q missing %q", tt.focus, got, tt.want)
		}
	}
}
