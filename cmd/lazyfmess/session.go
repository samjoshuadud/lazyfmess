package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"

	"github.com/rs/zerolog/log"

	"lazyfmess/internal/meta"
	"lazyfmess/internal/session"
	"lazyfmess/internal/store"
	"lazyfmess/internal/ui"
)

const sessionExpired = "The Messenger session expired. Log in again."

type sessions struct {
	dir       string
	logWriter io.Writer
	db        *store.Store
	clients   *clientHolder
	app       *atomic.Pointer[ui.App]
	creds     chan *session.Cookies
}

func (s *sessions) run(ctx context.Context, initial *session.Cookies, loginReason string) {
	pending := initial
	if pending == nil {
		s.app.Load().NeedLogin(loginReason)
	}
	for {
		if pending == nil {
			select {
			case pending = <-s.creds:
			case <-ctx.Done():
				return
			}
		}
		creds := pending
		pending = nil
		lost := make(chan struct{}, 1)
		client, err := meta.New(creds, s.dir, s.logWriter, s.handleEvent(lost))
		if err != nil {
			s.app.Load().NeedLogin(fmt.Sprintf("The saved login could not be used: %v", err))
			continue
		}
		s.clients.set(client)
		err = connect(ctx, client, s.db)
		if meta.IsLoggedOut(err) {
			s.drop(client)
			s.app.Load().NeedLogin(sessionExpired)
			continue
		}
		s.app.Load().Connected(err)
		select {
		case <-ctx.Done():
			s.drop(client)
			return
		case <-lost:
			s.drop(client)
			if saved, err := session.LoadCookies(s.dir); err == nil {
				pending = saved
			} else {
				s.app.Load().NeedLogin(sessionExpired)
			}
		}
	}
}

func (s *sessions) drop(client *meta.Client) {
	s.clients.clear()
	client.Disconnect()
}

func (s *sessions) handleEvent(lost chan struct{}) meta.EventHandler {
	return func(ctx context.Context, evt meta.Event) {
		app := s.app.Load()
		switch {
		case evt.Type == "permanent_error" && meta.IsLoggedOut(evt.Err):
			select {
			case lost <- struct{}{}:
			default:
			}
			return
		case evt.Type == "permanent_error" && evt.Err != nil:
			app.Error(evt.Err.Error())
		case evt.Err != nil:
			app.Toast(evt.Err.Error())
		case evt.Type == "reconnected":
			app.Toast("reconnected")
		}
		changed, err := meta.ApplyEvent(ctx, s.db, evt)
		if err != nil {
			log.Error().Err(err).Str("event", evt.Type).Msg("apply event")
			return
		}
		if changed {
			app.Notify()
		}
		typing, err := meta.TypingFromEvent(ctx, s.db, evt)
		if err != nil {
			log.Error().Err(err).Str("event", evt.Type).Msg("read typing event")
		}
		for _, t := range typing {
			app.Typing(t.ThreadID, t.SenderID, t.Active)
		}
	}
}

func (s *sessions) deliver(ctx context.Context, creds *session.Cookies) error {
	if err := session.SaveCookies(s.dir, creds); err != nil {
		return err
	}
	select {
	case s.creds <- creds:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

type loginService struct {
	sessions *sessions
	home     string
	mu       sync.Mutex
	login    *meta.Login
	stores   []session.BrowserStore
}

func (l *loginService) Browsers() []string {
	stores := session.FindBrowserStores(l.home)
	l.mu.Lock()
	l.stores = stores
	l.mu.Unlock()
	names := make([]string, len(stores))
	for i, s := range stores {
		names[i] = s.Name
	}
	return names
}

func (l *loginService) ImportBrowser(ctx context.Context, name string) error {
	l.mu.Lock()
	stores := l.stores
	l.mu.Unlock()
	for _, store := range stores {
		if store.Name == name {
			creds, err := session.ImportBrowserCookies(ctx, store)
			if err != nil {
				return err
			}
			return l.sessions.deliver(ctx, creds)
		}
	}
	return fmt.Errorf("browser %q not found", name)
}

func (l *loginService) StartAccount(ctx context.Context) (ui.LoginStep, error) {
	l.Abort()
	login, step, err := meta.StartLogin(ctx, l.sessions.logWriter)
	if err != nil {
		return ui.LoginStep{}, err
	}
	l.mu.Lock()
	l.login = login
	l.mu.Unlock()
	return l.result(ctx, step)
}

func (l *loginService) Submit(ctx context.Context, values map[string]string) (ui.LoginStep, error) {
	return l.call(ctx, func(login *meta.Login) (meta.LoginStep, error) { return login.Submit(ctx, values) })
}

func (l *loginService) Wait(ctx context.Context) (ui.LoginStep, error) {
	return l.call(ctx, func(login *meta.Login) (meta.LoginStep, error) { return login.Wait(ctx) })
}

func (l *loginService) CancelStep(ctx context.Context) (ui.LoginStep, error) {
	return l.call(ctx, func(login *meta.Login) (meta.LoginStep, error) { return login.Cancel(ctx) })
}

func (l *loginService) OpenAttachment(path string) error {
	l.mu.Lock()
	login := l.login
	l.mu.Unlock()
	if login == nil {
		return errors.New("no login in progress")
	}
	return login.OpenAttachment(path)
}

func (l *loginService) Abort() {
	l.mu.Lock()
	login := l.login
	l.login = nil
	l.mu.Unlock()
	if login != nil {
		login.Close()
	}
}

func (l *loginService) UseCookies(ctx context.Context, raw string) error {
	creds, err := session.ParseCookies([]byte(raw))
	if err != nil {
		return err
	}
	return l.sessions.deliver(ctx, creds)
}

func (l *loginService) call(ctx context.Context, step func(*meta.Login) (meta.LoginStep, error)) (ui.LoginStep, error) {
	l.mu.Lock()
	login := l.login
	l.mu.Unlock()
	if login == nil {
		return ui.LoginStep{}, errors.New("no login in progress")
	}
	next, err := step(login)
	if err != nil {
		return ui.LoginStep{}, err
	}
	return l.result(ctx, next)
}

func (l *loginService) result(ctx context.Context, step meta.LoginStep) (ui.LoginStep, error) {
	if step.Kind == meta.LoginDone {
		l.Abort()
		if err := l.sessions.deliver(ctx, step.Credentials); err != nil {
			return ui.LoginStep{}, err
		}
		return ui.LoginStep{Kind: ui.LoginDone}, nil
	}
	fields := make([]ui.LoginField, len(step.Fields))
	for i, f := range step.Fields {
		fields[i] = ui.LoginField{ID: f.ID, Name: f.Name, Description: f.Description, Secret: f.Secret, Options: f.Options}
	}
	kind := map[meta.LoginStepKind]ui.LoginStepKind{
		meta.LoginInput:       ui.LoginInput,
		meta.LoginWait:        ui.LoginWait,
		meta.LoginUnsupported: ui.LoginUnsupported,
	}[step.Kind]
	return ui.LoginStep{
		Kind:         kind,
		Instructions: step.Instructions,
		Fields:       fields,
		Attachment:   step.Attachment,
		CanCancel:    step.CanCancel,
	}, nil
}
