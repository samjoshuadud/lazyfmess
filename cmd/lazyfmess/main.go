package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"sync/atomic"
	"syscall"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	signallog "go.mau.fi/libsignal/logger"

	"lazyfmess/internal/meta"
	"lazyfmess/internal/session"
	"lazyfmess/internal/store"
	"lazyfmess/internal/ui"
)

type signalLogBridge struct{}

func (signalLogBridge) Debug(caller, message string) {
	log.Debug().Str("caller", caller).Msg(message)
}

func (signalLogBridge) Info(caller, message string) {
	log.Info().Str("caller", caller).Msg(message)
}

func (signalLogBridge) Warning(caller, message string) {
	log.Warn().Str("caller", caller).Msg(message)
}

func (signalLogBridge) Error(caller, message string) {
	log.Error().Str("caller", caller).Msg(message)
}

func (signalLogBridge) Configure(string) {}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "lazyfmess: %v\n", err)
		os.Exit(1)
	}
}

type senderAdapter struct {
	client *meta.Client
}

func (s senderAdapter) AccountID() int64 {
	return s.client.AccountID()
}

func (s senderAdapter) SendText(ctx context.Context, thread store.Thread, text string, replyTo store.Message) (ui.SendResult, error) {
	result, err := s.client.SendText(ctx, thread, text, replyTo)
	if err != nil {
		return ui.SendResult{}, err
	}
	return ui.SendResult{MessageID: result.MessageID, Timestamp: result.Timestamp}, nil
}

func (s senderAdapter) SendFile(ctx context.Context, thread store.Thread, path string) (ui.SendResult, error) {
	result, err := s.client.SendFile(ctx, thread, path)
	if err != nil {
		return ui.SendResult{}, err
	}
	return ui.SendResult{MessageID: result.MessageID, Timestamp: result.Timestamp}, nil
}

type historyAdapter struct {
	client *meta.Client
	store  *store.Store
}

func (h historyAdapter) LoadHistory(ctx context.Context, thread store.Thread) error {
	return h.client.SyncThreadHistory(ctx, h.store, thread)
}

type unlockAdapter struct {
	client *meta.Client
}

func (u unlockAdapter) NeedsPIN(ctx context.Context) (bool, error) {
	return u.client.NeedsHistoryPIN(ctx)
}

func (u unlockAdapter) Unlock(ctx context.Context, pin string) error {
	return u.client.UnlockHistory(ctx, pin)
}

type mediaAdapter struct {
	client *meta.Client
}

func (m mediaAdapter) OpenMedia(ctx context.Context, msg store.Message) error {
	return m.client.OpenMedia(ctx, msg)
}

type readAdapter struct {
	client *meta.Client
	store  *store.Store
}

func (r readAdapter) MarkRead(ctx context.Context, thread store.Thread) error {
	return r.client.MarkRead(ctx, r.store, thread)
}

type typingAdapter struct {
	client *meta.Client
}

func (t typingAdapter) SetTyping(ctx context.Context, thread store.Thread, active bool) error {
	return t.client.SetTyping(ctx, thread, active)
}

type reactAdapter struct {
	client *meta.Client
	store  *store.Store
}

func (r reactAdapter) React(ctx context.Context, thread store.Thread, msg store.Message, emoji string) error {
	return r.client.React(ctx, r.store, thread, msg, emoji)
}

type editAdapter struct {
	client *meta.Client
	store  *store.Store
}

func (e editAdapter) Edit(ctx context.Context, thread store.Thread, msg store.Message, text string) error {
	return e.client.Edit(ctx, e.store, thread, msg, text)
}

func (e editAdapter) Unsend(ctx context.Context, thread store.Thread, msg store.Message) error {
	return e.client.Unsend(ctx, e.store, thread, msg)
}

type muteAdapter struct {
	client *meta.Client
	store  *store.Store
}

func (m muteAdapter) SetMuted(ctx context.Context, thread store.Thread, muted bool) error {
	return m.client.SetMuted(ctx, m.store, thread, muted)
}

type searchAdapter struct {
	client *meta.Client
	store  *store.Store
}

func (s searchAdapter) SearchPeople(ctx context.Context, query string) ([]ui.PersonHit, error) {
	people, err := s.client.SearchPeople(ctx, query)
	if err != nil {
		return nil, err
	}
	hits := make([]ui.PersonHit, 0, len(people))
	for _, person := range people {
		hits = append(hits, ui.PersonHit{
			ID:         person.ID,
			Name:       person.Name,
			Context:    person.Context,
			ThreadType: person.ThreadType,
		})
	}
	return hits, nil
}

