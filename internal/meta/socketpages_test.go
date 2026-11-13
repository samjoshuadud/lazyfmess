package meta

import (
	"testing"

	"go.mau.fi/mautrix-meta/pkg/messagix/table"
)

func TestNotifyRangesDeliversToWaitingChat(t *testing.T) {
	t.Parallel()
	c := &Client{}
	ch, done := c.waitRange(42)
	other, doneOther := c.waitRange(7)
	defer doneOther()
	c.notifyRanges(&table.LSTable{
		LSUpsertMessage: []*table.LSUpsertMessage{
			{MessageId: "mid-2", ThreadKey: 42, SenderId: 9, Text: "older", TimestampMs: 2000},
		},
		LSInsertNewMessageRange: []*table.LSInsertNewMessageRange{
			{ThreadKey: 42, MinTimestampMs: 2000, MinMessageId: "mid-2", HasMoreBefore: true},
		},
		LSUpdateExistingMessageRange: []*table.LSUpdateExistingMessageRange{{ThreadKey: 7, TimestampMS: 1}},
	})
	sig := <-ch
	page := pageFromUpsert(sig.upsert, 42)
	if len(page.messages) != 1 || page.messages[0].ID != "mid-2" || !page.more || page.before != 2000 {
		t.Fatalf("page: %+v", page)
	}
	if end := <-other; !end.ended {
		t.Fatalf("existing range should end the other chat's backfill: %+v", end)
	}
	done()
	c.notifyRanges(&table.LSTable{LSUpdateExistingMessageRange: []*table.LSUpdateExistingMessageRange{{ThreadKey: 42}}})
	select {
	case sig := <-ch:
		t.Fatalf("finished waiter still received %+v", sig)
	default:
	}
}
