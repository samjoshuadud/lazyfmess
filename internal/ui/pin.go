package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type pinResultMsg struct {
	err     error
	skipped bool
}

func (m *Model) openPINModal() {
	m.showPIN = true
	m.pinErr = ""
	m.pinInput = newPINInput()
	m.composer.Blur()
}

func (m *Model) closePINModal() {
	m.showPIN = false
	m.pinErr = ""
	m.pinInput.Blur()
	m.applyFocus()
}

func (m Model) updatePIN(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keys.ForceQuit):
			return m, tea.Quit
		case key.Matches(msg, m.keys.Skip):
			m.closePINModal()
			m.status = "history unlock skipped"
			m.kickHistorySync()
			return m, nil
		case key.Matches(msg, m.keys.Unlock):
			value := strings.TrimSpace(m.pinInput.Value())
			if value == "" {
				m.closePINModal()
				m.status = "history unlock skipped"
				m.kickHistorySync()
				return m, nil
			}
			if len(value) != 6 {
				m.pinErr = "enter exactly 6 digits, or leave empty to skip"
				return m, nil
			}
			for _, r := range value {
				if r < '0' || r > '9' {
					m.pinErr = "PIN must be digits only"
					return m, nil
				}
			}
			m.status = "unlocking history"
			return m, m.track(m.submitPIN(value))
		}
	}
	var cmd tea.Cmd
	m.pinInput, cmd = m.pinInput.Update(msg)
	return m, cmd
}

func (m Model) submitPIN(pin string) tea.Cmd {
	return func() tea.Msg {
		if m.unlocker == nil {
			return pinResultMsg{skipped: true}
		}
		err := m.unlocker.Unlock(m.ctx, pin)
		return pinResultMsg{err: err}
	}
}

func (m Model) renderPINModal() string {
	width := 48
	if m.width > 0 && m.width < width+4 {
		width = m.width - 4
	}
	if width < 28 {
		width = 28
	}

	title := titleStyle.Render("Secure storage PIN")
	hint := statusStyle.Render("Encrypted history unlock. Input shows as ******.")
	body := m.pinInput.View()
	errLine := " "
	if m.pinErr != "" {
		errLine = errorStatusStyle.Render(m.pinErr)
	}
	help := renderHints(m.contextBindings(), width-4)

	return modalStyle.Width(width).Render(
		lipgloss.JoinVertical(
			lipgloss.Left,
			title,
			"",
			hint,
			"",
			body,
			errLine,
			"",
			help,
		),
	)
}
