package ui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"lazyfmess/internal/store"
)

func keyPress(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func TestPopupKeys(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		open     func(*Model)
		keys     []string
		wantOpen bool
		wantHit  string
	}{
		{"error dismiss enter", func(m *Model) { m.showError("boom") }, []string{"enter"}, false, ""},
		{"error ignores letters", func(m *Model) { m.showError("boom") }, []string{"j", "q"}, true, ""},
		{"confirm yes", nil, []string{"y"}, false, "confirmed"},
		{"confirm cancel", nil, []string{"esc"}, false, ""},
		{"menu second item", nil, []string{"j", "enter"}, false, "second"},
		{"menu clamps", nil, []string{"k", "k", "enter"}, false, "first"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m := Model{keys: newKeyMap(), width: 80, height: 24}
			hit := ""
			record := func(label string) func(*Model) tea.Cmd {
				return func(*Model) tea.Cmd { hit = label; return nil }
			}
			switch {
			case tt.open != nil:
				tt.open(&m)
			case strings.HasPrefix(tt.name, "confirm"):
				m.showConfirm("Unsend", "sure?", record("confirmed"))
			default:
				m.showMenu("Actions", []menuItem{{"first", record("first")}, {"second", record("second")}})
			}
			for _, k := range tt.keys {
				m, _ = m.updatePopup(keyPress(k))
			}
			if (m.popup != nil) != tt.wantOpen {
				t.Fatalf("popup open = %v, want %v", m.popup != nil, tt.wantOpen)
			}
			if hit != tt.wantHit {
				t.Fatalf("action = %q, want %q", hit, tt.wantHit)
			}
		})
	}
}

type failingSender struct{}

func (failingSender) AccountID() int64 { return 1 }
func (failingSender) SendText(context.Context, store.Thread, string, store.Message) (SendResult, error) {
	return SendResult{}, errors.New("network down")
}

func TestSendFailureOpensErrorPopup(t *testing.T) {
	t.Parallel()
	m := newTestModel(t, 1, 0)
	m.sender = failingSender{}
	m = drain(t, m, keyPress("i"))
	m.composer.SetValue("hello")
	m = drain(t, m, keyPress("enter"))
	if m.popup == nil || m.popup.kind != popupError || !strings.Contains(m.popup.body, "network down") {
		t.Fatalf("popup: %+v", m.popup)
	}
	if len(m.messages) != 1 || m.messages[0].Error == "" || m.messages[0].Pending {
		t.Fatalf("failed message not kept: %+v", m.messages)
	}
	if !strings.Contains(m.View(), "network down") {
		t.Fatal("error popup not rendered")
	}
}
