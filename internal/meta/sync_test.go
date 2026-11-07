package meta

import (
	"context"
	"testing"

	"go.mau.fi/mautrix-meta/pkg/messagix/table"

	"fmesstui/internal/store"
)

func TestApplyTable(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	tbl := &table.LSTable{
		LSDeleteThenInsertThread: []*table.LSDeleteThenInsertThread{
			{
				ThreadKey:               42,
				ThreadName:              "Charlie",
				Snippet:                 "hello",
				LastActivityTimestampMs: 1_700_000_000_000,
			},
		},
		LSInsertMessage: []*table.LSInsertMessage{
			{
				MessageId:   "mid-1",
				ThreadKey:   42,
				SenderId:    99,
				Text:        "hello",
				TimestampMs: 1_700_000_000_100,
			},
		},
	}
	if err := ApplyTable(ctx, s, tbl); err != nil {
		t.Fatalf("ApplyTable: %v", err)
	}
	threads, err := s.ListThreads(ctx)
	if err != nil {
		t.Fatalf("ListThreads: %v", err)
	}
	if len(threads) != 1 || threads[0].Title != "Charlie" {
		t.Fatalf("unexpected threads: %+v", threads)
	}
	msgs, err := s.ListMessages(ctx, 42)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(msgs) != 1 || msgs[0].Text != "hello" {
		t.Fatalf("unexpected messages: %+v", msgs)
	}
}
