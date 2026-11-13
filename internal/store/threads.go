package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

func (s *Store) UpsertContact(ctx context.Context, id int64, name string) error {
	if id == 0 || name == "" {
		return nil
	}
	_, err := s.q.ExecContext(ctx, `
INSERT INTO contacts (id, name)
VALUES (?, ?)
ON CONFLICT(id) DO UPDATE SET
	name=excluded.name
`, id, name)
	if err != nil {
		return fmt.Errorf("upsert contact: %w", err)
	}
	return nil
}

func (s *Store) ContactName(ctx context.Context, id int64) (string, error) {
	var name string
	err := s.q.QueryRowContext(ctx, `SELECT name FROM contacts WHERE id = ?`, id).Scan(&name)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("contact name: %w", err)
	}
	return name, nil
}

func (s *Store) PutHybridThread(ctx context.Context, fbKey, jid, threadType int64) error {
	if fbKey == 0 || jid == 0 {
		return nil
	}
	_, err := s.q.ExecContext(ctx, `
INSERT INTO hybrid_threads (fb_key, jid, thread_type)
VALUES (?, ?, ?)
ON CONFLICT(fb_key) DO UPDATE SET
	jid=excluded.jid,
	thread_type=CASE WHEN excluded.thread_type = 0 THEN hybrid_threads.thread_type ELSE excluded.thread_type END
`, fbKey, jid, threadType)
	if err != nil {
		return fmt.Errorf("put hybrid thread: %w", err)
	}
	return nil
}

func (s *Store) HybridThreadJID(ctx context.Context, fbKey int64) (int64, error) {
	var jid int64
	err := s.q.QueryRowContext(ctx, `SELECT jid FROM hybrid_threads WHERE fb_key = ?`, fbKey).Scan(&jid)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("hybrid thread jid: %w", err)
	}
	return jid, nil
}

func (s *Store) UpsertThread(ctx context.Context, t Thread) error {
	_, err := s.q.ExecContext(ctx, `
INSERT INTO threads (id, title, last_preview, thread_type, updated_at_ms)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	title=CASE WHEN excluded.title = '' THEN threads.title ELSE excluded.title END,
	last_preview=CASE
		WHEN excluded.last_preview = '' THEN threads.last_preview
		WHEN threads.last_preview = '' OR excluded.updated_at_ms >= threads.updated_at_ms THEN excluded.last_preview
		ELSE threads.last_preview
	END,
	thread_type=CASE WHEN excluded.thread_type = 0 THEN threads.thread_type ELSE excluded.thread_type END,
	updated_at_ms=MAX(threads.updated_at_ms, excluded.updated_at_ms)
`, t.ID, t.Title, t.LastPreview, t.ThreadType, unixMilli(t.UpdatedAt))
	if err != nil {
		return fmt.Errorf("upsert thread: %w", err)
	}
	return nil
}

func (s *Store) ListThreads(ctx context.Context) ([]Thread, error) {
	rows, err := s.q.QueryContext(ctx, `
SELECT t.id,
	CASE WHEN t.title != '' THEN t.title ELSE COALESCE(c.name, '') END,
	t.last_preview, t.thread_type, t.updated_at_ms, t.last_read_ms, t.mute_until_ms
FROM threads t
LEFT JOIN contacts c ON c.id = t.id
ORDER BY t.updated_at_ms DESC, t.id DESC
`)
	if err != nil {
		return nil, fmt.Errorf("list threads: %w", err)
	}
	defer rows.Close()
	var out []Thread
	for rows.Next() {
		var t Thread
		var ms, readMS int64
		if err := rows.Scan(&t.ID, &t.Title, &t.LastPreview, &t.ThreadType, &ms, &readMS, &t.MuteUntilMs); err != nil {
			return nil, fmt.Errorf("scan thread: %w", err)
		}
		t.UpdatedAt = time.UnixMilli(ms)
		t.LastRead = time.UnixMilli(readMS)
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) GetThread(ctx context.Context, id int64) (Thread, error) {
	var t Thread
	var ms, readMS int64
	err := s.q.QueryRowContext(ctx, `
SELECT t.id,
	CASE WHEN t.title != '' THEN t.title ELSE COALESCE(c.name, '') END,
	t.last_preview, t.thread_type, t.updated_at_ms, t.last_read_ms, t.mute_until_ms
FROM threads t
LEFT JOIN contacts c ON c.id = t.id
WHERE t.id = ?
`, id).Scan(&t.ID, &t.Title, &t.LastPreview, &t.ThreadType, &ms, &readMS, &t.MuteUntilMs)
	if err != nil {
		return Thread{}, fmt.Errorf("get thread: %w", err)
	}
	t.UpdatedAt = time.UnixMilli(ms)
	t.LastRead = time.UnixMilli(readMS)
	return t, nil
}

func (s *Store) SetParticipantRead(ctx context.Context, threadID, contactID int64, at time.Time) error {
	if threadID == 0 || contactID == 0 || at.UnixMilli() <= 0 {
		return nil
	}
	_, err := s.q.ExecContext(ctx, `
INSERT INTO participant_reads (thread_id, contact_id, read_ms) VALUES (?, ?, ?)
ON CONFLICT(thread_id, contact_id) DO UPDATE SET read_ms = MAX(read_ms, excluded.read_ms)
`, threadID, contactID, at.UnixMilli())
	if err != nil {
		return fmt.Errorf("set participant read: %w", err)
	}
	return nil
}

func (s *Store) ReadMarks(ctx context.Context, threadID, selfID int64) ([]time.Time, error) {
	rows, err := s.q.QueryContext(ctx, `SELECT read_ms FROM participant_reads WHERE thread_id = ? AND contact_id != ?`, threadID, selfID)
	if err != nil {
		return nil, fmt.Errorf("read marks: %w", err)
	}
	defer rows.Close()
	var out []time.Time
	for rows.Next() {
		var ms int64
		if err := rows.Scan(&ms); err != nil {
			return nil, fmt.Errorf("scan read mark: %w", err)
		}
		out = append(out, time.UnixMilli(ms))
	}
	return out, rows.Err()
}

func (s *Store) ThreadCount(ctx context.Context) (int, error) {
	var n int
	if err := s.q.QueryRowContext(ctx, `SELECT COUNT(*) FROM threads`).Scan(&n); err != nil {
		return 0, fmt.Errorf("count threads: %w", err)
	}
	return n, nil
}

func (s *Store) SetMute(ctx context.Context, threadID, untilMs int64) error {
	_, err := s.q.ExecContext(ctx, `UPDATE threads SET mute_until_ms = ? WHERE id = ?`, untilMs, threadID)
	if err != nil {
		return fmt.Errorf("set mute: %w", err)
	}
	return nil
}

func (s *Store) SetLastRead(ctx context.Context, threadID int64, at time.Time) error {
	if at.IsZero() || at.UnixMilli() <= 0 {
		return nil
	}
	_, err := s.q.ExecContext(ctx, `UPDATE threads SET last_read_ms = MAX(last_read_ms, ?) WHERE id = ?`, at.UnixMilli(), threadID)
	if err != nil {
		return fmt.Errorf("set last read: %w", err)
	}
	return nil
}
