package meta

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestWaitReady(t *testing.T) {
	t.Parallel()
	c := &Client{ready: make(chan struct{})}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := c.waitReady(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("before connect: %v", err)
	}
	c.connectErr = errors.New("expired")
	close(c.ready)
	if err := c.waitReady(context.Background()); err == nil || err.Error() != "not connected: expired" {
		t.Fatalf("after failed connect: %v", err)
	}
	ok := &Client{ready: make(chan struct{}), cookieUserID: 42}
	close(ok.ready)
	if err := ok.waitReady(context.Background()); err != nil || ok.AccountID() != 42 {
		t.Fatalf("after connect: %v account %d", err, ok.AccountID())
	}
}
