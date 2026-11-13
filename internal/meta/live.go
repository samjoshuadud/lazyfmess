package meta

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"go.mau.fi/mautrix-meta/pkg/messagix/table"
	"go.mau.fi/whatsmeow/proto/waConsumerApplication"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"

	"lazyfmess/internal/store"
)

type Event struct {
	Type     string
	Table    *table.LSTable
	Err      error
	live     *liveMessage
	reaction *store.Reaction
	change   *messageChange
	receipt  *liveReceipt
	typing   []Typing
}

type liveReceipt struct {
	threadID int64
	readerID int64
	at       time.Time
	self     bool
}

type liveMessage struct {
	message    store.Message
	threadType table.ThreadType
	fromMe     bool
}

func (c *Client) onE2EEEvent(ctx context.Context, evt any) {
	if c.handler == nil {
		return
	}
	switch e := evt.(type) {
	case *events.FBMessage:
		if live, ok := decodeLiveMessage(e); ok {
			c.handler(ctx, Event{Type: "e2ee_message", live: &live})
		} else if reaction, ok := decodeLiveReaction(e); ok {
			c.handler(ctx, Event{Type: "e2ee_reaction", reaction: &reaction})
		} else if change, ok := decodeLiveChange(e); ok {
			c.handler(ctx, Event{Type: "e2ee_change", change: &change})
		}
	case *events.ChatPresence:
		if typing, ok := decodeChatPresence(e); ok {
			c.handler(ctx, Event{Type: "e2ee_typing", typing: []Typing{typing}})
		}
	case *events.Receipt:
		if receipt, ok := decodeReceipt(e); ok {
			c.handler(ctx, Event{Type: "e2ee_receipt", receipt: &receipt})
		}
	case *events.Disconnected:
		c.handler(ctx, Event{Type: "e2ee_disconnected", Err: fmt.Errorf("encrypted chats disconnected")})
	}
}

func decodeLiveMessage(evt *events.FBMessage) (liveMessage, bool) {
	consumer, ok := evt.Message.(*waConsumerApplication.ConsumerApplication)
	if !ok {
		return liveMessage{}, false
	}
	content, ok := decodeContent(consumer.GetPayload().GetContent())
	if !ok {
		return liveMessage{}, false
	}
	threadID, err := strconv.ParseInt(evt.Info.Chat.User, 10, 64)
	if err != nil {
		return liveMessage{}, false
	}
	senderID, err := strconv.ParseInt(evt.Info.Sender.User, 10, 64)
	if err != nil {
		return liveMessage{}, false
	}
	threadType := table.ENCRYPTED_OVER_WA_ONE_TO_ONE
	if evt.Info.Chat.Server == types.GroupServer {
		threadType = table.ENCRYPTED_OVER_WA_GROUP
	}
	return liveMessage{
		message: store.Message{
			ID:        evt.Info.ID,
			ThreadID:  threadID,
			SenderID:  senderID,
			Text:      content.text,
			MediaKind: content.mediaKind,
			MediaName: content.mediaName,
			Timestamp: evt.Info.Timestamp,
			ReplyToID: evt.FBApplication.GetMetadata().GetQuotedMessage().GetStanzaID(),
		},
		threadType: threadType,
		fromMe:     evt.Info.IsFromMe,
	}, true
}

func decodeReceipt(evt *events.Receipt) (liveReceipt, bool) {
	if evt.Type != types.ReceiptTypeRead && evt.Type != types.ReceiptTypeReadSelf {
		return liveReceipt{}, false
	}
	threadID, err := strconv.ParseInt(evt.Chat.User, 10, 64)
	if err != nil {
		return liveReceipt{}, false
	}
	readerID, err := strconv.ParseInt(evt.Sender.User, 10, 64)
	if err != nil {
		return liveReceipt{}, false
	}
	return liveReceipt{threadID: threadID, readerID: readerID, at: evt.Timestamp, self: evt.Type == types.ReceiptTypeReadSelf}, true
}

func ApplyEvent(ctx context.Context, s *store.Store, evt Event) (bool, error) {
	switch {
	case evt.Table != nil:
		return true, ApplyTable(ctx, s, evt.Table)
	case evt.live != nil:
		return true, applyLiveMessage(ctx, s, *evt.live)
	case evt.reaction != nil:
		return true, s.SetReaction(ctx, *evt.reaction)
	case evt.change != nil:
		return true, applyChange(ctx, s, *evt.change)
	case evt.receipt != nil && evt.receipt.self:
		return true, s.SetLastRead(ctx, evt.receipt.threadID, evt.receipt.at)
	case evt.receipt != nil:
		return true, s.SetParticipantRead(ctx, evt.receipt.threadID, evt.receipt.readerID, evt.receipt.at)
	}
	return false, nil
}

func applyLiveMessage(ctx context.Context, s *store.Store, live liveMessage) error {
	msg := live.message
	if err := s.UpsertThread(ctx, store.Thread{
		ID:          msg.ThreadID,
		LastPreview: previewOf(msg),
		ThreadType:  int64(live.threadType),
		UpdatedAt:   msg.Timestamp,
	}); err != nil {
		return err
	}
	if err := s.UpsertMessage(ctx, msg); err != nil {
		return err
	}
	if live.fromMe {
		return s.SetLastRead(ctx, msg.ThreadID, msg.Timestamp)
	}
	return nil
}
