package meta

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"go.mau.fi/mautrix-meta/pkg/messagix/socket"
	"go.mau.fi/mautrix-meta/pkg/messagix/table"

	"lazyfmess/internal/store"
)

const socketPageTimeout = 15 * time.Second

type rangeSignal struct {
	upsert *table.UpsertMessages
	ended  bool
}

func (c *Client) waitRange(threadID int64) (chan rangeSignal, func()) {
	ch := make(chan rangeSignal, 4)
	c.rangeMu.Lock()
	if c.rangeWaiters == nil {
		c.rangeWaiters = make(map[int64]chan rangeSignal)
	}
	c.rangeWaiters[threadID] = ch
	c.rangeMu.Unlock()
	return ch, func() {
		c.rangeMu.Lock()
		if c.rangeWaiters[threadID] == ch {
			delete(c.rangeWaiters, threadID)
		}
		c.rangeMu.Unlock()
	}
}

func (c *Client) notifyRanges(tbl *table.LSTable) {
	c.rangeMu.Lock()
	defer c.rangeMu.Unlock()
	if len(c.rangeWaiters) == 0 {
		return
	}
	send := func(threadID int64, sig rangeSignal) {
		if ch, ok := c.rangeWaiters[threadID]; ok {
			select {
			case ch <- sig:
			default:
			}
		}
	}
	upsert, _ := tbl.WrapMessages()
	for key, um := range upsert {
		send(key, rangeSignal{upsert: um})
	}
	for _, r := range tbl.LSUpdateExistingMessageRange {
		send(r.ThreadKey, rangeSignal{ended: true})
	}
}

func (c *Client) socketPages(s *store.Store, thread store.Thread) pageFetcher {
	return func(ctx context.Context, _ int64) (historyPage, error) {
		ch, done := c.waitRange(thread.ID)
		defer done()
		var task socket.Task
		oldest, err := s.OldestMessage(ctx, thread.ID)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			task = &socket.CreateThreadTask{ThreadFBID: thread.ID, SyncGroup: 1}
		case err != nil:
			return historyPage{}, err
		default:
			task = &socket.FetchMessagesTask{
				ThreadKey:            thread.ID,
				ReferenceTimestampMs: oldest.Timestamp.UnixMilli(),
				ReferenceMessageId:   oldest.ID,
				SyncGroup:            1,
				Cursor:               c.mx.GetCursor(1),
			}
		}
		for attempt := 0; attempt < 2; attempt++ {
			tbl, err := c.mx.ExecuteTasks(ctx, task)
			if err != nil {
				return historyPage{}, fmt.Errorf("fetch history: %w", err)
			}
			if err := ApplyTable(ctx, s, tbl); err != nil {
				return historyPage{}, err
			}
			if upsert, _ := tbl.WrapMessages(); upsert[thread.ID] != nil {
				return pageFromUpsert(upsert[thread.ID], thread.ID), nil
			}
			select {
			case sig := <-ch:
				if sig.ended {
					return historyPage{}, nil
				}
				return pageFromUpsert(sig.upsert, thread.ID), nil
			case <-time.After(socketPageTimeout):
			case <-ctx.Done():
				return historyPage{}, ctx.Err()
			}
		}
		return historyPage{}, fmt.Errorf("fetch history: no response from Messenger")
	}
}

func pageFromUpsert(um *table.UpsertMessages, threadID int64) historyPage {
	var page historyPage
	for _, msg := range um.Messages {
		if msg != nil && msg.LSInsertMessage != nil && msg.MessageId != "" {
			page.messages = append(page.messages, wrappedMessage(msg, threadID))
		}
	}
	if um.Range != nil {
		page.before = um.Range.MinTimestampMs
		page.more = um.Range.HasMoreBefore
	}
	return page
}
