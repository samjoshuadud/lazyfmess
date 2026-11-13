package store

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"path/filepath"
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
