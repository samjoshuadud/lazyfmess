package meta

import (
	"context"
	"time"

	"go.mau.fi/mautrix-meta/pkg/messagix/table"

	"lazyfmess/internal/store"
)

func ApplyTable(ctx context.Context, s *store.Store, tbl *table.LSTable) error {
	if s == nil || tbl == nil {
		return nil
	}
	return s.InTx(ctx, func(s *store.Store) error {
		return applyTable(ctx, s, tbl)
	})
}

func applyTable(ctx context.Context, s *store.Store, tbl *table.LSTable) error {
	if err := applyContacts(ctx, s, tbl); err != nil {
		return err
	}
	if err := applyHybridMappings(ctx, s, tbl); err != nil {
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
	if err := applyReactions(ctx, s, tbl); err != nil {
		return err
	}
	if err := applyEdits(ctx, s, tbl); err != nil {
		return err
	}
	if err := applyReceipts(ctx, s, tbl); err != nil {
		return err
	}
	if err := applyMuteSettings(ctx, s, tbl); err != nil {
		return err
	}
	return applyReadMarks(ctx, s, tbl)
}

func applyReceipts(ctx context.Context, s *store.Store, tbl *table.LSTable) error {
	type receipt struct{ threadKey, contactID, readMS int64 }
	var receipts []receipt
	for _, p := range tbl.LSAddParticipantIdToGroupThread {
		receipts = append(receipts, receipt{p.ThreadKey, p.ContactId, p.ReadWatermarkTimestampMs})
	}
	for _, r := range tbl.LSUpdateReadReceipt {
		receipts = append(receipts, receipt{r.ThreadKey, r.ContactId, r.ReadWatermarkTimestampMs})
	}
	for _, r := range receipts {
		key, err := resolveThreadKey(ctx, s, r.threadKey)
		if err != nil {
			return err
		}
		if err := s.SetParticipantRead(ctx, key, r.contactID, time.UnixMilli(r.readMS)); err != nil {
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

func applyHybridMappings(ctx context.Context, s *store.Store, tbl *table.LSTable) error {
	for _, mapping := range tbl.LSUpdateThreadAuthorityAndMappingWithOTIDFromJID {
		if mapping == nil {
			continue
		}
		if err := s.PutHybridThread(ctx, mapping.ThreadKey, mapping.ThreadJID, 0); err != nil {
			return err
		}
	}
	for _, hybrid := range tbl.LSVerifyHybridThreadExists {
		if hybrid == nil {
			continue
		}
		if err := s.PutHybridThread(ctx, hybrid.ThreadKey, hybrid.ThreadJID, int64(hybrid.ThreadType)); err != nil {
			return err
		}
		title, err := s.ContactName(ctx, hybrid.ThreadJID)
		if err != nil {
			return err
		}
		if err := s.UpsertThread(ctx, store.Thread{
			ID:         hybrid.ThreadJID,
			Title:      title,
			ThreadType: int64(hybrid.ThreadType),
			UpdatedAt:  time.UnixMilli(hybrid.LastActivityTimestampMS),
		}); err != nil {
			return err
		}
	}
	return nil
}

func resolveThreadKey(ctx context.Context, s *store.Store, key int64) (int64, error) {
	jid, err := s.HybridThreadJID(ctx, key)
	if err != nil {
		return 0, err
	}
	if jid != 0 {
		return jid, nil
	}
	return key, nil
}

func upsertThread(ctx context.Context, s *store.Store, thread table.ThreadInfo) error {
	if thread == nil {
		return nil
	}
	key, err := resolveThreadKey(ctx, s, thread.GetThreadKey())
	if err != nil {
		return err
	}
	title := thread.GetThreadName()
	if title == "" {
		title = thread.GetThreadDescription()
	}
	if title == "" {
		name, err := s.ContactName(ctx, key)
		if err != nil {
			return err
		}
		title = name
	}
	var updatedAt time.Time
	var preview string
	var threadType int64
	var muteUntil *int64
	switch t := thread.(type) {
	case *table.LSDeleteThenInsertThread:
		updatedAt = time.UnixMilli(t.LastActivityTimestampMs)
		preview = t.Snippet
		threadType = int64(t.ThreadType)
		muteUntil = &t.MuteExpireTimeMs
	case *table.LSUpdateOrInsertThread:
		updatedAt = time.UnixMilli(t.LastActivityTimestampMs)
		preview = t.Snippet
		threadType = int64(t.ThreadType)
		muteUntil = &t.MuteExpireTimeMs
	default:
		threadType = int64(thread.GetThreadType())
	}
	if err := s.UpsertThread(ctx, store.Thread{
		ID:          key,
		Title:       title,
		LastPreview: preview,
		ThreadType:  threadType,
		UpdatedAt:   updatedAt,
	}); err != nil {
		return err
	}
	if muteUntil != nil {
		if err := s.SetMute(ctx, key, *muteUntil); err != nil {
			return err
		}
	}
	return s.SetLastRead(ctx, key, time.UnixMilli(thread.GetLastReadWatermarkTimestampMs()))
}

func applyMuteSettings(ctx context.Context, s *store.Store, tbl *table.LSTable) error {
	for _, setting := range tbl.LSUpdateThreadMuteSetting {
		key, err := resolveThreadKey(ctx, s, setting.ThreadKey)
		if err != nil {
			return err
		}
		if err := s.SetMute(ctx, key, setting.MuteExpireTimeMS); err != nil {
			return err
		}
	}
	return nil
}

func applyReadMarks(ctx context.Context, s *store.Store, tbl *table.LSTable) error {
	marks := make(map[int64]int64)
	for _, mark := range tbl.LSMarkThreadRead {
		marks[mark.ThreadKey] = max(marks[mark.ThreadKey], mark.LastReadWatermarkTimestampMs)
	}
	for _, mark := range tbl.LSMarkThreadReadV2 {
		marks[mark.ThreadKey] = max(marks[mark.ThreadKey], mark.LastReadWatermarkTimestampMs)
	}
	for threadKey, watermark := range marks {
		key, err := resolveThreadKey(ctx, s, threadKey)
		if err != nil {
			return err
		}
		if err := s.SetLastRead(ctx, key, time.UnixMilli(watermark)); err != nil {
			return err
		}
	}
	return nil
}

func upsertWrapped(ctx context.Context, s *store.Store, msg *table.WrappedMessage) error {
	if msg == nil || msg.LSInsertMessage == nil || msg.MessageId == "" {
		return nil
	}
	key, err := resolveThreadKey(ctx, s, msg.ThreadKey)
	if err != nil {
		return err
	}
	if err := s.EnsureThread(ctx, key); err != nil {
		return err
	}
	kind, url, mime, name, durationMs := mediaFromWrapped(msg)
	return s.UpsertMessage(ctx, store.Message{
		ID:              msg.MessageId,
		ThreadID:        key,
		SenderID:        msg.SenderId,
		Text:            msg.Text,
		Timestamp:       time.UnixMilli(msg.TimestampMs),
		MediaKind:       kind,
		MediaURL:        url,
		MediaMime:       mime,
		MediaName:       name,
		MediaDurationMs: durationMs,
		ReplyToID:       msg.ReplySourceId,
		ReplyText:       msg.ReplyMessageText,
		ReplySenderID:   msg.ReplyToUserId,
		Edited:          msg.EditCount > 0,
		Unsent:          msg.IsUnsent,
	})
}

func upsertDeletedMessage(ctx context.Context, s *store.Store, msg *table.LSDeleteThenInsertMessage) error {
	if msg == nil || msg.MessageId == "" {
		return nil
	}
	key, err := resolveThreadKey(ctx, s, msg.ThreadKey)
	if err != nil {
		return err
	}
	if err := s.EnsureThread(ctx, key); err != nil {
		return err
	}
	return s.UpsertMessage(ctx, store.Message{
		ID:            msg.MessageId,
		ThreadID:      key,
		SenderID:      msg.SenderId,
		Text:          msg.Text,
		Timestamp:     time.UnixMilli(msg.TimestampMs),
		ReplyToID:     msg.ReplySourceId,
		ReplyText:     msg.ReplyMessageText,
		ReplySenderID: msg.ReplyToUserId,
		Edited:        msg.EditCount > 0,
		Unsent:        msg.IsUnsent,
	})
}
