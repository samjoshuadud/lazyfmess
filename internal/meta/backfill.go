package meta

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"go.mau.fi/mautrix-meta/pkg/messagix/securehistory"
	"go.mau.fi/mautrix-meta/pkg/messagix/table"

	"lazyfmess/internal/store"
)

const (
	historyPageDelay    = 300 * time.Millisecond
	historyPagesPerChat = 20
	historyPagesPerRun  = 400
)

var errNoHistoryDevice = errors.New("encrypted history is locked")

type historyPage struct {
	messages  []store.Message
	reactions []store.Reaction
	before    int64
	more      bool
	failed    int
}

type pageFetcher func(ctx context.Context, before int64) (historyPage, error)

type historyResult struct {
	pages  int
	failed int
}

func syncHistory(ctx context.Context, s *store.Store, thread store.Thread, fetch pageFetcher, budget int, delay time.Duration) (historyResult, error) {
	var res historyResult
	now := time.Now().Add(time.Minute).UnixMilli()
	state, found, err := s.HistoryState(ctx, thread.ID)
	if err != nil {
		return res, err
	}
	_, newest, have, err := s.MessageTimeRange(ctx, thread.ID)
	if err != nil {
		return res, err
	}
	page := func(before int64) (historyPage, error) {
		if res.pages > 0 {
			select {
			case <-ctx.Done():
				return historyPage{}, ctx.Err()
			case <-time.After(delay):
			}
		}
		p, err := fetch(ctx, before)
		if err != nil {
			return p, err
		}
		res.pages++
		res.failed += p.failed
		return p, storeHistoryPage(ctx, s, thread, p.messages, p.reactions)
	}
	if found && have {
		before := now
		for res.pages < budget {
			p, err := page(before)
			if err != nil {
				return res, err
			}
			if !p.more || p.before >= before || len(p.messages) == 0 || oldestMS(p.messages) <= newest.UnixMilli() {
				break
			}
			before = p.before
		}
	}
	if found && state.ReachedStart {
		return res, nil
	}
	cursor := now
	if found {
		cursor = state.Before.UnixMilli()
	}
	for res.pages < budget {
		p, err := page(cursor)
		if err != nil {
			return res, err
		}
		reached := !p.more || p.before >= cursor
		if !reached {
			cursor = p.before
		}
		if err := s.SetHistoryState(ctx, thread.ID, store.HistoryState{Before: time.UnixMilli(cursor), ReachedStart: reached}); err != nil {
			return res, err
		}
		if reached {
			break
		}
	}
	return res, nil
}

func oldestMS(messages []store.Message) int64 {
	oldest := messages[0].Timestamp.UnixMilli()
	for _, m := range messages[1:] {
		oldest = min(oldest, m.Timestamp.UnixMilli())
	}
	return oldest
}

func storeHistoryPage(ctx context.Context, s *store.Store, thread store.Thread, messages []store.Message, reactions []store.Reaction) error {
	if len(messages) == 0 {
		return nil
	}
	return s.InTx(ctx, func(s *store.Store) error {
		if err := s.EnsureThread(ctx, thread.ID); err != nil {
			return err
		}
		newest := messages[0]
		for _, m := range messages {
			if err := s.UpsertMessage(ctx, m); err != nil {
				return err
			}
			if m.Timestamp.After(newest.Timestamp) {
				newest = m
			}
		}
		for _, r := range reactions {
			if err := s.SetReaction(ctx, r); err != nil {
				return err
			}
		}
		return s.UpsertThread(ctx, store.Thread{
			ID:          thread.ID,
			LastPreview: previewOf(newest),
			ThreadType:  thread.ThreadType,
			UpdatedAt:   newest.Timestamp,
		})
	})
}

func (c *Client) encryptedPages(thread store.Thread) pageFetcher {
	threadID := strconv.FormatInt(thread.ID, 10)
	isGroup := table.ThreadType(thread.ThreadType) == table.ENCRYPTED_OVER_WA_GROUP
	return func(ctx context.Context, before int64) (historyPage, error) {
		device := c.HistoryDevice()
		if device == nil || device.ID == "" {
			return historyPage{}, errNoHistoryDevice
		}
		r, err := c.mx.GetSecureHistoryClient().FetchMessages(ctx, device, threadID, true, before, historyPageSize)
		if err != nil {
			return historyPage{}, fmt.Errorf("fetch encrypted history: %w", err)
		}
		if r.Epochs != nil && len(r.Epochs.Edges) > 0 {
			updated, err := device.WithEpochs(r.Epochs)
			if err != nil {
				return historyPage{}, fmt.Errorf("apply history epochs: %w", err)
			}
			if err := c.saveHistoryDevice(updated); err != nil {
				return historyPage{}, err
			}
			device = updated
		}
		if r.ThreadMissing {
			return historyPage{}, nil
		}
		decrypted, failed := decryptRange(r, device, threadID, c.AccountID(), isGroup)
		page := historyPage{failed: failed}
		if r.Range != nil {
			page.before = r.Range.Before
			page.more = r.Range.HasMoreBefore != nil && *r.Range.HasMoreBefore
		}
		for _, d := range decrypted {
			if archived, ok := decodeArchived(d, thread.ID); ok {
				page.messages = append(page.messages, archived.message)
				page.reactions = append(page.reactions, archived.reactions...)
			}
		}
		return page, nil
	}
}

func decryptRange(r *securehistory.MessageRange, device *securehistory.Device, threadID string, self int64, isGroup bool) ([]*securehistory.DecryptedMessage, int) {
	all, err := r.Decrypt(device, threadID, self, isGroup)
	if err == nil {
		return all, 0
	}
	var out []*securehistory.DecryptedMessage
	failed := 0
	for _, m := range r.Messages {
		single := *r
		single.Messages = []*securehistory.ArchiveMessage{m}
		d, err := single.Decrypt(device, threadID, self, isGroup)
		if err != nil {
			failed++
			continue
		}
		out = append(out, d...)
	}
	return out, failed
}
