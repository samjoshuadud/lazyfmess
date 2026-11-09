package ui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"lazyfmess/internal/store"
)

func TestConnectingState(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		err       error
		wantPopup bool
	}{
		{"connected", nil, false},
		{"failed", errors.New("session expired"), true},
	}
	for _, tt := range tests {
		ctx := context.Background()
		s, err := store.Open(ctx, t.TempDir())
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		m := New(ctx, s, Services{Sender: stubSender{id: 1}}, Config{Connecting: true})
		model, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
		m = model.(Model)
		if !strings.Contains(ansi.Strip(m.View()), "connecting to Messenger") {
			t.Fatalf("%s: connecting state not shown", tt.name)
		}
		model, _ = m.Update(connectedMsg{err: tt.err})
		m = model.(Model)
		if m.connecting || m.inFlight != 0 {
			t.Fatalf("%s: connecting %v inFlight %d", tt.name, m.connecting, m.inFlight)
		}
		if (m.popup != nil) != tt.wantPopup {
			t.Fatalf("%s: popup %+v", tt.name, m.popup)
		}
		s.Close()
	}
}
