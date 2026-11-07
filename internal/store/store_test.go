package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestUpsertAndList(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	now := time.UnixMilli(1_700_000_000_000)
	tests := []struct {
		thread  Thread
		message Message
	}{
		{
			thread:  Thread{ID: 1, Title: "Alice", LastPreview: "hi", UpdatedAt: now},
			message: Message{ID: "m1", ThreadID: 1, SenderID: 10, Text: "hi", Timestamp: now},
		},
		{
			thread:  Thread{ID: 2, Title: "Bob", LastPreview: "yo", UpdatedAt: now.Add(time.Second)},
			message: Message{ID: "m2", ThreadID: 2, SenderID: 20, Text: "yo", Timestamp: now.Add(time.Second)},
		},
	}
	for _, tt := range tests {
		if err := s.UpsertThread(ctx, tt.thread); err != nil {
			t.Fatalf("UpsertThread: %v", err)
		}
		if err := s.UpsertMessage(ctx, tt.message); err != nil {
			t.Fatalf("UpsertMessage: %v", err)
		}
	}
	if err := s.UpsertThread(ctx, Thread{ID: 1, Title: "Alice", LastPreview: "updated", UpdatedAt: now.Add(2 * time.Second)}); err != nil {
		t.Fatalf("UpsertThread update: %v", err)
	}

	threads, err := s.ListThreads(ctx)
	if err != nil {
		t.Fatalf("ListThreads: %v", err)
	}
	if len(threads) != 2 {
		t.Fatalf("thread count: got %d want 2", len(threads))
	}
	if threads[0].ID != 1 || threads[0].LastPreview != "updated" {
		t.Fatalf("unexpected first thread: %+v", threads[0])
	}

	msgs, err := s.ListMessages(ctx, 1)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(msgs) != 1 || msgs[0].Text != "hi" {
		t.Fatalf("unexpected messages: %+v", msgs)
	}

	n, err := s.ThreadCount(ctx)
	if err != nil {
		t.Fatalf("ThreadCount: %v", err)
	}
	if n != 2 {
		t.Fatalf("ThreadCount: got %d want 2", n)
	}

	if filepath.Base(DBFileName) != DBFileName {
		t.Fatal("unexpected db file name")
	}
}
