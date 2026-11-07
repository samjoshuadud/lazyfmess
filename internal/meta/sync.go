package meta

import (
	"context"
	"time"

	"go.mau.fi/mautrix-meta/pkg/messagix/table"

	"fmesstui/internal/store"
)

func ApplyTable(ctx context.Context, s *store.Store, tbl *table.LSTable) error {
	if s == nil || tbl == nil {
		return nil
	}
	for _, thread := range tbl.LSDeleteThenInsertThread {
		if thread == nil {
			continue
		}
		title := thread.ThreadName
		if title == "" {
			title = thread.ThreadDescription
		}
		if err := s.UpsertThread(ctx, store.Thread{
			ID:          thread.ThreadKey,
			Title:       title,
			LastPreview: thread.Snippet,
			ThreadType:  int64(thread.ThreadType),
			UpdatedAt:   time.UnixMilli(thread.LastActivityTimestampMs),
		}); err != nil {
			return err
		}
	}
	upsert, insert := tbl.WrapMessages()
	for _, um := range upsert {
		for _, msg := range um.Messages {
			if err := upsertWrapped(ctx, s, msg); err != nil {
				return err
			}
		}
	}
	for _, msg := range insert {
		if err := upsertWrapped(ctx, s, msg); err != nil {
			return err
		}
	}
	return nil
}

func upsertWrapped(ctx context.Context, s *store.Store, msg *table.WrappedMessage) error {
	if msg == nil || msg.LSInsertMessage == nil || msg.MessageId == "" {
		return nil
	}
	if err := s.EnsureThread(ctx, msg.ThreadKey); err != nil {
		return err
	}
	return s.UpsertMessage(ctx, store.Message{
		ID:        msg.MessageId,
		ThreadID:  msg.ThreadKey,
		SenderID:  msg.SenderId,
		Text:      msg.Text,
		Timestamp: time.UnixMilli(msg.TimestampMs),
	})
}
