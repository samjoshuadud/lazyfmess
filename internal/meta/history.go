package meta

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"time"

	"go.mau.fi/mautrix-meta/pkg/messagix/securehistory"
	"go.mau.fi/mautrix-meta/pkg/messagix/socket"
	"go.mau.fi/mautrix-meta/pkg/messagix/table"
	"go.mau.fi/mautrix-meta/pkg/metaid"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waArmadilloBackupMessage"
	"go.mau.fi/whatsmeow/proto/waConsumerApplication"
	"go.mau.fi/whatsmeow/proto/waMsgApplication"
	"google.golang.org/protobuf/proto"

	"lazyfmess/internal/session"
	"lazyfmess/internal/store"
)

const (
	historyPageSize   = 50
	historyMaxPages   = 20
	threadBatchDelay  = 2 * time.Second
	threadBatchLimit  = 40
	encryptedMaxPages = 10
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
	if table.ThreadType(thread.ThreadType).IsWhatsApp() {
		return c.syncEncryptedThread(ctx, s, thread)
	}
	return c.syncSocketThread(ctx, s, thread)
}

func (c *Client) SyncAllEncryptedHistory(ctx context.Context, s *store.Store) error {
	if err := c.waitReady(ctx); err != nil {
		return err
	}
	threads, err := s.ListThreads(ctx)
	if err != nil {
		return err
	}
	for _, thread := range threads {
		if !table.ThreadType(thread.ThreadType).IsWhatsApp() {
			continue
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := c.syncEncryptedThread(ctx, s, thread); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return nil
}

func (c *Client) syncEncryptedThread(ctx context.Context, s *store.Store, thread store.Thread) error {
	device := c.HistoryDevice()
	if device == nil || device.ID == "" {
		return nil
	}
	threadID := strconv.FormatInt(thread.ID, 10)
	timestamp := time.Now().Add(time.Minute).UnixMilli()
	isGroup := table.ThreadType(thread.ThreadType) == table.ENCRYPTED_OVER_WA_GROUP
	for page := 0; page < encryptedMaxPages; page++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		rangeResp, err := c.mx.GetSecureHistoryClient().FetchMessages(ctx, device, threadID, true, timestamp, historyPageSize)
		if err != nil {
			return fmt.Errorf("fetch encrypted history %d: %w", thread.ID, err)
		}
		if rangeResp.Epochs != nil && len(rangeResp.Epochs.Edges) > 0 {
			updated, err := device.WithEpochs(rangeResp.Epochs)
			if err != nil {
				return fmt.Errorf("apply history epochs: %w", err)
			}
			if err := c.saveHistoryDevice(updated); err != nil {
				return err
			}
			device = updated
		}
		messages, err := rangeResp.Decrypt(device, threadID, c.AccountID(), isGroup)
		if err != nil {
			return fmt.Errorf("decrypt encrypted history %d: %w", thread.ID, err)
		}
		if err := s.InTx(ctx, func(s *store.Store) error {
			return storeArchivePage(ctx, s, thread, messages)
		}); err != nil {
			return err
		}
		if rangeResp.Range == nil || rangeResp.Range.HasMoreBefore == nil || !*rangeResp.Range.HasMoreBefore {
			return nil
		}
		next := rangeResp.Range.Before
		if next >= timestamp {
			return nil
		}
		timestamp = next
	}
	return nil
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

func storeArchivePage(ctx context.Context, s *store.Store, thread store.Thread, messages []*securehistory.DecryptedMessage) error {
	var newestPreview string
	var newestTS time.Time
	for _, msg := range messages {
		plain, ok := archivePlainMessage(msg)
		if !ok {
			continue
		}
		if err := s.EnsureThread(ctx, thread.ID); err != nil {
			return err
		}
		if err := s.UpsertMessage(ctx, store.Message{
			ID:        plain.ID,
			ThreadID:  thread.ID,
			SenderID:  plain.SenderID,
			Text:      plain.Text,
			Timestamp: time.UnixMilli(plain.TimestampMS),
		}); err != nil {
			return err
		}
		if plain.TimestampMS >= newestTS.UnixMilli() {
			newestTS = time.UnixMilli(plain.TimestampMS)
			newestPreview = plain.Text
		}
	}
	if newestPreview == "" {
		return nil
	}
	return s.UpsertThread(ctx, store.Thread{
		ID:          thread.ID,
		Title:       thread.Title,
		LastPreview: newestPreview,
		ThreadType:  thread.ThreadType,
		UpdatedAt:   newestTS,
	})
}

type plainArchiveMessage struct {
	ID          string
	SenderID    int64
	Text        string
	TimestampMS int64
}

func archivePlainMessage(msg *securehistory.DecryptedMessage) (plainArchiveMessage, bool) {
	if msg == nil || msg.Message == nil || msg.Message.Message == nil {
		return plainArchiveMessage{}, false
	}
	meta := msg.Message.Message.GetMetadata()
	if meta == nil || meta.GetMessageID() == "" || meta.GetSenderID() == "" {
		return plainArchiveMessage{}, false
	}
	senderID, err := metaid.ParseIDFromString(meta.GetSenderID())
	if err != nil {
		return plainArchiveMessage{}, false
	}
	text, ok := archiveMessageText(msg.Message.Message)
	if !ok {
		return plainArchiveMessage{}, false
	}
	return plainArchiveMessage{
		ID:          meta.GetMessageID(),
		SenderID:    senderID,
		Text:        text,
		TimestampMS: msg.Message.TimestampMS,
	}, true
}

func archiveMessageText(message *waArmadilloBackupMessage.BackupMessage) (string, bool) {
	payload, ok := message.GetPayload().(*waArmadilloBackupMessage.BackupMessage_EncryptedTransportMessage)
	if !ok {
		return "", false
	}
	var application waMsgApplication.MessageApplication
	if err := proto.Unmarshal(payload.EncryptedTransportMessage, &application); err != nil {
		return "", false
	}
	decoded, err := whatsmeow.DecodeFBApplication(&application)
	if err != nil {
		return "", false
	}
	return consumerText(decoded.GetConsumerApplication())
}

func consumerText(msg *waConsumerApplication.ConsumerApplication) (string, bool) {
	if msg == nil || msg.GetPayload() == nil {
		return "", false
	}
	content := msg.GetPayload().GetContent()
	if content == nil {
		return "", false
	}
	switch typed := content.GetContent().(type) {
	case *waConsumerApplication.ConsumerApplication_Content_MessageText:
		if typed.MessageText == nil {
			return "", false
		}
		text := typed.MessageText.GetText()
		return text, text != ""
	case *waConsumerApplication.ConsumerApplication_Content_ExtendedTextMessage:
		if typed.ExtendedTextMessage == nil || typed.ExtendedTextMessage.GetText() == nil {
			return "", false
		}
		text := typed.ExtendedTextMessage.GetText().GetText()
		return text, text != ""
	case *waConsumerApplication.ConsumerApplication_Content_ImageMessage:
		if typed.ImageMessage != nil && typed.ImageMessage.GetCaption() != nil {
			if text := typed.ImageMessage.GetCaption().GetText(); text != "" {
				return text, true
			}
		}
		return "[image]", true
	case *waConsumerApplication.ConsumerApplication_Content_VideoMessage:
		if typed.VideoMessage != nil && typed.VideoMessage.GetCaption() != nil {
			if text := typed.VideoMessage.GetCaption().GetText(); text != "" {
				return text, true
			}
		}
		return "[video]", true
	case *waConsumerApplication.ConsumerApplication_Content_StickerMessage:
		return "[sticker]", true
	case *waConsumerApplication.ConsumerApplication_Content_AudioMessage:
		return "[audio]", true
	default:
		return "", false
	}
}
