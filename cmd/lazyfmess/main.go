package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
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

func (s senderAdapter) SendText(ctx context.Context, thread store.Thread, text string) (ui.SendResult, error) {
	result, err := s.client.SendText(ctx, thread, text)
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

	var app *ui.App
	client, err := meta.New(cookies, dir, logWriter, func(evtCtx context.Context, evt meta.Event) {
		if evt.Err != nil && app != nil {
			app.Toast(evt.Err.Error())
		}
		if evt.Table == nil {
			return
		}
		if err := meta.ApplyTable(evtCtx, db, evt.Table); err != nil {
			return
		}
		if app != nil {
			app.Notify()
		}
	})
	if err != nil {
		return err
	}
	defer client.Disconnect()

	if err := client.Connect(ctx); err != nil {
		return err
	}
	if err := meta.ApplyTable(ctx, db, client.InitialTable()); err != nil {
		return fmt.Errorf("sync initial table: %w", err)
	}

	app = ui.NewApp(
		ctx,
		db,
		senderAdapter{client: client},
		historyAdapter{client: client, store: db},
		unlockAdapter{client: client},
		ui.Config{
			PrefillPIN: *pinFlag,
			OnReady: func() {
				go syncHistoryBackground(ctx, client, db, app)
			},
		},
	)
	return app.Run()
}

func syncHistoryBackground(ctx context.Context, client *meta.Client, db *store.Store, app *ui.App) {
	if err := client.SyncInboxThreads(ctx, db); err == nil && app != nil {
		app.Notify()
	}
	if client.HasHistoryDevice() {
		if err := client.SyncAllEncryptedHistory(ctx, db); err == nil && app != nil {
			app.Notify()
		}
	}
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
