package ui

import (
	"context"
	"lazyfmess/internal/store"
	"time"
)

type SendResult struct {
	MessageID string
	Timestamp time.Time
}

type Sender interface {
	SendText(ctx context.Context, thread store.Thread, text string, replyTo store.Message) (SendResult, error)
	SendFile(ctx context.Context, thread store.Thread, path string) (SendResult, error)
	AccountID() int64
}

type HistoryLoader interface {
	LoadHistory(ctx context.Context, thread store.Thread) error
}

type HistoryUnlocker interface {
	NeedsPIN(ctx context.Context) (bool, error)
	Unlock(ctx context.Context, pin string) error
}

type MediaOpener interface {
	OpenMedia(ctx context.Context, msg store.Message) error
}

type ReadMarker interface {
	MarkRead(ctx context.Context, thread store.Thread) error
}

type Services struct {
	Sender   Sender
	History  HistoryLoader
	Unlocker HistoryUnlocker
	Searcher PersonSearcher
	Media    MediaOpener
	Reader   ReadMarker
	Typing   TypingNotifier
	Reactor  Reactor
	Editor   MessageEditor
	Muter    Muter
	Login    LoginService
}

type Config struct {
	PrefillPIN     string
	Connecting     bool
	BackgroundSync func(ctx context.Context) error
}
