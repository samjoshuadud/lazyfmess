package meta

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"go.mau.fi/mautrix-meta/pkg/messagix/methods"
	"go.mau.fi/mautrix-meta/pkg/messagix/socket"
	"go.mau.fi/mautrix-meta/pkg/messagix/table"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waConsumerApplication"
	"go.mau.fi/whatsmeow/proto/waMsgApplication"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"

	"lazyfmess/internal/store"
)

type SendResult struct {
	MessageID string
	Timestamp time.Time
}

func (c *Client) SendText(ctx context.Context, thread store.Thread, text string, replyTo store.Message) (SendResult, error) {
	text = trimMessage(text)
	if text == "" {
		return SendResult{}, fmt.Errorf("message is empty")
	}
	if table.ThreadType(thread.ThreadType).IsWhatsApp() {
		return c.sendE2EEText(ctx, thread, text, replyTo)
	}
	return c.sendLightspeedText(ctx, thread.ID, text, replyTo)
}

func (c *Client) sendLightspeedText(ctx context.Context, threadID int64, text string, replyTo store.Message) (SendResult, error) {
	if err := c.mx.WaitUntilCanSendMessages(ctx, 15*time.Second); err != nil {
		return SendResult{}, fmt.Errorf("wait to send: %w", err)
	}
	otid := methods.GenerateEpochID()
	task := &socket.SendMessageTask{
		ThreadId:         threadID,
		Otid:             otid,
		Source:           table.MESSENGER_INBOX_IN_THREAD,
		InitiatingSource: table.FACEBOOK_INBOX,
		SendType:         table.TEXT,
		Text:             text,
		SyncGroup:        1,
	}
	if replyTo.ID != "" {
		task.ReplyMetaData = &socket.ReplyMetaData{
			ReplyMessageId:  replyTo.ID,
			ReplySourceType: 1,
			ReplySender:     replyTo.SenderID,
		}
	}
	resp, err := c.mx.ExecuteTasks(ctx, task)
	if err != nil {
		return SendResult{}, fmt.Errorf("execute send task: %w", err)
	}
	msgID, ts := extractSendResult(resp, otid)
	if msgID == "" {
		for _, failed := range resp.LSMarkOptimisticMessageFailed {
			return SendResult{}, fmt.Errorf("send rejected: %s", failed.Message)
		}
		for _, failed := range resp.LSHandleFailedTask {
			return SendResult{}, fmt.Errorf("send rejected: %s", failed.Message)
		}
		return SendResult{}, fmt.Errorf("send response missing message id")
	}
	if ts.IsZero() {
		ts = time.Now()
	}
	return SendResult{MessageID: msgID, Timestamp: ts}, nil
}

func (c *Client) sendE2EEText(ctx context.Context, thread store.Thread, text string, replyTo store.Message) (SendResult, error) {
	if c.e2ee == nil {
		return SendResult{}, fmt.Errorf("e2ee client is not connected")
	}
	otid := methods.GenerateEpochID()
	msgID := strconv.FormatInt(otid, 10)
	chat := chatJID(thread)
	payload := consumerContent(&waConsumerApplication.ConsumerApplication_Content{
		Content: &waConsumerApplication.ConsumerApplication_Content_MessageText{
			MessageText: &waCommon.MessageText{Text: proto.String(text)},
		},
	})
	var metadata *waMsgApplication.MessageApplication_Metadata
	if replyTo.ID != "" {
		participant := types.JID{User: strconv.FormatInt(replyTo.SenderID, 10), Server: types.MessengerServer}
		metadata = &waMsgApplication.MessageApplication_Metadata{
			QuotedMessage: &waMsgApplication.MessageApplication_Metadata_QuotedMessage{
				StanzaID:    proto.String(replyTo.ID),
				Participant: proto.String(participant.String()),
			},
		}
	}
	resp, err := c.e2ee.SendFBMessage(ctx, chat, payload, metadata, whatsmeow.SendRequestExtra{ID: msgID})
	if err != nil {
		return SendResult{}, fmt.Errorf("send e2ee message: %w", err)
	}
	ts := resp.Timestamp
	if ts.IsZero() {
		ts = time.Now()
	}
	return SendResult{MessageID: msgID, Timestamp: ts}, nil
}

func extractSendResult(resp *table.LSTable, otid int64) (string, time.Time) {
	if resp == nil {
		return "", time.Time{}
	}
	otidStr := strconv.FormatInt(otid, 10)
	for _, replace := range resp.LSReplaceOptimsiticMessage {
		if replace == nil {
			continue
		}
		if replace.OfflineThreadingId == otidStr && replace.MessageId != "" {
			if ts, err := methods.ParseMessageID(replace.MessageId); err == nil {
				return replace.MessageId, time.UnixMilli(ts)
			}
			return replace.MessageId, time.Time{}
		}
	}
	return "", time.Time{}
}

func trimMessage(text string) string {
	for len(text) > 0 && (text[0] == ' ' || text[0] == '\n' || text[0] == '\t' || text[0] == '\r') {
		text = text[1:]
	}
	for len(text) > 0 {
		last := text[len(text)-1]
		if last != ' ' && last != '\n' && last != '\t' && last != '\r' {
			break
		}
		text = text[:len(text)-1]
	}
	return text
}
