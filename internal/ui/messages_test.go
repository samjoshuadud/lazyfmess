package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"lazyfmess/internal/store"
)

func TestSeenStatus(t *testing.T) {
	t.Parallel()
	base := time.UnixMilli(1_700_000_000_000)
	msgs := []store.Message{
		{ID: "a", SenderID: 1, Timestamp: base},
		{ID: "b", SenderID: 2, Timestamp: base.Add(time.Minute)},
		{ID: "c", SenderID: 1, Timestamp: base.Add(2 * time.Minute)},
		{ID: "d", SenderID: 1, Timestamp: base.Add(3 * time.Minute), Pending: true},
	}
	tests := []struct {
		name      string
		marks     []time.Time
		wantID    string
		wantLabel string
	}{
		{"nobody read", []time.Time{base.Add(time.Minute)}, "", ""},
		{"one reader", []time.Time{base.Add(2 * time.Minute)}, "c", "Seen"},
		{"group", []time.Time{base.Add(5 * time.Minute), base.Add(3 * time.Minute), base}, "c", "Seen by 2"},
	}
	for _, tt := range tests {
		id, label := seenStatus(msgs, 1, tt.marks)
		if id != tt.wantID || label != tt.wantLabel {
			t.Errorf("%s: got (%q, %q) want (%q, %q)", tt.name, id, label, tt.wantID, tt.wantLabel)
		}
	}
}

func TestSystemMessageLine(t *testing.T) {
	t.Parallel()
	base := time.UnixMilli(1_700_000_000_000)
	msgs := []store.Message{
		{ID: "a", SenderID: 2, SenderName: "Dana", Text: "hello", Timestamp: base},
		{ID: "b", SenderID: 1, Text: "named the group Hikers", System: true, Timestamp: base.Add(time.Minute)},
		{ID: "c", SenderID: 2, SenderName: "Dana", Text: "nice", Timestamp: base.Add(2 * time.Minute)},
	}
	out := renderMessages(msgs, 60, 1, "", nil, time.Time{}, nil).content
	if !strings.Contains(out, "You named the group Hikers") {
		t.Fatalf("system line missing:\n%s", out)
	}
	if strings.Count(out, "Dana") != 2 {
		t.Fatalf("a system line should start a new sender group:\n%s", out)
	}
}

func bigChat(n int) []store.Message {
	base := time.UnixMilli(1_700_000_000_000)
	msgs := make([]store.Message, n)
	for i := range msgs {
		msgs[i] = store.Message{ID: fmt.Sprintf("m%d", i), SenderID: int64(i%3 + 1), SenderName: "Dana", Text: "a message long enough to wrap once or twice inside the chat pane at normal widths", Timestamp: base.Add(time.Duration(i) * 3 * time.Minute)}
	}
	return msgs
}

func TestRenderCacheReusesBlocks(t *testing.T) {
	t.Parallel()
	msgs := bigChat(200)
	cache := map[string]string{}
	first := renderMessages(msgs, 80, 1, "m10", nil, time.Time{}, cache)
	if len(cache) != len(msgs) {
		t.Fatalf("cache size %d, want %d", len(cache), len(msgs))
	}
	moved := renderMessages(msgs, 80, 1, "m11", nil, time.Time{}, cache)
	if len(cache) != len(msgs) {
		t.Fatalf("a cursor move rendered new blocks: cache size %d", len(cache))
	}
	uncached := renderMessages(msgs, 80, 1, "m11", nil, time.Time{}, nil)
	if moved.content != uncached.content || first.content == moved.content {
		t.Fatal("cached render differs from a fresh render")
	}
	msgs[50].Edited = true
	renderMessages(msgs, 80, 1, "m11", nil, time.Time{}, cache)
	if len(cache) != len(msgs)+1 {
		t.Fatalf("an edit should re-render exactly one block, cache size %d", len(cache))
	}
}

func BenchmarkRenderCursorMove(b *testing.B) {
	msgs := bigChat(5000)
	cache := map[string]string{}
	renderMessages(msgs, 100, 1, "m0", nil, time.Time{}, cache)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		renderMessages(msgs, 100, 1, fmt.Sprintf("m%d", i%5000), nil, time.Time{}, cache)
	}
}

