package store

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const DBFileName = "messages.db"

type Thread struct {
	ID          int64
	Title       string
	LastPreview string
	ThreadType  int64
	UpdatedAt   time.Time
	LastRead    time.Time
}

func (t Thread) Unread() bool {
	return t.UpdatedAt.UnixMilli() > t.LastRead.UnixMilli()
}

type Message struct {
	ID              string
	ThreadID        int64
	SenderID        int64
	SenderName      string
	Text            string
	Timestamp       time.Time
	Pending         bool
	Error           string
	MediaKind       string
	MediaURL        string
	MediaMime       string
	MediaName       string
	MediaDurationMs int64
}

type Store struct {
	db *sql.DB
}

func Open(ctx context.Context, dataDir string) (*Store, error) {
	path := filepath.Join(dataDir, DBFileName)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

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
CREATE INDEX IF NOT EXISTS messages_thread_ts ON messages(thread_id, timestamp_ms);
`
	if _, err := s.db.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	for _, stmt := range []string{
		`ALTER TABLE threads ADD COLUMN thread_type INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE threads ADD COLUMN last_read_ms INTEGER NOT NULL DEFAULT 0`,
	} {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
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
	} {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			if !isDuplicateColumn(err) {
				return fmt.Errorf("migrate media columns: %w", err)
			}
		}
	}
	return nil
}

func isDuplicateColumn(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate column") || strings.Contains(msg, "already exists")
}

func (s *Store) UpsertContact(ctx context.Context, id int64, name string) error {
	if id == 0 || name == "" {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `
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
	err := s.db.QueryRowContext(ctx, `SELECT name FROM contacts WHERE id = ?`, id).Scan(&name)
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
	_, err := s.db.ExecContext(ctx, `
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
	err := s.db.QueryRowContext(ctx, `SELECT jid FROM hybrid_threads WHERE fb_key = ?`, fbKey).Scan(&jid)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("hybrid thread jid: %w", err)
	}
	return jid, nil
}

func (s *Store) OldestMessage(ctx context.Context, threadID int64) (Message, error) {
	var m Message
	var ms int64
	var pending int
	err := s.db.QueryRowContext(ctx, `
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

func (s *Store) UpsertThread(ctx context.Context, t Thread) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO threads (id, title, last_preview, thread_type, updated_at_ms)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	title=CASE WHEN excluded.title = '' THEN threads.title ELSE excluded.title END,
	last_preview=CASE WHEN excluded.last_preview = '' THEN threads.last_preview ELSE excluded.last_preview END,
	thread_type=CASE WHEN excluded.thread_type = 0 THEN threads.thread_type ELSE excluded.thread_type END,
	updated_at_ms=CASE WHEN excluded.updated_at_ms = 0 THEN threads.updated_at_ms ELSE excluded.updated_at_ms END
`, t.ID, t.Title, t.LastPreview, t.ThreadType, t.UpdatedAt.UnixMilli())
	if err != nil {
		return fmt.Errorf("upsert thread: %w", err)
	}
	return nil
}

func (s *Store) UpsertMessage(ctx context.Context, m Message) error {
	pending := 0
	if m.Pending {
		pending = 1
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO messages (
	id, thread_id, sender_id, text, timestamp_ms, pending, error,
	media_kind, media_url, media_mime, media_name, media_duration_ms
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	thread_id=excluded.thread_id,
	sender_id=excluded.sender_id,
	text=excluded.text,
	timestamp_ms=excluded.timestamp_ms,
	pending=excluded.pending,
	error=excluded.error,
	media_kind=CASE WHEN excluded.media_kind = '' THEN messages.media_kind ELSE excluded.media_kind END,
	media_url=CASE WHEN excluded.media_url = '' THEN messages.media_url ELSE excluded.media_url END,
	media_mime=CASE WHEN excluded.media_mime = '' THEN messages.media_mime ELSE excluded.media_mime END,
	media_name=CASE WHEN excluded.media_name = '' THEN messages.media_name ELSE excluded.media_name END,
	media_duration_ms=CASE WHEN excluded.media_duration_ms = 0 THEN messages.media_duration_ms ELSE excluded.media_duration_ms END
`, m.ID, m.ThreadID, m.SenderID, m.Text, m.Timestamp.UnixMilli(), pending, m.Error,
		m.MediaKind, m.MediaURL, m.MediaMime, m.MediaName, m.MediaDurationMs)
	if err != nil {
		return fmt.Errorf("upsert message: %w", err)
	}
	return nil
}

func (s *Store) ListThreads(ctx context.Context) ([]Thread, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT t.id,
	CASE WHEN t.title != '' THEN t.title ELSE COALESCE(c.name, '') END,
	t.last_preview, t.thread_type, t.updated_at_ms, t.last_read_ms
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
		if err := rows.Scan(&t.ID, &t.Title, &t.LastPreview, &t.ThreadType, &ms, &readMS); err != nil {
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
	err := s.db.QueryRowContext(ctx, `
SELECT t.id,
	CASE WHEN t.title != '' THEN t.title ELSE COALESCE(c.name, '') END,
	t.last_preview, t.thread_type, t.updated_at_ms, t.last_read_ms
FROM threads t
LEFT JOIN contacts c ON c.id = t.id
WHERE t.id = ?
`, id).Scan(&t.ID, &t.Title, &t.LastPreview, &t.ThreadType, &ms, &readMS)
	if err != nil {
		return Thread{}, fmt.Errorf("get thread: %w", err)
	}
	t.UpdatedAt = time.UnixMilli(ms)
	t.LastRead = time.UnixMilli(readMS)
	return t, nil
}

func (s *Store) ListMessages(ctx context.Context, threadID int64) ([]Message, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT m.id, m.thread_id, m.sender_id, COALESCE(c.name, ''), m.text, m.timestamp_ms, m.pending, m.error,
	m.media_kind, m.media_url, m.media_mime, m.media_name, m.media_duration_ms
FROM messages m
LEFT JOIN contacts c ON c.id = m.sender_id
WHERE m.thread_id = ?
ORDER BY m.timestamp_ms ASC, m.id ASC
`, threadID)
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
		); err != nil {
			return nil, fmt.Errorf("scan message: %w", err)
		}
		m.Timestamp = time.UnixMilli(ms)
		m.Pending = pending != 0
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) ThreadCount(ctx context.Context) (int, error) {
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM threads`).Scan(&n); err != nil {
		return 0, fmt.Errorf("count threads: %w", err)
	}
	return n, nil
}

func (s *Store) SetLastRead(ctx context.Context, threadID int64, at time.Time) error {
	if at.IsZero() || at.UnixMilli() <= 0 {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `UPDATE threads SET last_read_ms = MAX(last_read_ms, ?) WHERE id = ?`, at.UnixMilli(), threadID)
	if err != nil {
		return fmt.Errorf("set last read: %w", err)
	}
	return nil
}

func (s *Store) UnreadMessageIDs(ctx context.Context, threadID, selfID int64) (map[int64][]string, error) {
	rows, err := s.db.QueryContext(ctx, `
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
	_, err := s.db.ExecContext(ctx, `DELETE FROM messages WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete message: %w", err)
	}
	return nil
}
