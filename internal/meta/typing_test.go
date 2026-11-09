package meta

import (
	"context"
	"testing"

	"go.mau.fi/mautrix-meta/pkg/messagix/table"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"

	"lazyfmess/internal/store"
)

func TestTypingFromEvent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()
	presence, ok := decodeChatPresence(&events.ChatPresence{
		MessageSource: types.MessageSource{
			Chat:   types.JID{User: "900", Server: types.GroupServer},
			Sender: types.JID{User: "200", Server: types.MessengerServer},
		},
		State: types.ChatPresenceComposing,
	})
	if !ok {
		t.Fatal("decodeChatPresence failed")
	}
	tests := []struct {
		name string
		evt  Event
		want []Typing
	}{
		{"lightspeed", Event{Table: &table.LSTable{LSUpdateTypingIndicator: []*table.LSUpdateTypingIndicator{
			{ThreadKey: 42, SenderId: 7, IsTyping: true},
		}}}, []Typing{{ThreadID: 42, SenderID: 7, Active: true}}},
		{"e2ee", Event{typing: []Typing{presence}}, []Typing{{ThreadID: 900, SenderID: 200, Active: true}}},
		{"none", Event{Type: "connected"}, nil},
	}
	for _, tt := range tests {
		got, err := TypingFromEvent(ctx, s, tt.evt)
		if err != nil {
			t.Fatalf("%s: %v", tt.name, err)
		}
		if len(got) != len(tt.want) || (len(got) > 0 && got[0] != tt.want[0]) {
			t.Errorf("%s: got %+v want %+v", tt.name, got, tt.want)
		}
	}
}
