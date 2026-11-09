package meta

import (
	"context"
	"testing"

	"go.mau.fi/mautrix-meta/pkg/messagix/table"

	"lazyfmess/internal/store"
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

func TestApplyTableStoresImageAttachment(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	tbl := &table.LSTable{
		LSInsertMessage: []*table.LSInsertMessage{
			{
				MessageId:   "mid-img",
				ThreadKey:   42,
				SenderId:    99,
				Text:        "look",
				TimestampMs: 1_700_000_000_100,
			},
		},
		LSInsertAttachment: []*table.LSInsertAttachment{
			{
				MessageId:           "mid-img",
				Filename:            "pic.jpg",
				PlayableUrl:         "https://example.test/pic.jpg",
				PlayableUrlMimeType: "image/jpeg",
				AttachmentType:      table.AttachmentTypeImage,
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
	if len(msgs) != 1 {
		t.Fatalf("message count: %d", len(msgs))
	}
	if msgs[0].MediaKind != "image" || msgs[0].MediaURL != "https://example.test/pic.jpg" {
		t.Fatalf("unexpected media: %+v", msgs[0])
	}
	if msgs[0].MediaName != "pic.jpg" || msgs[0].MediaMime != "image/jpeg" {
		t.Fatalf("unexpected media meta: %+v", msgs[0])
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

func TestApplyTableReadState(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()
	const activity = 1_700_000_000_000
	tables := []struct {
		name   string
		tbl    *table.LSTable
		unread bool
	}{
		{"unread thread row", &table.LSTable{LSDeleteThenInsertThread: []*table.LSDeleteThenInsertThread{
			{ThreadKey: 7, ThreadName: "Dana", LastActivityTimestampMs: activity, LastReadWatermarkTimestampMs: activity - 5000},
		}}, true},
		{"read elsewhere", &table.LSTable{LSMarkThreadReadV2: []*table.LSMarkThreadReadV2{
			{ThreadKey: 7, LastReadWatermarkTimestampMs: activity},
		}}, false},
	}
	for _, tt := range tables {
		if err := ApplyTable(ctx, s, tt.tbl); err != nil {
			t.Fatalf("%s: ApplyTable: %v", tt.name, err)
		}
		thread, err := s.GetThread(ctx, 7)
		if err != nil {
			t.Fatalf("%s: GetThread: %v", tt.name, err)
		}
		if thread.Unread() != tt.unread {
			t.Fatalf("%s: unread = %v, thread %+v", tt.name, thread.Unread(), thread)
		}
	}
}

func TestApplyTableKeepsReplyFields(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()
	tbl := &table.LSTable{
		LSInsertMessage: []*table.LSInsertMessage{
			{MessageId: "mid-2", ThreadKey: 42, SenderId: 7, Text: "yes", TimestampMs: 1_700_000_000_200, ReplySourceId: "mid-1", ReplyMessageText: "lunch?", ReplyToUserId: 9},
		},
	}
	if err := ApplyTable(ctx, s, tbl); err != nil {
		t.Fatalf("ApplyTable: %v", err)
	}
	msgs, err := s.ListMessages(ctx, 42)
	if err != nil || len(msgs) != 1 {
		t.Fatalf("ListMessages: %+v %v", msgs, err)
	}
	if m := msgs[0]; m.ReplyToID != "mid-1" || m.ReplyText != "lunch?" || m.ReplySenderID != 9 {
		t.Fatalf("reply fields: %+v", m)
	}
}