func (s searchAdapter) OpenPerson(ctx context.Context, person ui.PersonHit) (int64, error) {
	thread, err := s.client.OpenPerson(ctx, s.store, meta.Person{
		ID:         person.ID,
		Name:       person.Name,
		Context:    person.Context,
		ThreadType: person.ThreadType,
	})
	if err != nil {
		return 0, err
	}
	return thread.ID, nil
}

func run() error {
	dataDirFlag := flag.String("data-dir", "", "directory for session and database files")
	verbose := flag.Bool("verbose", false, "enable trace-level protocol logs in the log file")
	pinFlag := flag.String("pin", "", "six-digit Messenger secure storage PIN for encrypted history")
	flag.Parse()

	dir, err := resolveDataDir(*dataDirFlag)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create data directory: %w", err)
	}

	logCloser, logWriter, err := setupLogging(dir, *verbose)
	if err != nil {
		return err
	}
	defer logCloser.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cookies, err := session.LoadCookies(dir)
	if err != nil {
		return err
	}

	db, err := store.Open(ctx, dir)
	if err != nil {
		return err
	}
	defer db.Close()

	var appRef atomic.Pointer[ui.App]
	client, err := meta.New(cookies, dir, logWriter, func(evtCtx context.Context, evt meta.Event) {
		app := appRef.Load()
		switch {
		case evt.Type == "permanent_error" && evt.Err != nil:
			app.Error(evt.Err.Error())
		case evt.Err != nil:
			app.Toast(evt.Err.Error())
		case evt.Type == "reconnected":
			app.Toast("reconnected")
		}
		changed, err := meta.ApplyEvent(evtCtx, db, evt)
		if err != nil {
			log.Error().Err(err).Str("event", evt.Type).Msg("apply event")
			return
		}
		if changed {
			app.Notify()
		}
		typing, err := meta.TypingFromEvent(evtCtx, db, evt)
		if err != nil {
			log.Error().Err(err).Str("event", evt.Type).Msg("read typing event")
		}
		for _, t := range typing {
			app.Typing(t.ThreadID, t.SenderID, t.Active)
		}
	})
	if err != nil {
		return err
	}
	defer client.Disconnect()

	app := ui.NewApp(
		ctx,
		db,
		ui.Services{
			Sender:   senderAdapter{client: client},
			History:  historyAdapter{client: client, store: db},
			Unlocker: unlockAdapter{client: client},
			Searcher: searchAdapter{client: client, store: db},
			Media:    mediaAdapter{client: client},
			Reader:   readAdapter{client: client, store: db},
			Typing:   typingAdapter{client: client},
			Reactor:  reactAdapter{client: client, store: db},
			Editor:   editAdapter{client: client, store: db},
			Muter:    muteAdapter{client: client, store: db},
		},
		ui.Config{
			PrefillPIN: *pinFlag,
			Connecting: true,
			BackgroundSync: func(ctx context.Context) error {
				return syncHistoryBackground(ctx, client, db, appRef.Load())
			},
		},
	)
	appRef.Store(app)

	connectCtx, cancelConnect := context.WithCancel(ctx)
	connected := make(chan struct{})
	go func() {
		defer close(connected)
		app.Connected(connect(connectCtx, client, db))
	}()
	err = app.Run()
	cancelConnect()
	<-connected
	return err
}

func connect(ctx context.Context, client *meta.Client, db *store.Store) error {
	if err := client.Connect(ctx); err != nil {
		return err
	}
	if err := meta.ApplyTable(ctx, db, client.InitialTable()); err != nil {
		return fmt.Errorf("sync initial table: %w", err)
	}
	return nil
}

func syncHistoryBackground(ctx context.Context, client *meta.Client, db *store.Store, app *ui.App) error {
	inboxErr := client.SyncInboxThreads(ctx, db)
	app.Notify()
	if !client.HasHistoryDevice() {
		return inboxErr
	}
	return errors.Join(inboxErr, client.SyncAllEncryptedHistory(ctx, db))
}

func setupLogging(dir string, verbose bool) (io.Closer, io.Writer, error) {
	f, err := os.OpenFile(filepath.Join(dir, "lazyfmess.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, nil, fmt.Errorf("open log file: %w", err)
	}
	var w io.Writer = f
	level := zerolog.DebugLevel
	if verbose {
		level = zerolog.TraceLevel
	}
	zerolog.SetGlobalLevel(level)
	log.Logger = zerolog.New(w).With().Timestamp().Logger()
	var signalLogger signallog.Loggable = signalLogBridge{}
	signallog.Setup(&signalLogger)
	return f, w, nil
}

func resolveDataDir(override string) (string, error) {
	if override != "" {
		return filepath.Abs(override)
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config directory: %w", err)
	}
	return filepath.Join(base, "lazyfmess"), nil
}
