package meta

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"testing"
	"time"

	"lazyfmess/internal/store"
)

type fakeArchive struct {
	times    []int64
	perPage  int
	calls    []int64
	failAt   int
	failures int
}

func (a *fakeArchive) fetch(_ context.Context, before int64) (historyPage, error) {
	a.calls = append(a.calls, before)
	if a.failAt > 0 && len(a.calls) == a.failAt {
		return historyPage{}, errors.New("archive unavailable")
	}
	var older []int64
	for _, ts := range a.times {
		if ts < before {
			older = append(older, ts)
		}
	}
	sort.Slice(older, func(i, j int) bool { return older[i] > older[j] })
	n := min(a.perPage, len(older))
	page := historyPage{more: len(older) > n, failed: a.failures}
	for _, ts := range older[:n] {
		page.messages = append(page.messages, store.Message{ID: fmt.Sprintf("m%d", ts), ThreadID: 1, SenderID: 2, Text: "x", Timestamp: time.UnixMilli(ts)})
	}
	if n > 0 {
		page.before = older[n-1]
	}
	return page, nil
}

func archive(n int) *fakeArchive {
	a := &fakeArchive{perPage: 2}
	for i := 1; i <= n; i++ {
		a.times = append(a.times, int64(i*1000))
	}
	return a
}

func openStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func messageCount(t *testing.T, s *store.Store) int {
	t.Helper()
	msgs, err := s.ListMessages(context.Background(), 1)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	return len(msgs)
}

func TestSyncHistoryResumesAcrossRuns(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := openStore(t)
	thread := store.Thread{ID: 1, Title: "Dana"}
	a := archive(9)
	res, err := syncHistory(ctx, s, thread, a.fetch, 2, 0)
	if err != nil || res.pages != 2 || messageCount(t, s) != 4 {
		t.Fatalf("first run: res %+v err %v count %d", res, err, messageCount(t, s))
	}
	state, found, _ := s.HistoryState(ctx, 1)
	if !found || state.ReachedStart || state.Before.UnixMilli() != 6000 {
		t.Fatalf("state after first run: %+v found %v", state, found)
	}
	a.calls = nil
	if _, err := syncHistory(ctx, s, thread, a.fetch, 10, 0); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if a.calls[1] != 6000 {
		t.Fatalf("second run should resume from the saved cursor after catch-up, calls %v", a.calls)
	}
	state, _, _ = s.HistoryState(ctx, 1)
	if !state.ReachedStart || messageCount(t, s) != 9 {
		t.Fatalf("after second run: state %+v count %d", state, messageCount(t, s))
	}
	a.calls = nil
	if _, err := syncHistory(ctx, s, thread, a.fetch, 10, 0); err != nil {
		t.Fatalf("third run: %v", err)
	}
	if len(a.calls) != 1 {
		t.Fatalf("finished chat should only check for new messages, calls %v", a.calls)
	}
}

func TestSyncHistoryCatchesUpNewMessages(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := openStore(t)
	thread := store.Thread{ID: 1, Title: "Dana"}
	a := archive(4)
	if _, err := syncHistory(ctx, s, thread, a.fetch, 10, 0); err != nil {
		t.Fatalf("initial: %v", err)
	}
	for i := 5; i <= 10; i++ {
		a.times = append(a.times, int64(i*1000))
	}
	a.calls = nil
	if _, err := syncHistory(ctx, s, thread, a.fetch, 10, 0); err != nil {
		t.Fatalf("catch-up: %v", err)
	}
	if messageCount(t, s) != 10 {
		t.Fatalf("catch-up stored %d messages, want 10", messageCount(t, s))
	}
	if len(a.calls) != 4 {
		t.Fatalf("catch-up should stop once it overlaps stored messages, calls %v", a.calls)
	}
	got, _ := s.GetThread(ctx, 1)
	if got.UpdatedAt.UnixMilli() != 10000 {
		t.Fatalf("thread activity: %d", got.UpdatedAt.UnixMilli())
	}
}

func TestSyncHistoryKeepsProgressOnErrors(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := openStore(t)
	thread := store.Thread{ID: 1, Title: "Dana"}
	a := archive(9)
	a.failAt = 3
	a.failures = 1
	res, err := syncHistory(ctx, s, thread, a.fetch, 10, 0)
	if err == nil || res.pages != 2 || res.failed != 2 {
		t.Fatalf("res %+v err %v", res, err)
	}
	state, found, _ := s.HistoryState(ctx, 1)
	if !found || state.Before.UnixMilli() != 6000 {
		t.Fatalf("progress before the error should be saved: %+v", state)
	}
	if historyError(thread, res, err) == nil {
		t.Fatal("historyError should report the failure")
	}
	if historyError(thread, historyResult{pages: 3}, nil) != nil {
		t.Fatal("historyError should be nil for a clean run")
	}
}
