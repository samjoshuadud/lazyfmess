package meta

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"go.mau.fi/mautrix-meta/pkg/messagix/socket"
	"go.mau.fi/mautrix-meta/pkg/messagix/table"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waConsumerApplication"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"

	"lazyfmess/internal/store"
)

func (c *Client) React(ctx context.Context, s *store.Store, thread store.Thread, msg store.Message, emoji string) error {
	now := time.Now()
	if table.ThreadType(thread.ThreadType).IsWhatsApp() {
		if c.e2ee == nil {
			return fmt.Errorf("e2ee client is not connected")
		}
		payload := consumerContent(&waConsumerApplication.ConsumerApplication_Content{
			Content: &waConsumerApplication.ConsumerApplication_Content_ReactionMessage{
				ReactionMessage: &waConsumerApplication.ConsumerApplication_ReactionMessage{
					Key:               c.waMessageKey(thread, msg),
					Text:              proto.String(emoji),
					SenderTimestampMS: proto.Int64(now.UnixMilli()),
				},
			},
		})
		if _, err := c.e2ee.SendFBMessage(ctx, chatJID(thread), payload, nil, whatsmeow.SendRequestExtra{ID: c.e2ee.GenerateMessageID()}); err != nil {
			return fmt.Errorf("send e2ee reaction: %w", err)
		}
	} else if _, err := c.mx.ExecuteTasks(ctx, &socket.SendReactionTask{
		ThreadKey:       thread.ID,
		TimestampMs:     now.UnixMilli(),
		MessageID:       msg.ID,
		ActorID:         c.AccountID(),
		Reaction:        emoji,
		SyncGroup:       1,
		SendAttribution: table.MESSENGER_INBOX_IN_THREAD,
	}); err != nil {
		return fmt.Errorf("send reaction: %w", err)
	}
	return s.SetReaction(ctx, store.Reaction{MessageID: msg.ID, ActorID: c.AccountID(), Emoji: emoji})
}

func (c *Client) waMessageKey(thread store.Thread, msg store.Message) *waCommon.MessageKey {
	chat := chatJID(thread)
	key := &waCommon.MessageKey{
		RemoteJID: proto.String(chat.String()),
		ID:        proto.String(msg.ID),
	}
	if msg.SenderID == c.AccountID() {
		key.FromMe = proto.Bool(true)
	}
	if chat.Server == types.GroupServer {
		key.Participant = proto.String(types.JID{User: strconv.FormatInt(msg.SenderID, 10), Server: types.MessengerServer}.String())
	}
	return key
}

func consumerContent(content *waConsumerApplication.ConsumerApplication_Content) *waConsumerApplication.ConsumerApplication {
	return &waConsumerApplication.ConsumerApplication{
		Payload: &waConsumerApplication.ConsumerApplication_Payload{
			Payload: &waConsumerApplication.ConsumerApplication_Payload_Content{Content: content},
		},
	}
}

func decodeLiveReaction(evt *events.FBMessage) (store.Reaction, bool) {
	consumer, ok := evt.Message.(*waConsumerApplication.ConsumerApplication)
	if !ok {
		return store.Reaction{}, false
	}
	reaction := consumer.GetPayload().GetContent().GetReactionMessage()
	if reaction == nil || reaction.GetKey().GetID() == "" {
		return store.Reaction{}, false
	}
	actorID, err := strconv.ParseInt(evt.Info.Sender.User, 10, 64)
	if err != nil {
		return store.Reaction{}, false
	}
	return store.Reaction{MessageID: reaction.GetKey().GetID(), ActorID: actorID, Emoji: reaction.GetText()}, true
}

func applyReactions(ctx context.Context, s *store.Store, tbl *table.LSTable) error {
	for _, r := range tbl.LSUpsertReaction {
		if err := s.SetReaction(ctx, store.Reaction{MessageID: r.MessageId, ActorID: r.ActorId, Emoji: r.Reaction}); err != nil {
			return err
		}
	}
	for _, r := range tbl.LSDeleteReaction {
		if err := s.SetReaction(ctx, store.Reaction{MessageID: r.MessageId, ActorID: r.ActorId}); err != nil {
			return err
		}
	}
	return nil
}
