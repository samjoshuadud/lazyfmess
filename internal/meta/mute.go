package meta

import (
	"context"
	"fmt"

	"go.mau.fi/mautrix-meta/pkg/messagix/socket"

	"lazyfmess/internal/store"
)

func (c *Client) SetMuted(ctx context.Context, s *store.Store, thread store.Thread, muted bool) error {
	if err := c.waitReady(ctx); err != nil {
		return err
	}
	var until int64
	if muted {
		until = -1
	}
	if _, err := c.mx.ExecuteTasks(ctx, &socket.MuteThreadTask{
		ThreadKey:        thread.ID,
		MuteExpireTimeMS: until,
		SyncGroup:        1,
	}); err != nil {
		return fmt.Errorf("mute thread: %w", err)
	}
	return s.SetMute(ctx, thread.ID, until)
}
