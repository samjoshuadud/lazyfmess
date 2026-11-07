package meta

import (
	"testing"

	"go.mau.fi/mautrix-meta/pkg/messagix/table"
)

func TestExtractSendResult(t *testing.T) {
	t.Parallel()
	otid := int64(42)
	resp := &table.LSTable{
		LSReplaceOptimsiticMessage: []*table.LSReplaceOptimsiticMessage{
			{OfflineThreadingId: "42", MessageId: "mid.$cAAAAAQ"},
		},
	}
	id, _ := extractSendResult(resp, otid)
	if id != "mid.$cAAAAAQ" {
		t.Fatalf("id: %q", id)
	}
	if id, _ := extractSendResult(resp, 99); id != "" {
		t.Fatalf("unexpected id for other otid: %q", id)
	}
}

func TestTrimMessage(t *testing.T) {
	t.Parallel()
	if got := trimMessage("  hi\n"); got != "hi" {
		t.Fatalf("got %q", got)
	}
	if got := trimMessage("   "); got != "" {
		t.Fatalf("got %q", got)
	}
}
