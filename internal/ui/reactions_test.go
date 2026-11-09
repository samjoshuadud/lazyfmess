package ui

import (
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"lazyfmess/internal/store"
)

type storeReactor struct {
	s *store.Store
}

func (r storeReactor) React(ctx context.Context, _ store.Thread, msg store.Message, emoji string) error {
	return r.s.SetReaction(ctx, store.Reaction{MessageID: msg.ID, ActorID: 1, Emoji: emoji})
}

func TestReactAndToggleOff(t *testing.T) {
	t.Parallel()
	m := newTestModel(t, 1, 2)
	m.reactor = storeReactor{s: m.store}
	m = drain(t, m, keyPress("2"))
	m = drain(t, m, keyPress("+"))
	if m.popup == nil || m.popup.kind != popupMenu || len(m.popup.items) != len(quickReactions) {
		t.Fatalf("reaction menu: %+v", m.popup)
	}
	m = drain(t, m, keyPress("j"))
	m = drain(t, m, keyPress("enter"))
	msg, _ := m.cursorMessage()
	if got := reactionSummary(msg.Reactions); got != "❤ 1" {
		t.Fatalf("after react: %q", got)
	}
	if !strings.Contains(ansi.Strip(m.viewport.View()), "❤ 1") {
		t.Fatal("reaction summary not rendered")
	}
	m = drain(t, m, keyPress("+"))
	if !strings.HasSuffix(m.popup.items[1].label, "(remove)") {
		t.Fatalf("own reaction not marked: %q", m.popup.items[1].label)
	}
	m = drain(t, m, keyPress("j"))
	m = drain(t, m, keyPress("enter"))
	msg, _ = m.cursorMessage()
	if len(msg.Reactions) != 0 {
		t.Fatalf("reaction not removed: %+v", msg.Reactions)
	}
}

func TestReactionSummary(t *testing.T) {
	t.Parallel()
	got := reactionSummary([]store.Reaction{{Emoji: "👍"}, {Emoji: "❤"}, {Emoji: "👍"}})
	if got != "👍 2  ❤ 1" {
		t.Fatalf("summary: %q", got)
	}
}
