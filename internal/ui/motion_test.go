package ui

import (
	"fmt"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func press(m Model, t *testing.T, keys ...string) Model {
	t.Helper()
	for _, k := range keys {
		switch k {
		case "ctrl+d":
			m = drain(t, m, tea.KeyMsg{Type: tea.KeyCtrlD})
		case "ctrl+u":
			m = drain(t, m, tea.KeyMsg{Type: tea.KeyCtrlU})
		case "ctrl+f":
			m = drain(t, m, tea.KeyMsg{Type: tea.KeyCtrlF})
		case "ctrl+b":
			m = drain(t, m, tea.KeyMsg{Type: tea.KeyCtrlB})
		default:
			m = drain(t, m, keyPress(k))
		}
	}
	return m
}

func TestMessageMotions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		keys []string
		want string
	}{
		{"gg goes to the first message", []string{"g", "g"}, "m1-0"},
		{"count moves down", []string{"g", "g", "5", "j"}, "m1-5"},
		{"count moves up", []string{"7", "k"}, "m1-32"},
		{"multi digit count clamps", []string{"4", "0", "k"}, "m1-0"},
		{"count above the end clamps", []string{"g", "g", "9", "9", "j"}, "m1-39"},
		{"lone g waits for a second key", []string{"g"}, "m1-39"},
		{"unknown chord is dropped", []string{"g", "x", "k"}, "m1-38"},
		{"esc cancels a pending prefix", []string{"g", "esc", "2"}, ""},
	}
	for _, tt := range tests {
		m := newTestModel(t, 1, 40)
		m = drain(t, m, keyPress("2"))
		m = press(m, t, tt.keys...)
		if tt.want != "" && m.cursorID != tt.want {
			t.Errorf("%s: cursor %q, want %q", tt.name, m.cursorID, tt.want)
		}
	}
}

func TestViewportMotions(t *testing.T) {
	t.Parallel()
	m := newTestModel(t, 1, 80)
	m = drain(t, m, keyPress("2"))
	m = press(m, t, "g", "g", "4", "0", "j")
	m = press(m, t, "z", "t")
	if span := m.spans[m.cursorID]; span.start != m.viewport.YOffset {
		t.Fatalf("zt: span %+v offset %d", span, m.viewport.YOffset)
	}
	m = press(m, t, "z", "b")
	if span := m.spans[m.cursorID]; span.end != m.viewport.YOffset+m.viewport.Height-1 {
		t.Fatalf("zb: span %+v offset %d height %d", span, m.viewport.YOffset, m.viewport.Height)
	}
	m = press(m, t, "H")
	visible := m.visibleMessages()
	if m.cursorID != visible[0] {
		t.Fatalf("H: cursor %s, first visible %s", m.cursorID, visible[0])
	}
	m = press(m, t, "L")
	if m.cursorID != visible[len(visible)-1] {
		t.Fatalf("L: cursor %s, last visible %s", m.cursorID, visible[len(visible)-1])
	}
	m = press(m, t, "M")
	if m.cursorID != visible[len(visible)/2] {
		t.Fatalf("M: cursor %s", m.cursorID)
	}
	for _, k := range []string{"ctrl+d", "ctrl+f", "ctrl+u", "ctrl+b"} {
		m = press(m, t, k)
		span := m.spans[m.cursorID]
		if span.end < m.viewport.YOffset || span.start >= m.viewport.YOffset+m.viewport.Height {
			t.Fatalf("%s: cursor %s outside the viewport", k, m.cursorID)
		}
	}
}

func TestChatSearch(t *testing.T) {
	t.Parallel()
	m := newTestModel(t, 1, 40)
	m = drain(t, m, keyPress("2"))
	m = press(m, t, "/")
	if m.popup == nil || m.popup.kind != popupPrompt {
		t.Fatalf("expected the search prompt, got %+v", m.popup)
	}
	m = drain(t, m, keyPress("line 3"))
	m = drain(t, m, keyPress("enter"))
	if m.search != "line 3" || m.cursorID != "m1-3" {
		t.Fatalf("after search: query %q cursor %q", m.search, m.cursorID)
	}
	var seen []string
	for range 12 {
		m = press(m, t, "n")
		seen = append(seen, m.cursorID)
	}
	want := []string{"m1-30", "m1-31", "m1-32", "m1-33", "m1-34", "m1-35", "m1-36", "m1-37", "m1-38", "m1-39", "m1-3", "m1-30"}
	if fmt.Sprint(seen) != fmt.Sprint(want) {
		t.Fatalf("n sequence %v, want %v", seen, want)
	}
	m = press(m, t, "N")
	if m.cursorID != "m1-3" {
		t.Fatalf("N wraps back to %q", m.cursorID)
	}
	m = press(m, t, "N")
	if m.cursorID != "m1-39" {
		t.Fatalf("N from the first match: %q", m.cursorID)
	}
}

func TestChatSearchWithoutMatches(t *testing.T) {
	t.Parallel()
	m := newTestModel(t, 1, 5)
	m = drain(t, m, keyPress("2"))
	m = press(m, t, "/")
	m = drain(t, m, keyPress("zzz"))
	m = drain(t, m, keyPress("enter"))
	if m.search != "" || m.status == "" {
		t.Fatalf("query %q status %q", m.search, m.status)
	}
}

func TestInboxMotions(t *testing.T) {
	t.Parallel()
	m := newTestModel(t, 6, 1)
	m = press(m, t, "1", "g", "g")
	first := m.selectedID
	m = press(m, t, "4", "j")
	if m.selectedID == first {
		t.Fatal("4j should move the inbox selection")
	}
	m = press(m, t, "G")
	last := m.selectedID
	m = press(m, t, "g", "g")
	if m.selectedID != first || last == first {
		t.Fatalf("gg selected %d, want %d (last %d)", m.selectedID, first, last)
	}
}

func TestJumpUnreadChat(t *testing.T) {
	t.Parallel()
	m := newTestModel(t, 4, 1)
	m = press(m, t, "1")
	start := m.selectedID
	m = press(m, t, "]", "u")
	if m.selectedID == start || m.focus != focusMessages || m.openedID != m.selectedID {
		t.Fatalf("]u: selected %d opened %d focus %v", m.selectedID, m.openedID, m.focus)
	}
	back := m.selectedID
	m = press(m, t, "[", "u")
	if m.selectedID == back {
		t.Fatal("[u should move to another unread chat")
	}
}

func TestMediaMotion(t *testing.T) {
	t.Parallel()
	m := newTestModel(t, 1, 3)
	m = drain(t, m, keyPress("2"))
	m = press(m, t, "]", "m")
	if m.status != "no more media in this chat" {
		t.Fatalf("status %q", m.status)
	}
}

func TestPendingMotionLabel(t *testing.T) {
	t.Parallel()
	tests := []struct {
		keys []string
		want string
	}{
		{[]string{"5"}, "5"},
		{[]string{"1", "2"}, ""},
		{[]string{"1", "2", "g"}, "g"},
		{[]string{"4", "2", "g"}, "42g"},
		{[]string{"z"}, "z"},
		{[]string{"4", "j"}, ""},
	}
	for _, tt := range tests {
		m := newTestModel(t, 1, 3)
		m = drain(t, m, keyPress("2"))
		m = press(m, t, tt.keys...)
		if got := m.pending.label(); got != tt.want {
			t.Errorf("%v: label %q, want %q", tt.keys, got, tt.want)
		}
	}
}
