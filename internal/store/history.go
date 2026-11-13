package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

type HistoryState struct {
	Before       time.Time
	ReachedStart bool
}

func (s *Store) HistoryState(ctx context.Context, threadID int64) (HistoryState, bool, error) {
	var beforeMS int64
	var reached int
	err := s.q.QueryRowContext(ctx, `SELECT before_ms, reached_start FROM history_state WHERE thread_id = ?`, threadID).Scan(&beforeMS, &reached)
	if errors.Is(err, sql.ErrNoRows) {
		return HistoryState{}, false, nil
	}
	if err != nil {
		return HistoryState{}, false, fmt.Errorf("history state: %w", err)
	}
	return HistoryState{Before: time.UnixMilli(beforeMS), ReachedStart: reached != 0}, true, nil
}

func (s *Store) SetHistoryState(ctx context.Context, threadID int64, state HistoryState) error {
	_, err := s.q.ExecContext(ctx, `
INSERT INTO history_state (thread_id, before_ms, reached_start, updated_at_ms) VALUES (?, ?, ?, ?)
ON CONFLICT(thread_id) DO UPDATE SET
	before_ms = excluded.before_ms,
	reached_start = excluded.reached_start,
	updated_at_ms = excluded.updated_at_ms
`, threadID, state.Before.UnixMilli(), boolInt(state.ReachedStart), time.Now().UnixMilli())
	if err != nil {
		return fmt.Errorf("set history state: %w", err)
	}
	return nil
}

func (s *Store) PendingHistoryThreads(ctx context.Context) ([]Thread, error) {
	rows, err := s.q.QueryContext(ctx, `
SELECT t.id, t.title, t.last_preview, t.thread_type, t.updated_at_ms
FROM threads t
LEFT JOIN history_state h ON h.thread_id = t.id
WHERE COALESCE(h.reached_start, 0) = 0
ORDER BY t.updated_at_ms DESC, t.id DESC
`)
	if err != nil {
		return nil, fmt.Errorf("pending history threads: %w", err)
	}
	defer rows.Close()
	var out []Thread
	for rows.Next() {
		var t Thread
		var ms int64
		if err := rows.Scan(&t.ID, &t.Title, &t.LastPreview, &t.ThreadType, &ms); err != nil {
			return nil, fmt.Errorf("scan pending history thread: %w", err)
		}
		t.UpdatedAt = time.UnixMilli(ms)
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) MessageTimeRange(ctx context.Context, threadID int64) (oldest, newest time.Time, ok bool, err error) {
	var minMS, maxMS sql.NullInt64
	err = s.q.QueryRowContext(ctx, `
SELECT MIN(timestamp_ms), MAX(timestamp_ms) FROM messages WHERE thread_id = ? AND pending = 0
`, threadID).Scan(&minMS, &maxMS)
	if err != nil {
		return time.Time{}, time.Time{}, false, fmt.Errorf("message time range: %w", err)
	}
	if !minMS.Valid {
		return time.Time{}, time.Time{}, false, nil
	}
	return time.UnixMilli(minMS.Int64), time.UnixMilli(maxMS.Int64), true, nil
}
