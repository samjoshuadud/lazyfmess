package ui

import (
	"context"
	"sync"
	"testing"

	"lazyfmess/internal/store"
)

type recordingReader struct {
	mu     sync.Mutex
	s      *store.Store
	marked []int64
}

func (r *recordingReader) MarkRead(ctx context.Context, thread store.Thread) error {
	r.mu.Lock()
	r.marked = append(r.marked, thread.ID)
	r.mu.Unlock()
	return r.s.SetLastRead(ctx, thread.ID, thread.UpdatedAt)
}

func TestOpeningChatMarksRead(t *testing.T) {
	t.Parallel()
	m := newTestModel(t, 3, 1)
	reader := &recordingReader{s: m.store}
	m.reader = reader
	m = drain(t, m, keyPress("j"))
	if len(reader.marked) != 0 {
		t.Fatalf("preview marked read: %v", reader.marked)
	}
	m = drain(t, m, keyPress("enter"))
	if len(reader.marked) != 1 || reader.marked[0] != m.selectedID {
		t.Fatalf("marked: %v selected %d", reader.marked, m.selectedID)
	}
	if thread, ok := m.currentThread(); !ok || thread.Unread() {
		t.Fatalf("thread still unread after open: %+v", thread)
	}
	m = drain(t, m, RefreshMsg{})
	if len(reader.marked) != 1 {
		t.Fatalf("refresh re-marked read: %v", reader.marked)
	}
}
