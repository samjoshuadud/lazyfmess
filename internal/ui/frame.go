package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func frame(title string, focused bool, w, h int, body string) string {
	if w < 4 || h < 2 {
		return ""
	}
	color := line
	titleStyle := lipgloss.NewStyle().Foreground(muted)
	if focused {
		color = accent
		titleStyle = lipgloss.NewStyle().Foreground(accent).Bold(true)
	}
	edge := lipgloss.NewStyle().Foreground(color)
	border := lipgloss.RoundedBorder()

	innerW, innerH := w-2, h-2
	title = ansi.Truncate(title, innerW-2, "…")
	fill := innerW - 1 - lipgloss.Width(title)
	top := edge.Render(border.TopLeft+border.Top) + titleStyle.Render(title) + edge.Render(strings.Repeat(border.Top, fill)+border.TopRight)

	rows := strings.Split(body, "\n")
	out := make([]string, 0, h)
	out = append(out, top)
	side := edge.Render(border.Left)
	for i := 0; i < innerH; i++ {
		row := ""
		if i < len(rows) {
			row = ansi.Truncate(rows[i], innerW, "")
		}
		pad := innerW - lipgloss.Width(row)
		out = append(out, side+row+strings.Repeat(" ", pad)+edge.Render(border.Right))
	}
	out = append(out, edge.Render(border.BottomLeft+strings.Repeat(border.Bottom, innerW)+border.BottomRight))
	return strings.Join(out, "\n")
}

func overlay(base, popup string, w, h int) string {
	rows := strings.Split(base, "\n")
	for len(rows) < h {
		rows = append(rows, "")
	}
	popupRows := strings.Split(popup, "\n")
	popupW := lipgloss.Width(popup)
	top := max((h-len(popupRows))/2, 0)
	left := max((w-popupW)/2, 0)
	for i, p := range popupRows {
		y := top + i
		if y >= len(rows) {
			break
		}
		row := rows[y]
		if gap := left - lipgloss.Width(row); gap > 0 {
			row += strings.Repeat(" ", gap)
		}
		p = ansi.Truncate(p, w-left, "")
		right := ansi.TruncateLeft(row, left+lipgloss.Width(p), "")
		rows[y] = ansi.Truncate(row, left, "") + "\x1b[0m" + p + "\x1b[0m" + right
	}
	return strings.Join(rows, "\n")
}
