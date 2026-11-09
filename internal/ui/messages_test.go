package ui

import (
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
