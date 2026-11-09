package meta

import (
	"context"
	"fmt"
	"time"

	"go.mau.fi/mautrix-meta/pkg/messagix/socket"
	"go.mau.fi/mautrix-meta/pkg/messagix/table"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waConsumerApplication"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"

	"lazyfmess/internal/store"
)

type messageChange struct {
	id     string
	text   string
	unsent bool
}

func (c *Client) Edit(ctx context.Context, s *store.Store, thread store.Thread, msg store.Message, text string) error {
	text = trimMessage(text)
	if text == "" {
		return fmt.Errorf("edited message is empty")
	}
	if table.ThreadType(thread.ThreadType).IsWhatsApp() {
		payload := consumerContent(&waConsumerApplication.ConsumerApplication_Content{
			Content: &waConsumerApplication.ConsumerApplication_Content_EditMessage{
				EditMessage: &waConsumerApplication.ConsumerApplication_EditMessage{
					Key:         c.waMessageKey(thread, msg),
					Message:     &waCommon.MessageText{Text: proto.String(text)},
					TimestampMS: proto.Int64(time.Now().UnixMilli()),
				},
			},
		})
		if err := c.sendE2EEControl(ctx, thread, payload); err != nil {
			return fmt.Errorf("send e2ee edit: %w", err)
		}
	} else if _, err := c.mx.ExecuteTasks(ctx, &socket.EditMessageTask{MessageID: msg.ID, Text: text}); err != nil {
		return fmt.Errorf("edit message: %w", err)
	}
	return s.EditMessage(ctx, msg.ID, text)
}

func (c *Client) Unsend(ctx context.Context, s *store.Store, thread store.Thread, msg store.Message) error {
	if table.ThreadType(thread.ThreadType).IsWhatsApp() {
		payload := &waConsumerApplication.ConsumerApplication{
			Payload: &waConsumerApplication.ConsumerApplication_Payload{
				Payload: &waConsumerApplication.ConsumerApplication_Payload_ApplicationData{
					ApplicationData: &waConsumerApplication.ConsumerApplication_ApplicationData{
						ApplicationContent: &waConsumerApplication.ConsumerApplication_ApplicationData_Revoke{
							Revoke: &waConsumerApplication.ConsumerApplication_RevokeMessage{Key: c.waMessageKey(thread, msg)},
						},
					},
				},
			},
		}
		if err := c.sendE2EEControl(ctx, thread, payload); err != nil {
			return fmt.Errorf("send e2ee unsend: %w", err)
		}
	} else if _, err := c.mx.ExecuteTasks(ctx, &socket.DeleteMessageTask{MessageId: msg.ID}); err != nil {
		return fmt.Errorf("unsend message: %w", err)
	}
	return s.UnsendMessage(ctx, msg.ID)
}

func (c *Client) sendE2EEControl(ctx context.Context, thread store.Thread, payload *waConsumerApplication.ConsumerApplication) error {
	if c.e2ee == nil {
		return fmt.Errorf("e2ee client is not connected")
	}
	_, err := c.e2ee.SendFBMessage(ctx, chatJID(thread), payload, nil, whatsmeow.SendRequestExtra{ID: c.e2ee.GenerateMessageID()})
	return err
}

func decodeLiveChange(evt *events.FBMessage) (messageChange, bool) {
	consumer, ok := evt.Message.(*waConsumerApplication.ConsumerApplication)
	if !ok {
		return messageChange{}, false
	}
	if revoke := consumer.GetPayload().GetApplicationData().GetRevoke(); revoke.GetKey().GetID() != "" {
		return messageChange{id: revoke.GetKey().GetID(), unsent: true}, true
	}
	edit := consumer.GetPayload().GetContent().GetEditMessage()
	if edit.GetKey().GetID() == "" || edit.GetMessage().GetText() == "" {
		return messageChange{}, false
	}
	return messageChange{id: edit.GetKey().GetID(), text: edit.GetMessage().GetText()}, true
}

func applyChange(ctx context.Context, s *store.Store, change messageChange) error {
	if change.unsent {
		return s.UnsendMessage(ctx, change.id)
	}
	return s.EditMessage(ctx, change.id, change.text)
}

func applyEdits(ctx context.Context, s *store.Store, tbl *table.LSTable) error {
	for _, edit := range tbl.LSEditMessage {
		if err := s.EditMessage(ctx, edit.MessageID, edit.Text); err != nil {
			return err
		}
	}
	for _, deleted := range tbl.LSDeleteMessage {
		if err := s.DeleteMessage(ctx, deleted.MessageId); err != nil {
			return err
		}
	}
	return nil
}
