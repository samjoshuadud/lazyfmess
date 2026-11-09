package ui

import (
	"testing"
	"time"
)

func TestShortTimeAndDayLabel(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.March, 12, 15, 0, 0, 0, time.Local)
	tests := []struct {
		at    time.Time
		short string
		day   string
	}{
		{time.Date(2026, time.March, 12, 9, 5, 0, 0, time.Local), "09:05", "Today"},
		{time.Date(2026, time.March, 11, 23, 0, 0, 0, time.Local), "Wed", "Yesterday"},
		{time.Date(2026, time.March, 8, 10, 0, 0, 0, time.Local), "Sun", "Sun, Mar 8"},
		{time.Date(2026, time.January, 2, 10, 0, 0, 0, time.Local), "Jan 2", "Fri, Jan 2"},
		{time.Date(2025, time.December, 31, 10, 0, 0, 0, time.Local), "Dec 31 2025", "Wed, Dec 31 2025"},
	}
	for _, tt := range tests {
		if got := shortTime(tt.at, now); got != tt.short {
			t.Errorf("shortTime(%v) = %q, want %q", tt.at, got, tt.short)
		}
		if got := dayLabel(tt.at, now); got != tt.day {
			t.Errorf("dayLabel(%v) = %q, want %q", tt.at, got, tt.day)
		}
	}
}
