package ui

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"lazyfmess/internal/store"
)

type stubSender struct {
	id int64
}

func (s stubSender) AccountID() int64 { return s.id }
func (s stubSender) SendText(context.Context, store.Thread, string, store.Message) (SendResult, error) {
	return SendResult{MessageID: "sent-1", Timestamp: time.UnixMilli(1_700_000_000_500)}, nil
}

func (s stubSender) SendFile(context.Context, store.Thread, string) (SendResult, error) {
	return SendResult{MessageID: "sent-file", Timestamp: time.UnixMilli(1_700_000_000_600)}, nil
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

	m := New(ctx, s, Services{Sender: stubSender{id: 1}}, Config{})
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
	}, 40, 1, "m9", nil, time.Time{}, nil).content
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

func TestRefreshBurstReloadsOnce(t *testing.T) {
	t.Parallel()
	m := newTestModel(t, 2, 0)
	var cmds []tea.Cmd
	for range 50 {
		model, cmd := m.Update(RefreshMsg{})
		m = model.(Model)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	if len(cmds) != 1 || !m.refreshQueued {
		t.Fatalf("50 refreshes queued %d reloads", len(cmds))
	}
	m.status = ""
	for _, msg := range runCmd(cmds[0]) {
		m = drain(t, m, msg)
	}
	if m.refreshQueued || m.status != "2 chats · 2 unread" {
		t.Fatalf("after reload: queued %v status %q", m.refreshQueued, m.status)
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
	m := New(ctx, s, Services{Sender: stubSender{id: 1}}, Config{})
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
	m := New(ctx, s, Services{Sender: stubSender{id: 1}, Searcher: searcher}, Config{})
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
	m := New(ctx, s, Services{Sender: stubSender{id: 1}}, Config{})
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
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	var msg tea.Msg
	select {
	case msg = <-done:
	case <-time.After(250 * time.Millisecond):
		return nil
	}
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
	if _, ok := msg.(spinner.TickMsg); ok {
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

func TestStaleLoadsAreDropped(t *testing.T) {
	t.Parallel()
	m := newTestModel(t, 1, 3)
	older := m.loadMessages(1)()
	newer := m.loadMessages(1)()
	model, _ := m.Update(newer)
	m = model.(Model)
	model, _ = m.Update(messagesLoadedMsg{threadID: 1, seq: older.(messagesLoadedMsg).seq})
	m = model.(Model)
	if len(m.messages) != 3 {
		t.Fatalf("an older load replaced newer results: %d messages", len(m.messages))
	}
}

func TestResizeRendersAfterTheLastSize(t *testing.T) {
	t.Parallel()
	m := newTestModel(t, 1, 3)
	model, first := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = model.(Model)
	model, last := m.Update(tea.WindowSizeMsg{Width: 90, Height: 30})
	m = model.(Model)
	if first == nil || last == nil {
		t.Fatal("resize should schedule a render")
	}
	model, _ = m.Update(first())
	m = model.(Model)
	if m.resizeGen != 3 {
		t.Fatalf("resize generation: %d", m.resizeGen)
	}
	if w := m.viewport.Width; w != m.rightWidth-2 {
		t.Fatalf("viewport width %d does not follow the latest size", w)
	}
}

func TestAppQueuesWithoutBlocking(t *testing.T) {
	t.Parallel()
	a := &App{program: new(tea.Program), queue: make(chan tea.Msg, 2), done: make(chan struct{})}
	for range 5 {
		a.Notify()
	}
	if len(a.queue) != 1 {
		t.Fatalf("notifications should collapse into one refresh, queued %d", len(a.queue))
	}
	for range 10 {
		a.Typing(1, 2, true)
	}
	if len(a.queue) != 2 {
		t.Fatalf("queue length %d", len(a.queue))
	}
	close(a.done)
	a.Error("boom")
}

func TestResizeKeepsNewestMessageVisible(t *testing.T) {
	t.Parallel()
	m := newTestModel(t, 1, 40)
	if !m.viewport.AtBottom() {
		t.Fatal("expected to start at the bottom")
	}
	for _, size := range []tea.WindowSizeMsg{{Width: 60, Height: 16}, {Width: 160, Height: 40}, {Width: 70, Height: 20}} {
		m = drain(t, m, size)
		if !m.viewport.AtBottom() {
			t.Fatalf("after resize to %dx%d the newest message is out of view", size.Width, size.Height)
		}
	}
}

func TestCompactAndTinyLayouts(t *testing.T) {
	t.Parallel()
	m := newTestModel(t, 2, 3)
	m = drain(t, m, tea.WindowSizeMsg{Width: 30, Height: 10})
	if !strings.Contains(m.View(), "Too small: 30x10") {
		t.Fatal("tiny terminals should show a notice")
	}
	m = drain(t, m, tea.WindowSizeMsg{Width: 60, Height: 20})
	if v := m.View(); !strings.Contains(v, "[1] Chats") || strings.Contains(v, "[3] Compose") {
		t.Fatal("compact mode should show only the chats panel while it has focus")
	}
	m = drain(t, m, keyPress("enter"))
	if v := m.View(); strings.Contains(v, "[1] Chats") || !strings.Contains(v, "[3] Compose") {
		t.Fatal("compact mode should switch to the chat after opening it")
	}
	for i, line := range strings.Split(m.View(), "\n") {
		if w := lipgloss.Width(line); w > 60 {
			t.Fatalf("line %d is %d wide", i, w)
		}
	}
}

func TestRefreshKeepsSelectionUnderFilter(t *testing.T) {
	t.Parallel()
	m := newTestModel(t, 5, 0)
	m.list.SetFilterText("Thread 2")
	m.selectedID = 2
	m = drain(t, m, m.loadThreads())
	item, ok := m.list.SelectedItem().(threadItem)
	if !ok || item.thread.ID != 2 {
		t.Fatalf("selection after refresh under a filter: %+v", m.list.SelectedItem())
	}
}

func TestFastTypingIsTextNotKeys(t *testing.T) {
	t.Parallel()
	m := newTestModel(t, 1, 0)
	m = drain(t, m, keyPress("i"))
	for _, chunk := range []string{"go ", "left", " then ", "right", " and ", "tab", " ", "enter"} {
		model, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(chunk)})
		m = model.(Model)
	}
	if got := m.composer.Value(); got != "go left then right and tab enter" || m.focus != focusComposer {
		t.Fatalf("composer %q focus %v", got, m.focus)
	}
}
