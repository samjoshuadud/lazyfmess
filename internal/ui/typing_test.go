package ui

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"lazyfmess/internal/store"
)

type recordingTyping struct {
	mu    sync.Mutex
	calls []bool
}

func (r *recordingTyping) SetTyping(_ context.Context, _ store.Thread, active bool) error {
	r.mu.Lock()
	r.calls = append(r.calls, active)
	r.mu.Unlock()
	return nil
}

func TestTypingIndicatorShowsAndExpires(t *testing.T) {
	t.Parallel()
	m := newTestModel(t, 1, 2)
	model, _ := m.Update(typingMsg{threadID: 1, senderID: 2, active: true})
	m = model.(Model)
	if got := m.typingLabel(); !strings.Contains(got, "is typing") {
		t.Fatalf("label: %q", got)
	}
	if !strings.Contains(m.View(), "is typing") {
		t.Fatal("typing label not rendered")
	}
	m.expireTyping(time.Now().Add(typingTimeout + time.Second))
	if got := m.typingLabel(); got != "" {
		t.Fatalf("label after expiry: %q", got)
	}
	model, _ = m.Update(typingMsg{threadID: 1, senderID: 1, active: true})
	m = model.(Model)
	if got := m.typingLabel(); got != "" {
		t.Fatalf("own typing should be ignored: %q", got)
	}
}

func TestComposerSendsTypingState(t *testing.T) {
	t.Parallel()
	m := newTestModel(t, 1, 0)
	rec := &recordingTyping{}
	m.typingNotifier = rec
	for _, k := range []string{"i", "h", "e"} {
		m = drain(t, m, keyPress(k))
	}
	m = drain(t, m, keyPress("esc"))
	if got := rec.calls; len(got) != 2 || !got[0] || got[1] {
		t.Fatalf("typing calls: %v, want [true false]", got)
	}
}
