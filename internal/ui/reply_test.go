package ui

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"lazyfmess/internal/store"
)

type replySender struct {
	mu      sync.Mutex
	replyTo store.Message
}

func (r *replySender) AccountID() int64 { return 1 }

func (r *replySender) SendText(_ context.Context, _ store.Thread, _ string, replyTo store.Message) (SendResult, error) {
	r.mu.Lock()
	r.replyTo = replyTo
	r.mu.Unlock()
	return SendResult{MessageID: "sent-reply", Timestamp: time.UnixMilli(1_700_000_900_000)}, nil
}

func (r *replySender) SendFile(context.Context, store.Thread, string) (SendResult, error) {
	return SendResult{}, nil
}

func TestReplyFlow(t *testing.T) {
	t.Parallel()
	m := newTestModel(t, 1, 3)
	sender := &replySender{}
	m.sender = sender
	m = drain(t, m, keyPress("2"))
	m = drain(t, m, keyPress("k"))
	target := m.cursorID
	m = drain(t, m, keyPress("r"))
	if m.focus != focusComposer || m.replyTo.ID != target {
		t.Fatalf("reply mode: focus %v replyTo %q want %q", m.focus, m.replyTo.ID, target)
	}
	if !strings.Contains(ansi.Strip(m.View()), "Reply to") {
		t.Fatal("compose title missing reply banner")
	}
	m.composer.SetValue("sure")
	m = drain(t, m, keyPress("enter"))
	if sender.replyTo.ID != target || sender.replyTo.SenderID != 2 {
		t.Fatalf("sender got replyTo %+v", sender.replyTo)
	}
	if m.replyTo.ID != "" {
		t.Fatal("reply mode should clear after send")
	}
	last := m.messages[len(m.messages)-1]
	if last.ID != "sent-reply" || last.ReplyToID != target || last.ReplyText == "" {
		t.Fatalf("sent message: %+v", last)
	}
	if !strings.Contains(ansi.Strip(m.viewport.View()), "↪ 2: line 1") {
		t.Fatalf("quote not rendered:\n%s", ansi.Strip(m.viewport.View()))
	}
}

func TestEscCancelsReplyBeforeLeavingComposer(t *testing.T) {
	t.Parallel()
	m := newTestModel(t, 1, 2)
	m = drain(t, m, keyPress("2"))
	m = drain(t, m, keyPress("r"))
	m = drain(t, m, keyPress("esc"))
	if m.replyTo.ID != "" || m.focus != focusComposer {
		t.Fatalf("first esc: replyTo %q focus %v", m.replyTo.ID, m.focus)
	}
	m = drain(t, m, keyPress("esc"))
	if m.focus != focusList {
		t.Fatalf("second esc focus: %v", m.focus)
	}
}
