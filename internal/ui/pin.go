package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type pinModel struct {
	input textinput.Model
	err   string
	done  bool
	value string
	skip  bool
}

func PromptPIN() (string, error) {
	m := pinModel{input: newPINInput()}
	p := tea.NewProgram(m)
	final, err := p.Run()
	if err != nil {
		return "", fmt.Errorf("pin prompt: %w", err)
	}
	result, ok := final.(pinModel)
	if !ok {
		return "", fmt.Errorf("pin prompt: unexpected model")
	}
	if result.skip {
		return "", nil
	}
	return result.value, nil
}

func (m pinModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m pinModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			m.skip = true
			m.done = true
			return m, tea.Quit
		case "enter":
			value := strings.TrimSpace(m.input.Value())
			if value == "" {
				m.skip = true
				m.done = true
				return m, tea.Quit
			}
			if len(value) != 6 {
				m.err = "enter exactly 6 digits, or leave empty to skip"
				return m, nil
			}
			for _, r := range value {
				if r < '0' || r > '9' {
					m.err = "PIN must be digits only"
					return m, nil
				}
			}
			m.value = value
			m.done = true
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m pinModel) View() string {
	title := titleStyle.Render("lazyfmess")
	hint := statusStyle.Render("Secure storage PIN. Shown as ******. Empty skips history unlock.")
	body := m.input.View()
	errLine := ""
	if m.err != "" {
		errLine = errorStatusStyle.Render(m.err)
	}
	help := helpStyle.Render("enter confirm · esc skip")
	return lipgloss.JoinVertical(
		lipgloss.Left,
		"",
		"  "+title,
		"  "+hint,
		"",
		"  "+body,
		"  "+errLine,
		"",
		"  "+help,
		"",
	)
}
