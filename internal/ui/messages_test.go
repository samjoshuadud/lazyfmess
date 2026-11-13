package ui

import (
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
	out := renderMessages(msgs, 60, 1, "", nil).content
	if !strings.Contains(out, "You named the group Hikers") {
		t.Fatalf("system line missing:\n%s", out)
	}
	if strings.Count(out, "Dana") != 2 {
		t.Fatalf("a system line should start a new sender group:\n%s", out)
	}
}
