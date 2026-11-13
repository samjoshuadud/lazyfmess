package meta

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"go.mau.fi/mautrix-meta/pkg/messagix/securehistory"
	"go.mau.fi/mautrix-meta/pkg/messagix/table"

	"lazyfmess/internal/session"
	"lazyfmess/internal/store"
)

const (
	historyPageSize  = 50
	threadBatchDelay = 2 * time.Second
	threadBatchLimit = 40
)

func (c *Client) loadHistoryDevice() error {
	data, err := session.LoadHistoryDevice(c.dataDir)
	if err != nil || data == nil {
		return err
	}
	var device securehistory.Device
	if err := json.Unmarshal(data, &device); err != nil {
		return fmt.Errorf("parse history device: %w", err)
	}
	if device.ID == "" || device.BackupID == "" {
		return nil
	}
	c.historyMu.Lock()
	c.historyDevice = &device
	c.historyMu.Unlock()
	return nil
}

func (c *Client) HasHistoryDevice() bool {
	c.historyMu.Lock()
	defer c.historyMu.Unlock()
	return c.historyDevice != nil && c.historyDevice.ID != ""
}

func (c *Client) HistoryDevice() *securehistory.Device {
	c.historyMu.Lock()
	defer c.historyMu.Unlock()
	return c.historyDevice
}

func (c *Client) NeedsHistoryPIN(ctx context.Context) (bool, error) {
	if err := c.waitReady(ctx); err != nil {
		return false, err
	}
	if c.HasHistoryDevice() {
		return false, nil
	}
	_, err := c.mx.GetSecureHistoryClient().GetBackup(ctx)
	if errors.Is(err, securehistory.ErrNoPINBackup) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (c *Client) UnlockHistory(ctx context.Context, pin string) error {
	if err := c.waitReady(ctx); err != nil {
		return err
	}
	if c == nil || c.mx == nil {
		return fmt.Errorf("unlock history: client is nil")
	}
	if c.device == nil || c.device.IdentityKey == nil {
		return fmt.Errorf("unlock history: e2ee device is not ready")
	}
	if c.HasHistoryDevice() {
		return nil
	}
	history := c.mx.GetSecureHistoryClient()
	backup, err := history.GetBackup(ctx)
	if err != nil {
		return fmt.Errorf("get history backup: %w", err)
	}
	recovery, err := history.RecoverPIN(ctx, backup, []byte(pin))
	if err != nil {
		return fmt.Errorf("recover history pin: %w", err)
	}
	virtual, err := history.GetVirtualDevice(ctx, backup, c.AccountID(), recovery.Code)
	if err != nil {
		return fmt.Errorf("get virtual history device: %w", err)
	}
	epochs, err := virtual.DeriveEpochs()
	if err != nil {
		return fmt.Errorf("derive history epochs: %w", err)
	}
	enrollment, err := virtual.PrepareDevice(backup.ID, history.RegistrationID, c.device.IdentityKey, epochs)
	if err != nil {
		return fmt.Errorf("prepare history device: %w", err)
	}
	device, err := history.AddDevice(ctx, enrollment)
	if err != nil {
		return fmt.Errorf("add history device: %w", err)
	}
	return c.saveHistoryDevice(device)
}

func (c *Client) saveHistoryDevice(device *securehistory.Device) error {
	if device == nil || device.ID == "" {
		return nil
	}
	data, err := json.Marshal(device)
	if err != nil {
		return fmt.Errorf("marshal history device: %w", err)
	}
	if err := session.SaveHistoryDevice(c.dataDir, data); err != nil {
		return err
	}
	c.historyMu.Lock()
	c.historyDevice = device
	c.historyMu.Unlock()
	return nil
}

func (c *Client) SyncInboxThreads(ctx context.Context, s *store.Store) error {
	if err := c.waitReady(ctx); err != nil {
		return err
	}
	if c == nil || c.mx == nil || s == nil {
		return nil
	}
	var prevMin int64
	for batch := 0; batch < threadBatchLimit; batch++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		keyStore, tbl, err := c.mx.FetchMoreThreads(ctx, 1)
		if err != nil {
			return fmt.Errorf("fetch more threads: %w", err)
		}
		if tbl == nil {
			return nil
		}
		if err := ApplyTable(ctx, s, tbl); err != nil {
			return err
		}
		if keyStore == nil || !keyStore.HasMoreBefore || keyStore.MinThreadKey == prevMin {
			return nil
		}
		prevMin = keyStore.MinThreadKey
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(threadBatchDelay):
		}
	}
	return nil
}

func (c *Client) SyncThreadHistory(ctx context.Context, s *store.Store, thread store.Thread) error {
	if err := c.waitReady(ctx); err != nil {
		return err
	}
	if c == nil || s == nil || thread.ID == 0 {
		return nil
	}
	fetch, ok := c.historyFetcher(s, thread)
	if !ok {
		return nil
	}
	res, err := syncHistory(ctx, s, thread, fetch, historyPagesPerChat, historyPageDelay)
	return historyError(thread, res, err)
}

func (c *Client) historyFetcher(s *store.Store, thread store.Thread) (pageFetcher, bool) {
	if !table.ThreadType(thread.ThreadType).IsWhatsApp() {
		return c.socketPages(s, thread), true
	}
	if !c.HasHistoryDevice() {
		return nil, false
	}
	return c.encryptedPages(thread), true
}

func (c *Client) SyncAllHistory(ctx context.Context, s *store.Store, progress func(done, total int)) error {
	if err := c.waitReady(ctx); err != nil {
		return err
	}
	threads, err := s.ListThreads(ctx)
	if err != nil {
		return err
	}
	var errs []error
	remaining := historyPagesPerRun
	for i, thread := range threads {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if progress != nil {
			progress(i, len(threads))
		}
		if remaining <= 0 {
			break
		}
		fetch, ok := c.historyFetcher(s, thread)
		if !ok {
			continue
		}
		res, err := syncHistory(ctx, s, thread, fetch, min(historyPagesPerChat, remaining), historyPageDelay)
		remaining -= res.pages
		if err := historyError(thread, res, err); err != nil {
			errs = append(errs, err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(historyPageDelay):
		}
	}
	if progress != nil {
		progress(len(threads), len(threads))
	}
	return errors.Join(errs...)
}

func historyError(thread store.Thread, res historyResult, err error) error {
	var errs []error
	if err != nil {
		errs = append(errs, err)
	}
	if res.failed > 0 {
		errs = append(errs, fmt.Errorf("%d messages could not be decrypted", res.failed))
	}
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("chat %d (%s): %w", thread.ID, thread.Title, errors.Join(errs...))
}
