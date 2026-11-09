package ui

import (
	"context"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type LoginStepKind int

const (
	LoginInput LoginStepKind = iota
	LoginWait
	LoginDone
	LoginUnsupported
)

type LoginField struct {
	ID          string
	Name        string
	Description string
	Secret      bool
	Options     []string
}

type LoginStep struct {
	Kind         LoginStepKind
	Instructions string
	Fields       []LoginField
	Attachment   string
	CanCancel    bool
}

type LoginService interface {
	StartAccount(ctx context.Context) (LoginStep, error)
	Submit(ctx context.Context, values map[string]string) (LoginStep, error)
	Wait(ctx context.Context) (LoginStep, error)
	CancelStep(ctx context.Context) (LoginStep, error)
	OpenAttachment(path string) error
	Abort()
	UseCookies(ctx context.Context, raw string) error
}

type loginScreen int

const (
	loginChoose loginScreen = iota
	loginForm
	loginWaiting
	loginPaste
	loginBusy
)

type loginChoice struct {
	label  string
	action func(*Model) tea.Cmd
}

type loginState struct {
	screen  loginScreen
	notice  string
	err     string
	choice  int
	step    LoginStep
	inputs  []textinput.Model
	options []int
	field   int
	paste   textarea.Model
}

type needLoginMsg struct {
	reason string
}

type loginStepMsg struct {
	step LoginStep
	err  error
}

type cookiesUsedMsg struct {
	err error
}

func (m Model) loginChoices() []loginChoice {
	return []loginChoice{
		{"Log in with Messenger account", func(m *Model) tea.Cmd { return m.startAccountLogin() }},
		{"Paste browser cookies", func(m *Model) tea.Cmd { m.openPaste(); return textarea.Blink }},
		{"Quit", func(*Model) tea.Cmd { return tea.Quit }},
	}
}

func (m *Model) openLogin(reason string) {
	if m.connecting {
		m.connecting = false
		m.inFlight--
	}
	m.popup = nil
	m.login = &loginState{screen: loginChoose, notice: reason}
}

func (m *Model) startAccountLogin() tea.Cmd {
	m.login.screen = loginBusy
	m.login.err = ""
	return m.loginCall(m.loginService.StartAccount)
}

func (m *Model) loginCall(call func(context.Context) (LoginStep, error)) tea.Cmd {
	if m.loginService == nil {
		m.login.screen = loginChoose
		m.login.err = "login is unavailable"
		return nil
	}
	return m.track(func() tea.Msg {
		step, err := call(m.ctx)
		return loginStepMsg{step: step, err: err}
	})
}

func (m *Model) openPaste() {
	paste := textarea.New()
	paste.Placeholder = "Paste the cookie export (JSON) here"
	paste.ShowLineNumbers = false
	paste.SetWidth(max(min(60, m.width-12), 20))
	paste.SetHeight(6)
	paste.CharLimit = 0
	paste.KeyMap.InsertNewline = key.NewBinding(key.WithKeys("alt+enter", "ctrl+j"))
	paste.Focus()
	m.login.paste = paste
	m.login.screen = loginPaste
	m.login.err = ""
}

func (m *Model) applyLoginStep(msg loginStepMsg) tea.Cmd {
	l := m.login
	if l == nil {
		return nil
	}
	if msg.err != nil {
		l.err = msg.err.Error()
		if l.screen == loginBusy || l.screen == loginWaiting {
			if l.step.Kind == LoginInput && len(l.inputs) > 0 {
				l.screen = loginForm
			} else {
				l.screen = loginChoose
			}
		}
		return nil
	}
	l.err = ""
	l.step = msg.step
	switch msg.step.Kind {
	case LoginDone:
		return m.finishLogin()
	case LoginWait:
		l.screen = loginWaiting
		return m.loginCall(m.loginService.Wait)
	default:
		l.screen = loginForm
		l.field = 0
		l.inputs = make([]textinput.Model, len(msg.step.Fields))
		l.options = make([]int, len(msg.step.Fields))
		for i, f := range msg.step.Fields {
			in := textinput.New()
			in.Prompt = ""
			in.Placeholder = f.Name
			in.Width = max(min(48, m.width-20), 12)
			in.TextStyle = lipgloss.NewStyle().Foreground(text)
			in.PlaceholderStyle = lipgloss.NewStyle().Foreground(muted)
			if f.Secret {
				in.EchoMode = textinput.EchoPassword
				in.EchoCharacter = '•'
			}
			l.inputs[i] = in
		}
		return m.focusLoginField(0)
	}
}

func (m *Model) finishLogin() tea.Cmd {
	m.login = nil
	m.connecting = true
	m.inFlight++
	m.status = "logged in"
	return tea.Batch(m.spinner.Tick, m.loadThreads)
}

func (m *Model) focusLoginField(i int) tea.Cmd {
	l := m.login
	if len(l.inputs) == 0 {
		return nil
	}
	l.field = (i + len(l.inputs)) % len(l.inputs)
	var cmd tea.Cmd
	for j := range l.inputs {
		if j == l.field && len(l.step.Fields[j].Options) == 0 {
			cmd = l.inputs[j].Focus()
		} else {
			l.inputs[j].Blur()
		}
	}
	return cmd
}

func (m Model) loginValues() map[string]string {
	l := m.login
	values := make(map[string]string, len(l.step.Fields))
	for i, f := range l.step.Fields {
		if len(f.Options) > 0 {
			values[f.ID] = f.Options[l.options[i]]
		} else {
			values[f.ID] = l.inputs[i].Value()
		}
	}
	return values
}

func (m Model) updateLogin(msg tea.Msg) (Model, tea.Cmd) {
	l := m.login
	k, isKey := msg.(tea.KeyMsg)
	if !isKey {
		return m.forwardLoginInput(msg)
	}
	keys := m.keys
	if key.Matches(k, keys.ForceQuit) {
		return m, tea.Quit
	}
	switch l.screen {
	case loginChoose:
		choices := m.loginChoices()
		switch {
		case key.Matches(k, keys.Up):
			l.choice = max(l.choice-1, 0)
		case key.Matches(k, keys.Down):
			l.choice = min(l.choice+1, len(choices)-1)
		case key.Matches(k, keys.Select):
			return m, choices[l.choice].action(&m)
		case key.Matches(k, keys.Quit):
			return m, tea.Quit
		}
	case loginForm:
		switch {
		case key.Matches(k, keys.Back):
			m.abortLogin()
		case l.step.Kind == LoginUnsupported && key.Matches(k, keys.Select):
			m.loginService.Abort()
			m.openPaste()
			return m, textarea.Blink
		case key.Matches(k, keys.Select):
			values := m.loginValues()
			l.screen = loginBusy
			return m, m.loginCall(func(ctx context.Context) (LoginStep, error) {
				return m.loginService.Submit(ctx, values)
			})
		case key.Matches(k, keys.FieldNext):
			return m, m.focusLoginField(l.field + 1)
		case key.Matches(k, keys.FieldPrev):
			return m, m.focusLoginField(l.field - 1)
		case key.Matches(k, keys.OpenImage) && l.step.Attachment != "":
			if err := m.loginService.OpenAttachment(l.step.Attachment); err != nil {
				l.err = err.Error()
			}
		case len(l.inputs) > 0 && len(l.step.Fields[l.field].Options) > 0:
			opts := len(l.step.Fields[l.field].Options)
			switch {
			case key.Matches(k, keys.OptionNext):
				l.options[l.field] = (l.options[l.field] + 1) % opts
			case key.Matches(k, keys.OptionPrev):
				l.options[l.field] = (l.options[l.field] + opts - 1) % opts
			}
		default:
			return m.forwardLoginInput(msg)
		}
	case loginWaiting:
		if key.Matches(k, keys.Back) {
			if l.step.CanCancel {
				l.screen = loginBusy
				return m, m.loginCall(m.loginService.CancelStep)
			}
			m.abortLogin()
		}
	case loginPaste:
		switch {
		case key.Matches(k, keys.Back):
			l.screen = loginChoose
			l.err = ""
		case key.Matches(k, keys.Select):
			raw := strings.TrimSpace(l.paste.Value())
			if raw == "" {
				l.err = "paste the cookie export first"
				return m, nil
			}
			l.screen = loginBusy
			return m, m.track(func() tea.Msg {
				return cookiesUsedMsg{err: m.loginService.UseCookies(m.ctx, raw)}
			})
		default:
			return m.forwardLoginInput(msg)
		}
	}
	return m, nil
}

func (m Model) forwardLoginInput(msg tea.Msg) (Model, tea.Cmd) {
	l := m.login
	var cmd tea.Cmd
	switch {
	case l.screen == loginPaste:
		l.paste, cmd = l.paste.Update(msg)
	case l.screen == loginForm && len(l.inputs) > 0 && len(l.step.Fields[l.field].Options) == 0:
		l.inputs[l.field], cmd = l.inputs[l.field].Update(msg)
	}
	return m, cmd
}

func (m *Model) abortLogin() {
	if m.loginService != nil {
		m.loginService.Abort()
	}
	m.login.screen = loginChoose
	m.login.err = ""
	m.login.inputs = nil
	m.login.step = LoginStep{}
}

func (m Model) renderLogin() string {
	l := m.login
	width := max(min(68, m.width-4), 30)
	inner := width - 6
	wrap := lipgloss.NewStyle().Width(inner)
	parts := []string{titleStyle.Render("Log in to Messenger")}
	if l.notice != "" {
		parts = append(parts, "", wrap.Foreground(muted).Render(l.notice))
	}
	parts = append(parts, "")
	switch l.screen {
	case loginChoose:
		parts = append(parts, wrap.Foreground(muted).Render("Account login sends the password only to Meta and keeps a long-lived session. Cookie login uses an export from a logged-in browser."), "")
		for i, c := range m.loginChoices() {
			if i == l.choice {
				parts = append(parts, focusMark.Render("> "+c.label))
			} else {
				parts = append(parts, "  "+c.label)
			}
		}
	case loginForm:
		if l.step.Instructions != "" {
			parts = append(parts, wrap.Render(l.step.Instructions), "")
		}
		for i, f := range l.step.Fields {
			label := f.Name
			if i == l.field {
				label = focusMark.Render(label)
			}
			parts = append(parts, label)
			if len(f.Options) > 0 {
				parts = append(parts, "  ‹ "+f.Options[l.options[i]]+" ›")
			} else {
				parts = append(parts, "  "+l.inputs[i].View())
			}
			if f.Description != "" {
				parts = append(parts, wrap.Foreground(muted).Render("  "+f.Description))
			}
		}
		if l.step.Attachment != "" {
			parts = append(parts, "", wrap.Foreground(muted).Render("ctrl+o opens the image to read the code."))
		}
	case loginWaiting:
		instructions := l.step.Instructions
		if instructions == "" {
			instructions = "Approve the login on another device."
		}
		parts = append(parts, wrap.Render(instructions), "", m.spinner.View()+" waiting for approval…")
	case loginPaste:
		parts = append(parts, wrap.Foreground(muted).Render("Export messenger.com cookies from a private browser window, then close it without logging out."), "", l.paste.View())
	case loginBusy:
		parts = append(parts, m.spinner.View()+" contacting Meta…")
	}
	if l.err != "" {
		parts = append(parts, "", wrap.Foreground(danger).Render(l.err))
	}
	box := modalStyle.Width(width).Render(lipgloss.JoinVertical(lipgloss.Left, parts...))
	return lipgloss.Place(m.width, m.height-1, lipgloss.Center, lipgloss.Center, box) + "\n" + m.renderStatusBar()
}
