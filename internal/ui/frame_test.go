package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func TestFrameSize(t *testing.T) {
	t.Parallel()
	styled := lipgloss.NewStyle().Foreground(accent).Render("styled text that is far too long for the frame")
	tests := []struct {
		name  string
		title string
		body  string
		w, h  int
	}{
		{"short", "[1] Chats", "a\nb", 20, 6},
		{"long title", "[2] a very long conversation title indeed", "x", 16, 3},
		{"overflowing body", "t", strings.Repeat(styled+"\n", 20), 24, 5},
		{"wide runes", "[2] 日本語のタイトル", "絵文字 👍 テスト", 14, 4},
	}
	for _, tt := range tests {
		got := strings.Split(frame(tt.title, true, tt.w, tt.h, tt.body), "\n")
		if len(got) != tt.h {
			t.Errorf("%s: %d rows, want %d", tt.name, len(got), tt.h)
		}
		for i, row := range got {
			if w := lipgloss.Width(row); w != tt.w {
				t.Errorf("%s: row %d width %d, want %d", tt.name, i, w, tt.w)
			}
		}
	}
}

func TestOverlayKeepsBaseAroundPopup(t *testing.T) {
	t.Parallel()
	base := strings.TrimSuffix(strings.Repeat(lipgloss.NewStyle().Foreground(muted).Render(strings.Repeat("x", 30))+"\n", 9), "\n")
	popup := "+----+\n|ok  |\n+----+"
	got := strings.Split(overlay(base, popup, 30, 9), "\n")
	if len(got) != 9 {
		t.Fatalf("rows: %d", len(got))
	}
	for i, row := range got {
		if w := lipgloss.Width(row); w != 30 {
			t.Errorf("row %d width %d", i, w)
		}
	}
	if plain := ansi.Strip(got[4]); plain != strings.Repeat("x", 12)+"|ok  |"+strings.Repeat("x", 12) {
		t.Errorf("middle row: %q", plain)
	}
	if plain := ansi.Strip(got[0]); plain != strings.Repeat("x", 30) {
		t.Errorf("untouched row: %q", plain)
	}
}
