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
	if err := applyContacts(ctx, s, tbl); err != nil {
		return err
	}
	for _, thread := range tbl.LSDeleteThenInsertThread {
		if err := upsertThread(ctx, s, thread); err != nil {
			return err
		}
	}
	for _, thread := range tbl.LSUpdateOrInsertThread {
		if err := upsertThread(ctx, s, thread); err != nil {
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
	for _, msg := range tbl.LSDeleteThenInsertMessage {
		if err := upsertDeletedMessage(ctx, s, msg); err != nil {
			return err
		}
	}
	return nil
}

func applyContacts(ctx context.Context, s *store.Store, tbl *table.LSTable) error {
	for _, part := range tbl.LSAddParticipantIdToGroupThread {
		if part == nil || part.Nickname == "" {
			continue
		}
		if err := s.UpsertContact(ctx, part.ContactId, part.Nickname); err != nil {
			return err
		}
	}
	for _, contact := range tbl.LSVerifyContactParticipantExist {
		if contact == nil {
			continue
		}
		if err := s.UpsertContact(ctx, contact.ContactID, contact.Name); err != nil {
			return err
		}
	}
	for _, contact := range tbl.LSDeleteThenInsertContact {
		if contact == nil {
			continue
		}
		if err := s.UpsertContact(ctx, contact.GetFBID(), contact.GetName()); err != nil {
			return err
		}
	}
	for _, contact := range tbl.LSVerifyContactRowExists {
		if contact == nil {
			continue
		}
		if err := s.UpsertContact(ctx, contact.GetFBID(), contact.GetName()); err != nil {
			return err
		}
	}
	return nil
}

func upsertThread(ctx context.Context, s *store.Store, thread table.ThreadInfo) error {
	if thread == nil {
		return nil
	}
	title := thread.GetThreadName()
	if title == "" {
		title = thread.GetThreadDescription()
	}
	if title == "" {
		name, err := s.ContactName(ctx, thread.GetThreadKey())
		if err != nil {
			return err
		}
		title = name
	}
	var updatedAt time.Time
	var preview string
	var threadType int64
	switch t := thread.(type) {
	case *table.LSDeleteThenInsertThread:
		updatedAt = time.UnixMilli(t.LastActivityTimestampMs)
		preview = t.Snippet
		threadType = int64(t.ThreadType)
	case *table.LSUpdateOrInsertThread:
		updatedAt = time.UnixMilli(t.LastActivityTimestampMs)
		preview = t.Snippet
		threadType = int64(t.ThreadType)
	default:
		threadType = int64(thread.GetThreadType())
	}
	return s.UpsertThread(ctx, store.Thread{
		ID:          thread.GetThreadKey(),
		Title:       title,
		LastPreview: preview,
		ThreadType:  threadType,
		UpdatedAt:   updatedAt,
	})
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

func upsertDeletedMessage(ctx context.Context, s *store.Store, msg *table.LSDeleteThenInsertMessage) error {
	if msg == nil || msg.MessageId == "" {
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
