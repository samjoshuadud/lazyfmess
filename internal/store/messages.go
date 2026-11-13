package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

func (s *Store) OldestMessage(ctx context.Context, threadID int64) (Message, error) {
	var m Message
	var ms int64
	var pending int
	err := s.q.QueryRowContext(ctx, `
SELECT id, thread_id, sender_id, text, timestamp_ms, pending, error,
	media_kind, media_url, media_mime, media_name, media_duration_ms
FROM messages
WHERE thread_id = ?
ORDER BY timestamp_ms ASC, id ASC
LIMIT 1
`, threadID).Scan(
		&m.ID, &m.ThreadID, &m.SenderID, &m.Text, &ms, &pending, &m.Error,
		&m.MediaKind, &m.MediaURL, &m.MediaMime, &m.MediaName, &m.MediaDurationMs,
	)
	if err == sql.ErrNoRows {
		return Message{}, sql.ErrNoRows
	}
	if err != nil {
		return Message{}, fmt.Errorf("oldest message: %w", err)
	}
	m.Timestamp = time.UnixMilli(ms)
	m.Pending = pending != 0
	return m, nil
}

func (s *Store) UpsertMessage(ctx context.Context, m Message) error {
	_, err := s.q.ExecContext(ctx, `
INSERT INTO messages (
	id, thread_id, sender_id, text, timestamp_ms, pending, error,
	media_kind, media_url, media_mime, media_name, media_duration_ms,
	reply_to_id, reply_text, reply_sender_id, edited, unsent, system
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	thread_id=excluded.thread_id,
	sender_id=excluded.sender_id,
	text=CASE WHEN messages.unsent = 1 OR excluded.unsent = 1 THEN '' ELSE excluded.text END,
	edited=MAX(messages.edited, excluded.edited),
	unsent=MAX(messages.unsent, excluded.unsent),
	system=excluded.system,
	timestamp_ms=excluded.timestamp_ms,
	pending=excluded.pending,
	error=excluded.error,
	media_kind=CASE WHEN excluded.media_kind = '' THEN messages.media_kind ELSE excluded.media_kind END,
	media_url=CASE WHEN excluded.media_url = '' THEN messages.media_url ELSE excluded.media_url END,
	media_mime=CASE WHEN excluded.media_mime = '' THEN messages.media_mime ELSE excluded.media_mime END,
	media_name=CASE WHEN excluded.media_name = '' THEN messages.media_name ELSE excluded.media_name END,
	media_duration_ms=CASE WHEN excluded.media_duration_ms = 0 THEN messages.media_duration_ms ELSE excluded.media_duration_ms END,
	reply_to_id=CASE WHEN excluded.reply_to_id = '' THEN messages.reply_to_id ELSE excluded.reply_to_id END,
	reply_text=CASE WHEN excluded.reply_text = '' THEN messages.reply_text ELSE excluded.reply_text END,
	reply_sender_id=CASE WHEN excluded.reply_sender_id = 0 THEN messages.reply_sender_id ELSE excluded.reply_sender_id END
`, m.ID, m.ThreadID, m.SenderID, m.Text, m.Timestamp.UnixMilli(), boolInt(m.Pending), m.Error,
		m.MediaKind, m.MediaURL, m.MediaMime, m.MediaName, m.MediaDurationMs,
		m.ReplyToID, m.ReplyText, m.ReplySenderID, boolInt(m.Edited), boolInt(m.Unsent), boolInt(m.System))
	if err != nil {
		return fmt.Errorf("upsert message: %w", err)
	}
	return nil
}

func (s *Store) ListMessages(ctx context.Context, threadID int64) ([]Message, error) {
	return s.ListRecentMessages(ctx, threadID, -1)
}

