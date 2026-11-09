package ui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"lazyfmess/internal/store"
)

func TestMessageCursorStaysVisible(t *testing.T) {
	t.Parallel()
	m := newTestModel(t, 1, 40)
	m = drain(t, m, keyPress("2"))
	if m.cursorID != "m1-39" {
		t.Fatalf("cursor starts at %q, want last message", m.cursorID)
	}
	for range 30 {
		m = drain(t, m, keyPress("k"))
	}
	if m.cursorID != "m1-9" {
		t.Fatalf("cursor after 30 up: %q", m.cursorID)
	}
	span := m.spans[m.cursorID]
	if span.start < m.viewport.YOffset || span.end >= m.viewport.YOffset+m.viewport.Height {
		t.Fatalf("cursor span %+v outside viewport offset %d height %d", span, m.viewport.YOffset, m.viewport.Height)
	}
	m = drain(t, m, keyPress("G"))
	if m.cursorID != "m1-39" || !m.viewport.AtBottom() {
		t.Fatalf("G: cursor %q atBottom %v", m.cursorID, m.viewport.AtBottom())
	}
}

func TestActionMenuForMediaMessage(t *testing.T) {
	t.Parallel()
	m := newTestModel(t, 1, 2)
	err := m.store.UpsertMessage(context.Background(), store.Message{
		ID: "img", ThreadID: 1, SenderID: 2, Text: "look", Timestamp: time.UnixMilli(1_700_000_100_000),
		MediaKind: "image", MediaURL: "https://example.test/a.jpg",
	})
	if err != nil {
		t.Fatalf("UpsertMessage: %v", err)
	}
	m = drain(t, m, RefreshMsg{})
	m = drain(t, m, keyPress("2"))
	m = drain(t, m, keyPress("enter"))
	if m.popup == nil || m.popup.kind != popupMenu {
		t.Fatalf("expected action menu, got %+v", m.popup)
	}
	var labels []string
	for _, item := range m.popup.items {
		labels = append(labels, item.label)
	}
	if got := strings.Join(labels, ","); got != "Reply,Open [image],Copy text" {
		t.Fatalf("menu items: %s", got)
	}
}

func TestComposerNewlineKey(t *testing.T) {
	t.Parallel()
	m := newTestModel(t, 1, 0)
	m = drain(t, m, keyPress("i"))
	for _, msg := range []tea.KeyMsg{keyPress("a"), {Type: tea.KeyCtrlJ}, keyPress("b")} {
		model, _ := m.Update(msg)
		m = model.(Model)
	}
	if got := m.composer.Value(); got != "a\nb" {
		t.Fatalf("composer: %q", got)
	}
}

func TestMouseClickFocusesPanel(t *testing.T) {
	t.Parallel()
	m := newTestModel(t, 1, 0)
	tests := []struct {
		x, y int
		want focusPane
	}{
		{m.leftWidth + 5, m.height - 3, focusComposer},
		{m.leftWidth + 5, 2, focusMessages},
		{2, 2, focusList},
	}
	for _, tt := range tests {
		model, _ := m.Update(tea.MouseMsg{X: tt.x, Y: tt.y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
		m = model.(Model)
		if m.focus != tt.want {
			t.Errorf("click (%d,%d): focus %v, want %v", tt.x, tt.y, m.focus, tt.want)
		}
	}
}
