package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
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

func run() error {
	dataDirFlag := flag.String("data-dir", "", "directory for session and database files")
	verbose := flag.Bool("verbose", false, "enable trace-level protocol logs in the log file")
	pinFlag := flag.String("pin", "", "six-digit Messenger secure storage PIN for encrypted history")
	demo := flag.Bool("demo", false, "")
	flag.Usage = usage
	flag.Parse()

	if *demo {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		return runDemo(ctx)
	}

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

	db, err := store.Open(ctx, dir)
	if err != nil {
		return err
	}
	defer db.Close()

	creds, credsErr := session.LoadCookies(dir)
	loginReason := ""
	switch {
	case errors.Is(credsErr, fs.ErrNotExist):
		loginReason = "No saved login yet."
	case credsErr != nil:
		loginReason = fmt.Sprintf("The saved login could not be read: %v", credsErr)
	}

	home, _ := os.UserHomeDir()
	var appRef atomic.Pointer[ui.App]
	clients := newClientHolder()
	sess := &sessions{
		dir:       dir,
		logWriter: logWriter,
		db:        db,
		clients:   clients,
		app:       &appRef,
		creds:     make(chan *session.Cookies, 1),
	}
	b := backend{clients: clients, db: db}
	app := ui.NewApp(
		ctx,
		db,
		ui.Services{
			Sender:   b,
			History:  b,
			Unlocker: b,
			Searcher: b,
			Media:    b,
			Reader:   b,
			Typing:   b,
			Reactor:  b,
			Editor:   b,
			Muter:    b,
			Login:    &loginService{sessions: sess, home: home},
		},
		ui.Config{
			PrefillPIN: *pinFlag,
			Connecting: creds != nil,
			BackgroundSync: func(ctx context.Context) error {
				client, err := clients.wait(ctx)
				if err != nil {
					return err
				}
				return syncHistoryBackground(ctx, client, db, appRef.Load())
			},
		},
	)
	appRef.Store(app)

	sessionCtx, cancelSessions := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		sess.run(sessionCtx, creds, loginReason)
	}()
	err = app.Run()
	cancelSessions()
	<-done
	return err
}

func usage() {
	out := flag.CommandLine.Output()
	fmt.Fprintf(out, "Usage of %s:\n", os.Args[0])
	flag.VisitAll(func(f *flag.Flag) {
		if f.Name == "demo" {
			return
		}
		fmt.Fprintf(out, "  -%s\n    \t%s\n", f.Name, f.Usage)
	})
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
	historyErr := client.SyncAllHistory(ctx, db, func(done, total int) {
		app.SyncProgress(done, total)
		app.Notify()
	})
	return summarizeSyncErrors(errors.Join(inboxErr, historyErr))
}

func summarizeSyncErrors(err error) error {
	if err == nil {
		return nil
	}
	errs := []error{err}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		errs = joined.Unwrap()
	}
	for _, e := range errs {
		log.Error().Err(e).Msg("history sync")
	}
	if len(errs) == 1 {
		return errs[0]
	}
	return fmt.Errorf("%d problems during history sync, details in lazyfmess.log", len(errs))
}

const maxLogBytes = 10 << 20

func rotateLog(path string, limit int64) error {
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("check log file: %w", err)
	}
	if info.Size() <= limit {
		return nil
	}
	if err := os.Rename(path, path+".1"); err != nil {
		return fmt.Errorf("rotate log file: %w", err)
	}
	return nil
}

func setupLogging(dir string, verbose bool) (io.Closer, io.Writer, error) {
	path := filepath.Join(dir, "lazyfmess.log")
	if err := rotateLog(path, maxLogBytes); err != nil {
		return nil, nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
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
