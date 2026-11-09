package meta

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"go.mau.fi/mautrix-meta/pkg/messagix/socket"
	"go.mau.fi/mautrix-meta/pkg/messagix/table"
	"go.mau.fi/whatsmeow/types"

	"lazyfmess/internal/store"
)

func (c *Client) MarkRead(ctx context.Context, s *store.Store, thread store.Thread) error {
	if !thread.Unread() {
		return nil
	}
	if table.ThreadType(thread.ThreadType).IsWhatsApp() {
		if err := c.markE2EERead(ctx, s, thread); err != nil {
			return err
		}
	} else if _, err := c.mx.ExecuteTasks(ctx, &socket.ThreadMarkReadTask{
		ThreadId:            thread.ID,
		LastReadWatermarkTs: thread.UpdatedAt.UnixMilli(),
		SyncGroup:           1,
	}); err != nil {
		return fmt.Errorf("mark thread read: %w", err)
	}
	return s.SetLastRead(ctx, thread.ID, thread.UpdatedAt)
}

func (c *Client) markE2EERead(ctx context.Context, s *store.Store, thread store.Thread) error {
	if c.e2ee == nil {
		return fmt.Errorf("e2ee client is not connected")
	}
	unread, err := s.UnreadMessageIDs(ctx, thread.ID, c.AccountID())
	if err != nil {
		return err
	}
	chat := chatJID(thread)
	for senderID, ids := range unread {
		var sender types.JID
		if chat.Server == types.GroupServer {
			sender = types.JID{User: strconv.FormatInt(senderID, 10), Server: types.MessengerServer}
		}
		if err := c.e2ee.MarkRead(ctx, ids, time.Now(), chat, sender); err != nil {
			return fmt.Errorf("send e2ee read receipt: %w", err)
		}
	}
	return nil
}

func chatJID(thread store.Thread) types.JID {
	server := types.MessengerServer
	if table.ThreadType(thread.ThreadType) == table.ENCRYPTED_OVER_WA_GROUP {
		server = types.GroupServer
	}
	return types.JID{User: strconv.FormatInt(thread.ID, 10), Server: server}
}
