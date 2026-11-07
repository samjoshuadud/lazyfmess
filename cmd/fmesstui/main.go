package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"fmesstui/internal/meta"
	"fmesstui/internal/session"
	"fmesstui/internal/store"
	"fmesstui/internal/ui"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "fmesstui: %v\n", err)
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

func run() error {
	dataDirFlag := flag.String("data-dir", "", "directory for session and database files")
	flag.Parse()

	dir, err := resolveDataDir(*dataDirFlag)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create data directory: %w", err)
	}

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
	client, err := meta.New(cookies, dir, os.Stderr, func(evtCtx context.Context, evt meta.Event) {
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
	app = ui.NewApp(ctx, db, senderAdapter{client: client})
	return app.Run()
}

func resolveDataDir(override string) (string, error) {
	if override != "" {
		return filepath.Abs(override)
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config directory: %w", err)
	}
	return filepath.Join(base, "fmesstui"), nil
}
