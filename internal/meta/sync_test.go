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

func TestApplyTableResolvesDMTitleAndSender(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	defer s.Close()

	tbl := &table.LSTable{
		LSVerifyContactRowExists: []*table.LSVerifyContactRowExists{
			{ContactId: 99, Name: "Dana"},
			{ContactId: 55, Name: "Riley"},
		},
		LSDeleteThenInsertThread: []*table.LSDeleteThenInsertThread{
			{
				ThreadKey:               99,
				ThreadType:              table.ONE_TO_ONE,
				Snippet:                 "hey",
				LastActivityTimestampMs: 1_700_000_000_000,
			},
		},
		LSInsertMessage: []*table.LSInsertMessage{
			{
				MessageId:   "mid-dm",
				ThreadKey:   99,
				SenderId:    55,
				Text:        "hey",
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
	if len(threads) != 1 || threads[0].Title != "Dana" {
		t.Fatalf("unexpected threads: %+v", threads)
	}
	msgs, err := s.ListMessages(ctx, 99)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(msgs) != 1 || msgs[0].SenderName != "Riley" {
		t.Fatalf("unexpected messages: %+v", msgs)
	}
}

func TestApplyTableStoresDeleteThenInsertMessages(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	tbl := &table.LSTable{
		LSDeleteThenInsertMessage: []*table.LSDeleteThenInsertMessage{
			{
				MessageId:   "mid-history",
				ThreadKey:   42,
				SenderId:    99,
				Text:        "history message",
				TimestampMs: 1_700_000_000_100,
			},
		},
	}
	if err := ApplyTable(ctx, s, tbl); err != nil {
		t.Fatalf("ApplyTable: %v", err)
	}
	msgs, err := s.ListMessages(ctx, 42)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(msgs) != 1 || msgs[0].Text != "history message" {
		t.Fatalf("unexpected messages: %+v", msgs)
	}
}
