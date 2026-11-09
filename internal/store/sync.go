package store

import (
	"context"
	"fmt"
)

func (s *Store) EnsureThread(ctx context.Context, id int64) error {
	_, err := s.q.ExecContext(ctx, `
INSERT INTO threads (id, title, last_preview, thread_type, updated_at_ms)
VALUES (?, '', '', 0, 0)
ON CONFLICT(id) DO NOTHING
`, id)
	if err != nil {
		return fmt.Errorf("ensure thread: %w", err)
	}
	return nil
}
