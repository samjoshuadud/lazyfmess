package meta

import (
	"context"
	"fmt"
	"strconv"

	"go.mau.fi/mautrix-meta/pkg/messagix/table"
	"go.mau.fi/whatsmeow/proto/waConsumerApplication"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"

	"lazyfmess/internal/store"
)

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
	text, ok := consumerText(consumer)
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
			Text:      text,
			Timestamp: evt.Info.Timestamp,
		},
		threadType: threadType,
		fromMe:     evt.Info.IsFromMe,
	}, true
}

func ApplyEvent(ctx context.Context, s *store.Store, evt Event) (bool, error) {
	switch {
	case evt.Table != nil:
		return true, ApplyTable(ctx, s, evt.Table)
	case evt.live != nil:
		return true, applyLiveMessage(ctx, s, *evt.live)
	}
	return false, nil
}

func applyLiveMessage(ctx context.Context, s *store.Store, live liveMessage) error {
	msg := live.message
	if err := s.UpsertThread(ctx, store.Thread{
		ID:          msg.ThreadID,
		LastPreview: msg.Text,
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
