package ui

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

type fakeLogin struct {
	mu        sync.Mutex
	submitted map[string]string
	cookies   string
	cookieErr error
	steps     []LoginStep
	aborted   int
	browsers  []string
	imported  string
}

func (f *fakeLogin) next() (LoginStep, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	step := f.steps[0]
	f.steps = f.steps[1:]
	return step, nil
}

func (f *fakeLogin) StartAccount(context.Context) (LoginStep, error) { return f.next() }
func (f *fakeLogin) Submit(_ context.Context, values map[string]string) (LoginStep, error) {
	f.mu.Lock()
	f.submitted = values
	f.mu.Unlock()
	return f.next()
}
func (f *fakeLogin) Wait(context.Context) (LoginStep, error)       { return f.next() }
func (f *fakeLogin) CancelStep(context.Context) (LoginStep, error) { return f.next() }
func (f *fakeLogin) OpenAttachment(string) error                   { return nil }
func (f *fakeLogin) Abort()                                        { f.aborted++ }
func (f *fakeLogin) Browsers() []string                            { return f.browsers }
func (f *fakeLogin) ImportBrowser(_ context.Context, name string) error {
	f.imported = name
	return f.cookieErr
}
func (f *fakeLogin) UseCookies(_ context.Context, raw string) error {
	f.cookies = raw
	return f.cookieErr
}

func loginModel(t *testing.T, svc *fakeLogin) Model {
	t.Helper()
	m := newTestModel(t, 1, 0)
	m.loginService = svc
	return drain(t, m, needLoginMsg{reason: "No saved login yet."})
}

func typeText(m Model, s string) Model {
	for _, r := range s {
		model, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = model.(Model)
	}
	return m
}

func TestAccountLoginFlow(t *testing.T) {
	t.Parallel()
	svc := &fakeLogin{steps: []LoginStep{
		{Kind: LoginInput, Fields: []LoginField{{ID: "username", Name: "Email"}, {ID: "password", Name: "Password", Secret: true}}},
		{Kind: LoginWait, Instructions: "Approve on your phone", CanCancel: true},
		{Kind: LoginDone},
	}}
	m := loginModel(t, svc)
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "Log in to Messenger") || !strings.Contains(view, "No saved login yet.") {
		t.Fatalf("login screen missing:\n%s", view)
	}
	m = drain(t, m, keyPress("enter"))
	if m.login == nil || m.login.screen != loginForm || len(m.login.inputs) != 2 {
		t.Fatalf("expected credential form, got %+v", m.login)
	}
	m = typeText(m, "me@example.test")
	m = drain(t, m, tea.KeyMsg{Type: tea.KeyTab})
	m = typeText(m, "hunter2")
	if view := ansi.Strip(m.View()); strings.Contains(view, "hunter2") {
		t.Fatal("password must be masked")
	}
	m = drain(t, m, keyPress("enter"))
	if svc.submitted["username"] != "me@example.test" || svc.submitted["password"] != "hunter2" {
		t.Fatalf("submitted: %v", svc.submitted)
	}
	if m.login != nil || !m.connecting {
		t.Fatalf("after done: login %+v connecting %v", m.login, m.connecting)
	}
}

func TestPasteCookiesFlow(t *testing.T) {
	t.Parallel()
	svc := &fakeLogin{cookieErr: errors.New("parse cookies: missing required cookie \"xs\"")}
	m := loginModel(t, svc)
	m = drain(t, m, keyPress("j"))
	m = drain(t, m, keyPress("enter"))
	if m.login.screen != loginPaste {
		t.Fatalf("screen: %v", m.login.screen)
	}
	m.login.paste.SetValue(`{"platform":"messenger","cookies":{"c_user":"1"}}`)
	m = drain(t, m, keyPress("enter"))
	if m.login == nil || m.login.screen != loginPaste || !strings.Contains(m.login.err, "missing required cookie") {
		t.Fatalf("expected paste error, got %+v", m.login)
	}
	svc.cookieErr = nil
	m = drain(t, m, keyPress("enter"))
	if m.login != nil || !strings.Contains(svc.cookies, "c_user") {
		t.Fatalf("cookies not used: login %+v cookies %q", m.login, svc.cookies)
	}
}

func TestLoginEscStartsOver(t *testing.T) {
	t.Parallel()
	svc := &fakeLogin{steps: []LoginStep{{Kind: LoginInput, Fields: []LoginField{{ID: "code", Name: "Code"}}}}}
	m := loginModel(t, svc)
	m = drain(t, m, keyPress("enter"))
	m = drain(t, m, keyPress("esc"))
	if m.login.screen != loginChoose || svc.aborted == 0 {
		t.Fatalf("esc: screen %v aborted %d", m.login.screen, svc.aborted)
	}
}

func TestImportFromBrowserChoice(t *testing.T) {
	t.Parallel()
	svc := &fakeLogin{browsers: []string{"Firefox (default)"}, cookieErr: errors.New("no logged-in session found")}
	m := loginModel(t, svc)
	if !strings.Contains(ansi.Strip(m.View()), "Import cookies from Firefox (default)") {
		t.Fatal("browser choice missing")
	}
	m = drain(t, m, keyPress("j"))
	m = drain(t, m, keyPress("enter"))
	if svc.imported != "Firefox (default)" || m.login.screen != loginChoose || !strings.Contains(m.login.err, "no logged-in session") {
		t.Fatalf("failed import: imported %q login %+v", svc.imported, m.login)
	}
	svc.cookieErr = nil
	m = drain(t, m, keyPress("enter"))
	if m.login != nil {
		t.Fatal("successful import should leave the login screen")
	}
}
