package store

import (
	"context"
	"testing"
	"time"
)

func TestHistoryState(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()
	for _, th := range []Thread{
		{ID: 1, Title: "old", UpdatedAt: time.UnixMilli(1_000)},
		{ID: 2, Title: "new", UpdatedAt: time.UnixMilli(9_000)},
		{ID: 3, Title: "done", UpdatedAt: time.UnixMilli(5_000)},
	} {
		if err := s.UpsertThread(ctx, th); err != nil {
			t.Fatalf("UpsertThread: %v", err)
		}
	}
	if _, found, err := s.HistoryState(ctx, 1); err != nil || found {
		t.Fatalf("unset state: found %v err %v", found, err)
	}
	steps := []struct {
		thread int64
		state  HistoryState
	}{
		{1, HistoryState{Before: time.UnixMilli(500)}},
		{3, HistoryState{Before: time.UnixMilli(100), ReachedStart: true}},
		{1, HistoryState{Before: time.UnixMilli(200)}},
	}
	for _, step := range steps {
		if err := s.SetHistoryState(ctx, step.thread, step.state); err != nil {
			t.Fatalf("SetHistoryState: %v", err)
		}
		got, found, err := s.HistoryState(ctx, step.thread)
		if err != nil || !found || !got.Before.Equal(step.state.Before) || got.ReachedStart != step.state.ReachedStart {
			t.Fatalf("state %d: %+v found %v err %v", step.thread, got, found, err)
		}
	}
	pending, err := s.PendingHistoryThreads(ctx)
	if err != nil {
		t.Fatalf("PendingHistoryThreads: %v", err)
	}
	if len(pending) != 2 || pending[0].ID != 2 || pending[1].ID != 1 {
		t.Fatalf("pending: %+v", pending)
	}
}

func TestMessageTimeRange(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()
	if _, _, ok, err := s.MessageTimeRange(ctx, 1); err != nil || ok {
		t.Fatalf("empty thread: ok %v err %v", ok, err)
	}
	for i, ms := range []int64{300, 100, 200} {
		if err := s.UpsertMessage(ctx, Message{ID: string(rune('a' + i)), ThreadID: 1, Timestamp: time.UnixMilli(ms)}); err != nil {
			t.Fatalf("UpsertMessage: %v", err)
		}
	}
	if err := s.UpsertMessage(ctx, Message{ID: "p", ThreadID: 1, Timestamp: time.UnixMilli(900), Pending: true}); err != nil {
		t.Fatalf("UpsertMessage: %v", err)
	}
	oldest, newest, ok, err := s.MessageTimeRange(ctx, 1)
	if err != nil || !ok || oldest.UnixMilli() != 100 || newest.UnixMilli() != 300 {
		t.Fatalf("range: %v %v ok %v err %v", oldest.UnixMilli(), newest.UnixMilli(), ok, err)
	}
}
