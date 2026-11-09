package main

import (
	"context"
	"testing"
	"time"

	"lazyfmess/internal/store"
)

func TestSeedDemo(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()
	if err := seedDemo(ctx, db, time.Now()); err != nil {
		t.Fatalf("seedDemo: %v", err)
	}
	threads, err := db.ListThreads(ctx)
	if err != nil {
		t.Fatalf("ListThreads: %v", err)
	}
	unread, muted := 0, 0
	for _, th := range threads {
		if th.Unread() {
			unread++
		}
		if th.Muted(time.Now()) {
			muted++
		}
	}
	if len(threads) != len(demoThreads()) || unread != 2 || muted != 1 {
		t.Fatalf("threads %d unread %d muted %d", len(threads), unread, muted)
	}
	msgs, err := db.ListMessages(ctx, 200002)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if msgs[2].ReplyText == "" || len(msgs[4].Reactions) != 2 || msgs[0].SenderName != "Leo Martins" {
		t.Fatalf("group chat seed: %+v", msgs)
	}
}
