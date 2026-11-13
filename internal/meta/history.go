package meta

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"go.mau.fi/mautrix-meta/pkg/messagix/securehistory"
	"go.mau.fi/mautrix-meta/pkg/messagix/socket"
	"go.mau.fi/mautrix-meta/pkg/messagix/table"

	"lazyfmess/internal/session"
	"lazyfmess/internal/store"
)

const (
	historyPageSize  = 50
	historyMaxPages  = 20
	threadBatchDelay = 2 * time.Second
	threadBatchLimit = 40
)

func (c *Client) loadHistoryDevice() error {
	device, err := session.LoadHistoryDevice(c.dataDir)
	if err != nil {
		return err
	}
	c.historyMu.Lock()
	c.historyDevice = device
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
	if err := session.SaveHistoryDevice(c.dataDir, device); err != nil {
		return err
	}
	c.historyMu.Lock()
	c.historyDevice = device
	c.historyMu.Unlock()
	return nil
}

func (c *Client) saveHistoryDevice(device *securehistory.Device) error {
	if err := session.SaveHistoryDevice(c.dataDir, device); err != nil {
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
	if !table.ThreadType(thread.ThreadType).IsWhatsApp() {
		return c.syncSocketThread(ctx, s, thread)
	}
	if !c.HasHistoryDevice() {
		return nil
	}
	res, err := syncHistory(ctx, s, thread, c.encryptedPages(thread), historyPagesPerChat, historyPageDelay)
	return historyError(thread, res, err)
}

func (c *Client) SyncAllEncryptedHistory(ctx context.Context, s *store.Store, progress func(done, total int)) error {
	if err := c.waitReady(ctx); err != nil {
		return err
	}
	threads, err := s.ListThreads(ctx)
	if err != nil {
		return err
	}
	var encrypted []store.Thread
	for _, t := range threads {
		if table.ThreadType(t.ThreadType).IsWhatsApp() {
			encrypted = append(encrypted, t)
		}
	}
	var errs []error
	remaining := historyPagesPerRun
	for i, thread := range encrypted {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if progress != nil {
			progress(i, len(encrypted))
		}
		if remaining <= 0 || !c.HasHistoryDevice() {
			break
		}
		res, err := syncHistory(ctx, s, thread, c.encryptedPages(thread), min(historyPagesPerChat, remaining), historyPageDelay)
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
		progress(len(encrypted), len(encrypted))
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

func (c *Client) syncSocketThread(ctx context.Context, s *store.Store, thread store.Thread) error {
	var refTS int64
	var refID string
	oldest, err := s.OldestMessage(ctx, thread.ID)
	switch {
	case err == nil:
		refTS = oldest.Timestamp.UnixMilli()
		refID = oldest.ID
	case errors.Is(err, sql.ErrNoRows):
	default:
		return err
	}
	for page := 0; page < historyMaxPages; page++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var task socket.Task
		if refID == "" {
			task = &socket.CreateThreadTask{ThreadFBID: thread.ID, SyncGroup: 1}
		} else {
			task = &socket.FetchMessagesTask{
				ThreadKey:            thread.ID,
				Direction:            0,
				ReferenceTimestampMs: refTS,
				ReferenceMessageId:   refID,
				SyncGroup:            1,
				Cursor:               c.mx.GetCursor(1),
			}
		}
		tbl, err := c.mx.ExecuteTasks(ctx, task)
		if err != nil {
			return fmt.Errorf("fetch socket history %d: %w", thread.ID, err)
		}
		if err := ApplyTable(ctx, s, tbl); err != nil {
			return err
		}
		upsert, _ := tbl.WrapMessages()
		um := upsert[thread.ID]
		if um == nil || um.Range == nil || !um.Range.HasMoreBefore || um.Range.MinMessageId == "" || um.Range.MinMessageId == refID {
			return nil
		}
		refTS = um.Range.MinTimestampMs
		refID = um.Range.MinMessageId
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return nil
}
