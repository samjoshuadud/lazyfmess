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
}

type Message struct {
	ID         string
	ThreadID   int64
	SenderID   int64
	SenderName string
	Text       string
	Timestamp  time.Time
	Pending    bool
	Error      string
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
	if _, err := s.db.ExecContext(ctx, `ALTER TABLE threads ADD COLUMN thread_type INTEGER NOT NULL DEFAULT 0`); err != nil {
		if !isDuplicateColumn(err) {
			return fmt.Errorf("migrate thread_type: %w", err)
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
SELECT id, thread_id, sender_id, text, timestamp_ms, pending, error
FROM messages
WHERE thread_id = ?
ORDER BY timestamp_ms ASC, id ASC
LIMIT 1
`, threadID).Scan(&m.ID, &m.ThreadID, &m.SenderID, &m.Text, &ms, &pending, &m.Error)
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
INSERT INTO messages (id, thread_id, sender_id, text, timestamp_ms, pending, error)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	thread_id=excluded.thread_id,
	sender_id=excluded.sender_id,
	text=excluded.text,
	timestamp_ms=excluded.timestamp_ms,
	pending=excluded.pending,
	error=excluded.error
`, m.ID, m.ThreadID, m.SenderID, m.Text, m.Timestamp.UnixMilli(), pending, m.Error)
	if err != nil {
		return fmt.Errorf("upsert message: %w", err)
	}
	return nil
}

func (s *Store) ListThreads(ctx context.Context) ([]Thread, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT t.id,
	CASE WHEN t.title != '' THEN t.title ELSE COALESCE(c.name, '') END,
	t.last_preview, t.thread_type, t.updated_at_ms
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
		var ms int64
		if err := rows.Scan(&t.ID, &t.Title, &t.LastPreview, &t.ThreadType, &ms); err != nil {
			return nil, fmt.Errorf("scan thread: %w", err)
		}
		t.UpdatedAt = time.UnixMilli(ms)
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) GetThread(ctx context.Context, id int64) (Thread, error) {
	var t Thread
	var ms int64
	err := s.db.QueryRowContext(ctx, `
SELECT t.id,
	CASE WHEN t.title != '' THEN t.title ELSE COALESCE(c.name, '') END,
	t.last_preview, t.thread_type, t.updated_at_ms
FROM threads t
LEFT JOIN contacts c ON c.id = t.id
WHERE t.id = ?
`, id).Scan(&t.ID, &t.Title, &t.LastPreview, &t.ThreadType, &ms)
	if err != nil {
		return Thread{}, fmt.Errorf("get thread: %w", err)
	}
	t.UpdatedAt = time.UnixMilli(ms)
	return t, nil
}

func (s *Store) ListMessages(ctx context.Context, threadID int64) ([]Message, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT m.id, m.thread_id, m.sender_id, COALESCE(c.name, ''), m.text, m.timestamp_ms, m.pending, m.error
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
		if err := rows.Scan(&m.ID, &m.ThreadID, &m.SenderID, &m.SenderName, &m.Text, &ms, &pending, &m.Error); err != nil {
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

func (s *Store) DeleteMessage(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM messages WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete message: %w", err)
	}
	return nil
}
