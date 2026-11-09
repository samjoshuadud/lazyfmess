package store

import (
	"context"
	"fmt"
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

	if err := s.UpsertContact(ctx, 10, "Pat"); err != nil {
		t.Fatalf("UpsertContact: %v", err)
	}
	if err := s.UpsertThread(ctx, Thread{ID: 10, Title: "", ThreadType: 1, UpdatedAt: now}); err != nil {
		t.Fatalf("UpsertThread dm: %v", err)
	}
	threads, err = s.ListThreads(ctx)
	if err != nil {
		t.Fatalf("ListThreads after contact: %v", err)
	}
	found := false
	for _, th := range threads {
		if th.ID == 10 {
			found = true
			if th.Title != "Pat" {
				t.Fatalf("dm title: got %q want Pat", th.Title)
			}
		}
	}
	if !found {
		t.Fatal("missing dm thread")
	}
	if err := s.UpsertMessage(ctx, Message{ID: "m3", ThreadID: 10, SenderID: 10, Text: "yo", Timestamp: now}); err != nil {
		t.Fatalf("UpsertMessage: %v", err)
	}
	msgs, err = s.ListMessages(ctx, 10)
	if err != nil {
		t.Fatalf("ListMessages dm: %v", err)
	}
	if len(msgs) != 1 || msgs[0].SenderName != "Pat" {
		t.Fatalf("unexpected dm messages: %+v", msgs)
	}

	if err := s.UpsertMessage(ctx, Message{
		ID:              "m4",
		ThreadID:        10,
		SenderID:        10,
		Text:            "cap",
		Timestamp:       now.Add(time.Second),
		MediaKind:       "image",
		MediaURL:        "https://example.test/a.jpg",
		MediaMime:       "image/jpeg",
		MediaName:       "a.jpg",
		MediaDurationMs: 0,
	}); err != nil {
		t.Fatalf("UpsertMessage media: %v", err)
	}
	msgs, err = s.ListMessages(ctx, 10)
	if err != nil {
		t.Fatalf("ListMessages media: %v", err)
	}
	if len(msgs) != 2 || msgs[1].MediaKind != "image" || msgs[1].MediaURL == "" {
		t.Fatalf("unexpected media messages: %+v", msgs)
	}
	if err := s.UpsertMessage(ctx, Message{
		ID:        "m4",
		ThreadID:  10,
		SenderID:  10,
		Text:      "cap2",
		Timestamp: now.Add(2 * time.Second),
	}); err != nil {
		t.Fatalf("UpsertMessage preserve media: %v", err)
	}
	msgs, err = s.ListMessages(ctx, 10)
	if err != nil {
		t.Fatalf("ListMessages preserve: %v", err)
	}
	if msgs[1].Text != "cap2" || msgs[1].MediaKind != "image" || msgs[1].MediaURL == "" {
		t.Fatalf("media not preserved: %+v", msgs[1])
	}

	if filepath.Base(DBFileName) != DBFileName {
		t.Fatal("unexpected db file name")
	}
}

func TestReadState(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()
	base := time.UnixMilli(1_700_000_000_000)
	if err := s.UpsertThread(ctx, Thread{ID: 5, Title: "Group", UpdatedAt: base.Add(3 * time.Second)}); err != nil {
		t.Fatalf("UpsertThread: %v", err)
	}
	for i, sender := range []int64{1, 2, 3, 2} {
		msg := Message{ID: fmt.Sprintf("m%d", i), ThreadID: 5, SenderID: sender, Text: "x", Timestamp: base.Add(time.Duration(i) * time.Second)}
		if err := s.UpsertMessage(ctx, msg); err != nil {
			t.Fatalf("UpsertMessage: %v", err)
		}
	}
	if err := s.SetLastRead(ctx, 5, base); err != nil {
		t.Fatalf("SetLastRead: %v", err)
	}
	thread, err := s.GetThread(ctx, 5)
	if err != nil || !thread.Unread() {
		t.Fatalf("thread unread: %+v err=%v", thread, err)
	}
	unread, err := s.UnreadMessageIDs(ctx, 5, 1)
	if err != nil {
		t.Fatalf("UnreadMessageIDs: %v", err)
	}
	if fmt.Sprint(unread) != "map[2:[m1 m3] 3:[m2]]" {
		t.Fatalf("unread ids: %v", unread)
	}
	if err := s.SetLastRead(ctx, 5, base.Add(3*time.Second)); err != nil {
		t.Fatalf("SetLastRead: %v", err)
	}
	if err := s.SetLastRead(ctx, 5, base); err != nil {
		t.Fatalf("SetLastRead backwards: %v", err)
	}
	thread, _ = s.GetThread(ctx, 5)
	if thread.Unread() {
		t.Fatalf("thread should be read: %+v", thread)
	}
}

func TestReplyResolvesFromLocalMessage(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()
	base := time.UnixMilli(1_700_000_000_000)
	if err := s.UpsertContact(ctx, 2, "Dana"); err != nil {
		t.Fatalf("UpsertContact: %v", err)
	}
	msgs := []Message{
		{ID: "orig", ThreadID: 1, SenderID: 2, Text: "lunch?", Timestamp: base},
		{ID: "local", ThreadID: 1, SenderID: 1, Text: "yes", Timestamp: base.Add(time.Second), ReplyToID: "orig"},
		{ID: "server", ThreadID: 1, SenderID: 1, Text: "ok", Timestamp: base.Add(2 * time.Second), ReplyToID: "gone", ReplyText: "old text", ReplySenderID: 2},
	}
	if err := s.EnsureThread(ctx, 1); err != nil {
		t.Fatalf("EnsureThread: %v", err)
	}
	for _, m := range msgs {
		if err := s.UpsertMessage(ctx, m); err != nil {
			t.Fatalf("UpsertMessage: %v", err)
		}
	}
	got, err := s.ListMessages(ctx, 1)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	tests := []struct {
		idx                int
		text, name, target string
	}{
		{1, "lunch?", "Dana", "orig"},
		{2, "old text", "Dana", "gone"},
	}
	for _, tt := range tests {
		m := got[tt.idx]
		if m.ReplyToID != tt.target || m.ReplyText != tt.text || m.ReplySenderName != tt.name {
			t.Errorf("message %s: reply %q %q %q", m.ID, m.ReplyToID, m.ReplyText, m.ReplySenderName)
		}
	}
	if got[0].ReplyToID != "" {
		t.Errorf("original should have no reply: %+v", got[0])
	}
}