func (s *Store) ListRecentMessages(ctx context.Context, threadID int64, limit int) ([]Message, error) {
	rows, err := s.q.QueryContext(ctx, `
SELECT m.id, m.thread_id, m.sender_id, COALESCE(c.name, ''), m.text, m.timestamp_ms, m.pending, m.error,
	m.media_kind, m.media_url, m.media_mime, m.media_name, m.media_duration_ms,
	m.reply_to_id,
	CASE WHEN m.reply_text != '' THEN m.reply_text ELSE COALESCE(r.text, '') END,
	CASE WHEN m.reply_sender_id != 0 THEN m.reply_sender_id ELSE COALESCE(r.sender_id, 0) END,
	COALESCE(rc.name, ''), m.edited, m.unsent, m.system
FROM (
	SELECT * FROM messages WHERE thread_id = ? ORDER BY timestamp_ms DESC, id DESC LIMIT ?
) m
LEFT JOIN contacts c ON c.id = m.sender_id
LEFT JOIN messages r ON m.reply_to_id != '' AND r.id = m.reply_to_id
LEFT JOIN contacts rc ON rc.id = CASE WHEN m.reply_sender_id != 0 THEN m.reply_sender_id ELSE r.sender_id END
ORDER BY m.timestamp_ms ASC, m.id ASC
`, threadID, limit)
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}
	defer rows.Close()
	var out []Message
	for rows.Next() {
		var m Message
		var ms int64
		var pending int
		if err := rows.Scan(
			&m.ID, &m.ThreadID, &m.SenderID, &m.SenderName, &m.Text, &ms, &pending, &m.Error,
			&m.MediaKind, &m.MediaURL, &m.MediaMime, &m.MediaName, &m.MediaDurationMs,
			&m.ReplyToID, &m.ReplyText, &m.ReplySenderID, &m.ReplySenderName, &m.Edited, &m.Unsent, &m.System,
		); err != nil {
			return nil, fmt.Errorf("scan message: %w", err)
		}
		m.Timestamp = time.UnixMilli(ms)
		m.Pending = pending != 0
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}
	if len(out) == 0 {
		return out, nil
	}
	return out, s.attachReactions(ctx, threadID, out)
}

func (s *Store) attachReactions(ctx context.Context, threadID int64, messages []Message) error {
	rows, err := s.q.QueryContext(ctx, `
SELECT r.message_id, r.actor_id, r.emoji
FROM reactions r
JOIN messages m ON m.id = r.message_id
WHERE m.thread_id = ? AND m.timestamp_ms >= ?
ORDER BY r.rowid
`, threadID, messages[0].Timestamp.UnixMilli())
	if err != nil {
		return fmt.Errorf("list reactions: %w", err)
	}
	defer rows.Close()
	index := make(map[string]int, len(messages))
	for i, m := range messages {
		index[m.ID] = i
	}
	for rows.Next() {
		var r Reaction
		if err := rows.Scan(&r.MessageID, &r.ActorID, &r.Emoji); err != nil {
			return fmt.Errorf("scan reaction: %w", err)
		}
		if i, ok := index[r.MessageID]; ok {
			messages[i].Reactions = append(messages[i].Reactions, r)
		}
	}
	return rows.Err()
}

func (s *Store) EditMessage(ctx context.Context, id, text string) error {
	_, err := s.q.ExecContext(ctx, `UPDATE messages SET text = ?, edited = 1 WHERE id = ? AND unsent = 0`, text, id)
	if err != nil {
		return fmt.Errorf("edit message: %w", err)
	}
	return nil
}

func (s *Store) UnsendMessage(ctx context.Context, id string) error {
	_, err := s.q.ExecContext(ctx, `
UPDATE messages SET unsent = 1, text = '', media_kind = '', media_url = '', media_mime = '', media_name = '', media_duration_ms = 0
WHERE id = ?
`, id)
	if err != nil {
		return fmt.Errorf("unsend message: %w", err)
	}
	return nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (s *Store) SetReaction(ctx context.Context, r Reaction) error {
	if r.MessageID == "" || r.ActorID == 0 {
		return nil
	}
	var err error
	if r.Emoji == "" {
		_, err = s.q.ExecContext(ctx, `DELETE FROM reactions WHERE message_id = ? AND actor_id = ?`, r.MessageID, r.ActorID)
	} else {
		_, err = s.q.ExecContext(ctx, `
INSERT INTO reactions (message_id, actor_id, emoji) VALUES (?, ?, ?)
ON CONFLICT(message_id, actor_id) DO UPDATE SET emoji=excluded.emoji
`, r.MessageID, r.ActorID, r.Emoji)
	}
	if err != nil {
		return fmt.Errorf("set reaction: %w", err)
	}
	return nil
}

func (s *Store) UnreadMessageIDs(ctx context.Context, threadID, selfID int64) (map[int64][]string, error) {
	rows, err := s.q.QueryContext(ctx, `
SELECT m.sender_id, m.id
FROM messages m
JOIN threads t ON t.id = m.thread_id
WHERE m.thread_id = ? AND m.sender_id != ? AND m.timestamp_ms > t.last_read_ms AND m.pending = 0
ORDER BY m.timestamp_ms ASC
`, threadID, selfID)
	if err != nil {
		return nil, fmt.Errorf("unread messages: %w", err)
	}
	defer rows.Close()
	out := make(map[int64][]string)
	for rows.Next() {
		var sender int64
		var id string
		if err := rows.Scan(&sender, &id); err != nil {
			return nil, fmt.Errorf("scan unread message: %w", err)
		}
		out[sender] = append(out[sender], id)
	}
	return out, rows.Err()
}

func (s *Store) DeleteMessage(ctx context.Context, id string) error {
	_, err := s.q.ExecContext(ctx, `DELETE FROM messages WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete message: %w", err)
	}
	return nil
}
