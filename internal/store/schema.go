package store

import (
	"context"
	"fmt"
	"strings"
	"time"
)

func (s *Store) migrate(ctx context.Context) error {
	const schema = `
CREATE TABLE IF NOT EXISTS threads (
	id INTEGER PRIMARY KEY,
	title TEXT NOT NULL DEFAULT '',
	last_preview TEXT NOT NULL DEFAULT '',
	thread_type INTEGER NOT NULL DEFAULT 0,
	updated_at_ms INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS messages (
	id TEXT PRIMARY KEY,
	thread_id INTEGER NOT NULL,
	sender_id INTEGER NOT NULL DEFAULT 0,
	text TEXT NOT NULL DEFAULT '',
	timestamp_ms INTEGER NOT NULL DEFAULT 0,
	pending INTEGER NOT NULL DEFAULT 0,
	error TEXT NOT NULL DEFAULT '',
	media_kind TEXT NOT NULL DEFAULT '',
	media_url TEXT NOT NULL DEFAULT '',
	media_mime TEXT NOT NULL DEFAULT '',
	media_name TEXT NOT NULL DEFAULT '',
	media_duration_ms INTEGER NOT NULL DEFAULT 0,
	FOREIGN KEY(thread_id) REFERENCES threads(id)
);
CREATE TABLE IF NOT EXISTS contacts (
	id INTEGER PRIMARY KEY,
	name TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS hybrid_threads (
	fb_key INTEGER PRIMARY KEY,
	jid INTEGER NOT NULL,
	thread_type INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS reactions (
	message_id TEXT NOT NULL,
	actor_id INTEGER NOT NULL,
	emoji TEXT NOT NULL,
	PRIMARY KEY (message_id, actor_id)
);
CREATE TABLE IF NOT EXISTS participant_reads (
	thread_id INTEGER NOT NULL,
	contact_id INTEGER NOT NULL,
	read_ms INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (thread_id, contact_id)
);
CREATE TABLE IF NOT EXISTS history_state (
	thread_id INTEGER PRIMARY KEY,
	before_ms INTEGER NOT NULL DEFAULT 0,
	reached_start INTEGER NOT NULL DEFAULT 0,
	updated_at_ms INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS messages_thread_ts ON messages(thread_id, timestamp_ms);
`
	if _, err := s.q.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	for _, stmt := range []string{
		`ALTER TABLE threads ADD COLUMN thread_type INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE threads ADD COLUMN last_read_ms INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE threads ADD COLUMN mute_until_ms INTEGER NOT NULL DEFAULT 0`,
	} {
		if _, err := s.q.ExecContext(ctx, stmt); err != nil {
			if !isDuplicateColumn(err) {
				return fmt.Errorf("migrate thread columns: %w", err)
			}
		}
	}
	for _, stmt := range []string{
		`ALTER TABLE messages ADD COLUMN media_kind TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE messages ADD COLUMN media_url TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE messages ADD COLUMN media_mime TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE messages ADD COLUMN media_name TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE messages ADD COLUMN media_duration_ms INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE messages ADD COLUMN reply_to_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE messages ADD COLUMN reply_text TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE messages ADD COLUMN reply_sender_id INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE messages ADD COLUMN edited INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE messages ADD COLUMN unsent INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE messages ADD COLUMN system INTEGER NOT NULL DEFAULT 0`,
	} {
		if _, err := s.q.ExecContext(ctx, stmt); err != nil {
			if !isDuplicateColumn(err) {
				return fmt.Errorf("migrate message columns: %w", err)
			}
		}
	}
	if _, err := s.q.ExecContext(ctx, `
UPDATE threads SET
	updated_at_ms = newest.ts,
	last_preview = CASE
		WHEN newest.text != '' THEN newest.text
		WHEN newest.media_kind != '' THEN '[' || newest.media_kind || ']'
		ELSE threads.last_preview
	END
FROM (
	SELECT thread_id, text, media_kind, MAX(timestamp_ms) AS ts
	FROM messages WHERE unsent = 0 AND pending = 0 GROUP BY thread_id
) AS newest
WHERE newest.thread_id = threads.id AND newest.ts > threads.updated_at_ms
`); err != nil {
		return fmt.Errorf("repair thread activity: %w", err)
	}
	return nil
}

func unixMilli(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixMilli()
}

func isDuplicateColumn(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate column") || strings.Contains(msg, "already exists")
}
