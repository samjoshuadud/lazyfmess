package ui

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type threadDelegate struct{}

func (threadDelegate) Height() int                         { return 2 }
func (threadDelegate) Spacing() int                        { return 0 }
func (threadDelegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }

func (threadDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	t, ok := item.(threadItem)
	if !ok {
		return
	}
	width := m.Width() - 2
	selected := index == m.Index()

	marker := " "
	titleStyle := lipgloss.NewStyle().Foreground(text)
	previewStyle := lipgloss.NewStyle().Foreground(muted)
	if selected {
		marker = focusMark.Render("▌")
		titleStyle = titleStyle.Foreground(accent).Bold(true)
		previewStyle = previewStyle.Foreground(text)
	}

	when := ""
	if !t.thread.UpdatedAt.IsZero() {
		when = timeStyle.Render(shortTime(t.thread.UpdatedAt, time.Now()))
	}
	if t.thread.Unread() {
		titleStyle = titleStyle.Bold(true)
		previewStyle = previewStyle.Foreground(text)
		when = focusMark.Render("●") + " " + when
	}
	title := ansi.Truncate(t.thread.Title, width-lipgloss.Width(when)-1, "…")
	gap := max(width-lipgloss.Width(title)-lipgloss.Width(when), 1)
	preview := ansi.Truncate(strings.Join(strings.Fields(t.thread.LastPreview), " "), width, "…")

	fmt.Fprintf(w, "%s%s%s%s\n%s%s", marker, titleStyle.Render(title), strings.Repeat(" ", gap), when, marker, previewStyle.Render(preview))
}

func shortTime(t, now time.Time) string {
	t, now = t.Local(), now.Local()
	switch {
	case sameDay(t, now):
		return t.Format("15:04")
	case now.Sub(t) < 6*24*time.Hour && t.Before(now):
		return t.Format("Mon")
	case t.Year() == now.Year():
		return t.Format("Jan 2")
	default:
		return t.Format("Jan 2 2006")
	}
}

func dayLabel(t, now time.Time) string {
	t, now = t.Local(), now.Local()
	switch {
	case sameDay(t, now):
		return "Today"
	case sameDay(t, now.AddDate(0, 0, -1)):
		return "Yesterday"
	case t.Year() == now.Year():
		return t.Format("Mon, Jan 2")
	default:
		return t.Format("Mon, Jan 2 2006")
	}
}

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}