func BenchmarkRenderUncached(b *testing.B) {
	msgs := bigChat(5000)
	for i := 0; i < b.N; i++ {
		renderMessages(msgs, 100, 1, "m0", nil, time.Time{}, nil)
	}
}

func TestWrapWords(t *testing.T) {
	t.Parallel()
	family := "👨‍👩‍👧‍👦"
	tests := []struct {
		name  string
		in    string
		width int
		want  []string
	}{
		{"short", "hello world", 20, []string{"hello world"}},
		{"wraps at spaces", "aaa bbb ccc", 8, []string{"aaa bbb", "ccc"}},
		{"keeps inner spaces", "a    b", 20, []string{"a    b"}},
		{"keeps indentation", "    code()", 20, []string{"    code()"}},
		{"long word", "abcdefghijkl", 8, []string{"abcdefgh", "ijkl"}},
		{"emoji clusters stay whole", family + family + family + family + family, 8, []string{family + family + family + family, family}},
		{"wide characters", "日本語テストです", 8, []string{"日本語テ", "ストです"}},
		{"blank line", "a\n\nb", 20, []string{"a", "", "b"}},
	}
	for _, tt := range tests {
		got := wrapWords(tt.in, tt.width)
		if strings.Join(got, "|") != strings.Join(tt.want, "|") {
			t.Errorf("%s: got %q want %q", tt.name, got, tt.want)
		}
		for _, line := range got {
			if lipgloss.Width(line) > max(tt.width, 8) {
				t.Errorf("%s: line %q wider than %d", tt.name, line, tt.width)
			}
		}
	}
}

func TestOwnBlockIsAlignedAsAWhole(t *testing.T) {
	t.Parallel()
	msgs := []store.Message{{ID: "a", SenderID: 1, Text: "a long message that wraps across several lines in the pane", Timestamp: time.UnixMilli(1)}}
	out := renderMessages(msgs, 60, 1, "", nil, time.Time{}, nil).content
	col := -1
	for _, line := range strings.Split(ansi.Strip(out), "\n") {
		i := strings.Index(line, "▏")
		if i < 0 {
			continue
		}
		if col >= 0 && i != col {
			t.Fatalf("bar moved from column %d to %d:\n%s", col, i, ansi.Strip(out))
		}
		col = i
	}
	if col <= 0 {
		t.Fatalf("own message should be right-aligned with a bar:\n%s", ansi.Strip(out))
	}
}

func TestNewMessagesRule(t *testing.T) {
	t.Parallel()
	base := time.UnixMilli(1_700_000_000_000)
	msgs := []store.Message{
		{ID: "a", SenderID: 2, SenderName: "Dana", Text: "old", Timestamp: base},
		{ID: "b", SenderID: 2, SenderName: "Dana", Text: "fresh", Timestamp: base.Add(time.Minute)},
		{ID: "c", SenderID: 1, SenderName: "Pat", Text: "mine", Timestamp: base.Add(2 * time.Minute)},
	}
	tests := []struct {
		name  string
		after time.Time
		want  bool
	}{
		{"unread from someone else", base.Add(10 * time.Second), true},
		{"everything read", time.Time{}, false},
		{"only own messages newer", base.Add(90 * time.Second), false},
	}
	for _, tt := range tests {
		out := renderMessages(msgs, 60, 1, "", nil, tt.after, nil).content
		lines := strings.Split(out, "\n")
		at := -1
		for i, l := range lines {
			if strings.Contains(l, "New messages") {
				at = i
			}
		}
		if (at >= 0) != tt.want {
			t.Errorf("%s: rule present %v, want %v", tt.name, at >= 0, tt.want)
			continue
		}
		if tt.want && !strings.Contains(strings.Join(lines[at:], "\n"), "fresh") || tt.want && strings.Contains(strings.Join(lines[at:], "\n"), "old") {
			t.Errorf("%s: rule not placed between old and fresh", tt.name)
		}
	}
}
