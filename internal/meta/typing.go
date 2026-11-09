package meta

import (
	"context"
	"fmt"
	"strconv"

	"go.mau.fi/mautrix-meta/pkg/messagix/socket"
	"go.mau.fi/mautrix-meta/pkg/messagix/table"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"

	"lazyfmess/internal/store"
)

type Typing struct {
	ThreadID int64
	SenderID int64
	Active   bool
}

func (c *Client) SetTyping(ctx context.Context, thread store.Thread, active bool) error {
	threadType := table.ThreadType(thread.ThreadType)
	if threadType.IsWhatsApp() {
		if c.e2ee == nil {
			return fmt.Errorf("e2ee client is not connected")
		}
		state := types.ChatPresencePaused
		if active {
			state = types.ChatPresenceComposing
		}
		if err := c.e2ee.SendChatPresence(ctx, chatJID(thread), state, types.ChatPresenceMediaText); err != nil {
			return fmt.Errorf("send e2ee typing: %w", err)
		}
		return nil
	}
	task := &socket.UpdatePresenceTask{
		ThreadKey:     thread.ID,
		IsGroupThread: 1,
		SyncGroup:     1,
		ThreadType:    thread.ThreadType,
	}
	if threadType.IsOneToOne() {
		task.IsGroupThread = 0
	}
	if active {
		task.IsTyping = 1
	}
	if err := c.mx.ExecuteStatelessTask(ctx, task); err != nil {
		return fmt.Errorf("send typing: %w", err)
	}
	return nil
}

func TypingFromEvent(ctx context.Context, s *store.Store, evt Event) ([]Typing, error) {
	if evt.Table == nil {
		return evt.typing, nil
	}
	out := make([]Typing, 0, len(evt.Table.LSUpdateTypingIndicator))
	for _, indicator := range evt.Table.LSUpdateTypingIndicator {
		key, err := resolveThreadKey(ctx, s, indicator.ThreadKey)
		if err != nil {
			return nil, err
		}
		out = append(out, Typing{ThreadID: key, SenderID: indicator.SenderId, Active: indicator.IsTyping})
	}
	return out, nil
}

func decodeChatPresence(evt *events.ChatPresence) (Typing, bool) {
	threadID, err := strconv.ParseInt(evt.Chat.User, 10, 64)
	if err != nil {
		return Typing{}, false
	}
	senderID, err := strconv.ParseInt(evt.Sender.User, 10, 64)
	if err != nil {
		return Typing{}, false
	}
	return Typing{ThreadID: threadID, SenderID: senderID, Active: evt.State == types.ChatPresenceComposing}, true
}
