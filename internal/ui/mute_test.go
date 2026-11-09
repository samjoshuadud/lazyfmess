package ui

import (
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"lazyfmess/internal/store"
)

type storeMuter struct {
	s *store.Store
}

func (m storeMuter) SetMuted(ctx context.Context, thread store.Thread, muted bool) error {
	var until int64
	if muted {
		until = -1
	}
	return m.s.SetMute(ctx, thread.ID, until)
}

func TestMuteToggle(t *testing.T) {
	t.Parallel()
	m := newTestModel(t, 2, 0)
	m.muter = storeMuter{s: m.store}
	m = drain(t, m, keyPress("m"))
	if !strings.Contains(ansi.Strip(m.list.View()), "muted") {
		t.Fatal("muted marker missing after mute")
	}
	m = drain(t, m, keyPress("m"))
	if strings.Contains(ansi.Strip(m.list.View()), "muted") {
		t.Fatal("muted marker still shown after unmute")
	}
}
