package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

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
	out := renderMessages(msgs, 60, 1, "", nil, nil).content
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
	first := renderMessages(msgs, 80, 1, "m10", nil, cache)
	if len(cache) != len(msgs) {
		t.Fatalf("cache size %d, want %d", len(cache), len(msgs))
	}
	moved := renderMessages(msgs, 80, 1, "m11", nil, cache)
	if len(cache) != len(msgs) {
		t.Fatalf("a cursor move rendered new blocks: cache size %d", len(cache))
	}
	uncached := renderMessages(msgs, 80, 1, "m11", nil, nil)
	if moved.content != uncached.content || first.content == moved.content {
		t.Fatal("cached render differs from a fresh render")
	}
	msgs[50].Edited = true
	renderMessages(msgs, 80, 1, "m11", nil, cache)
	if len(cache) != len(msgs)+1 {
		t.Fatalf("an edit should re-render exactly one block, cache size %d", len(cache))
	}
}

func BenchmarkRenderCursorMove(b *testing.B) {
	msgs := bigChat(5000)
	cache := map[string]string{}
	renderMessages(msgs, 100, 1, "m0", nil, cache)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		renderMessages(msgs, 100, 1, fmt.Sprintf("m%d", i%5000), nil, cache)
	}
}

func BenchmarkRenderUncached(b *testing.B) {
	msgs := bigChat(5000)
	for i := 0; i < b.N; i++ {
		renderMessages(msgs, 100, 1, "m0", nil, nil)
	}
}
