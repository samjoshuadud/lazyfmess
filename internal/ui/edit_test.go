package ui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"lazyfmess/internal/store"
)

type storeEditor struct {
	s *store.Store
}

func (e storeEditor) Edit(ctx context.Context, _ store.Thread, msg store.Message, text string) error {
	return e.s.EditMessage(ctx, msg.ID, text)
}

func (e storeEditor) Unsend(ctx context.Context, _ store.Thread, msg store.Message) error {
	return e.s.UnsendMessage(ctx, msg.ID)
}

func modelWithOwnMessage(t *testing.T) Model {
	t.Helper()
	m := newTestModel(t, 1, 2)
	if err := m.store.UpsertMessage(context.Background(), store.Message{
		ID: "own", ThreadID: 1, SenderID: 1, Text: "helo", Timestamp: time.UnixMilli(1_700_000_100_000),
	}); err != nil {
		t.Fatalf("UpsertMessage: %v", err)
	}
	m.messageEditor = storeEditor{s: m.store}
	m = drain(t, m, RefreshMsg{})
	return drain(t, m, keyPress("2"))
}

func TestEditOwnMessage(t *testing.T) {
	t.Parallel()
	m := modelWithOwnMessage(t)
	m = drain(t, m, keyPress("e"))
	if m.editing.ID != "own" || m.composer.Value() != "helo" || m.focus != focusComposer {
		t.Fatalf("edit mode: editing %q composer %q focus %v", m.editing.ID, m.composer.Value(), m.focus)
	}
	m.composer.SetValue("hello")
	m = drain(t, m, keyPress("enter"))
	own, _ := m.messageByID("own")
	if own.Text != "hello" || !own.Edited || m.editing.ID != "" {
		t.Fatalf("after edit: %+v editing %q", own, m.editing.ID)
	}
	if !strings.Contains(ansi.Strip(m.viewport.View()), "edited") {
		t.Fatal("edited marker not rendered")
	}
}

func TestUnsendNeedsConfirmation(t *testing.T) {
	t.Parallel()
	m := modelWithOwnMessage(t)
	m = drain(t, m, keyPress("d"))
	if m.popup == nil || m.popup.kind != popupConfirm {
		t.Fatalf("expected confirm popup, got %+v", m.popup)
	}
	m = drain(t, m, keyPress("esc"))
	if own, _ := m.messageByID("own"); own.Unsent {
		t.Fatal("cancel still unsent the message")
	}
	m = drain(t, m, keyPress("d"))
	m = drain(t, m, keyPress("y"))
	if own, _ := m.messageByID("own"); !own.Unsent {
		t.Fatalf("message not unsent: %+v", own)
	}
	if !strings.Contains(ansi.Strip(m.viewport.View()), "message unsent") {
		t.Fatal("unsent placeholder not rendered")
	}
}

func TestCannotEditOthersMessages(t *testing.T) {
	t.Parallel()
	m := modelWithOwnMessage(t)
	m = drain(t, m, keyPress("k"))
	for _, k := range []string{"e", "d"} {
		m = drain(t, m, keyPress(k))
	}
	if m.editing.ID != "" || m.popup != nil {
		t.Fatalf("edit or unsend allowed on another person's message: editing %q popup %+v", m.editing.ID, m.popup)
	}
}
