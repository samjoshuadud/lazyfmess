package store

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
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
	MuteUntilMs int64
}

func (t Thread) Unread() bool {
	return t.UpdatedAt.UnixMilli() > t.LastRead.UnixMilli()
}

func (t Thread) Muted(now time.Time) bool {
	return t.MuteUntilMs == -1 || t.MuteUntilMs > now.UnixMilli()
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
	ReplyToID       string
	ReplyText       string
	ReplySenderID   int64
	ReplySenderName string
	Edited          bool
	Unsent          bool
	System          bool
	Reactions       []Reaction
}

type Reaction struct {
	MessageID string
	ActorID   int64
	Emoji     string
}

type queryer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type Store struct {
	db *sql.DB
	q  queryer
}

func Open(ctx context.Context, dataDir string) (*Store, error) {
	params := url.Values{}
	for _, pragma := range []string{"journal_mode(WAL)", "synchronous(NORMAL)", "busy_timeout(5000)"} {
		params.Add("_pragma", pragma)
	}
	params.Set("_txlock", "immediate")
	dsn := (&url.URL{Scheme: "file", Path: filepath.Join(dataDir, DBFileName), RawQuery: params.Encode()}).String()
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	db.SetMaxOpenConns(4)
	s := &Store{db: db, q: db}
	if err := s.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) InTx(ctx context.Context, fn func(*Store) error) error {
	if _, ok := s.q.(*sql.Tx); ok {
		return fn(s)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	if err := fn(&Store{db: s.db, q: tx}); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
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

func (s *Store) ListMessages(ctx context.Context, threadID int64) ([]Message, error) {
	rows, err := s.q.QueryContext(ctx, `
SELECT m.id, m.thread_id, m.sender_id, COALESCE(c.name, ''), m.text, m.timestamp_ms, m.pending, m.error,
	m.media_kind, m.media_url, m.media_mime, m.media_name, m.media_duration_ms,
	m.reply_to_id,
	CASE WHEN m.reply_text != '' THEN m.reply_text ELSE COALESCE(r.text, '') END,
	CASE WHEN m.reply_sender_id != 0 THEN m.reply_sender_id ELSE COALESCE(r.sender_id, 0) END,
	COALESCE(rc.name, ''), m.edited, m.unsent, m.system
FROM messages m
LEFT JOIN contacts c ON c.id = m.sender_id
LEFT JOIN messages r ON m.reply_to_id != '' AND r.id = m.reply_to_id
LEFT JOIN contacts rc ON rc.id = CASE WHEN m.reply_sender_id != 0 THEN m.reply_sender_id ELSE r.sender_id END
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
	return out, s.attachReactions(ctx, threadID, out)
}

func (s *Store) attachReactions(ctx context.Context, threadID int64, messages []Message) error {
	rows, err := s.q.QueryContext(ctx, `
SELECT r.message_id, r.actor_id, r.emoji
FROM reactions r
JOIN messages m ON m.id = r.message_id
WHERE m.thread_id = ?
ORDER BY r.rowid
`, threadID)
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
