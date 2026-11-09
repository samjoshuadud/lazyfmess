package meta

import (
	"context"
	"testing"
	"time"

	"go.mau.fi/mautrix-meta/pkg/messagix/table"
	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waConsumerApplication"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"

	"lazyfmess/internal/store"
)

func consumerTextMessage(text string) *waConsumerApplication.ConsumerApplication {
	return &waConsumerApplication.ConsumerApplication{
		Payload: &waConsumerApplication.ConsumerApplication_Payload{
			Payload: &waConsumerApplication.ConsumerApplication_Payload_Content{
				Content: &waConsumerApplication.ConsumerApplication_Content{
					Content: &waConsumerApplication.ConsumerApplication_Content_MessageText{
						MessageText: &waCommon.MessageText{Text: proto.String(text)},
					},
				},
			},
		},
	}
}

func fbMessage(chat types.JID, sender string, content *waConsumerApplication.ConsumerApplication) *events.FBMessage {
	return &events.FBMessage{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{Chat: chat, Sender: types.JID{User: sender, Server: types.MessengerServer}},
			ID:            "wa-1",
			Timestamp:     time.UnixMilli(1_700_000_000_000),
		},
		Message: content,
	}
}

func TestDecodeLiveMessage(t *testing.T) {
	t.Parallel()
	dm := types.JID{User: "100", Server: types.MessengerServer}
	group := types.JID{User: "900", Server: types.GroupServer}
	tests := []struct {
		name       string
		evt        *events.FBMessage
		ok         bool
		threadID   int64
		threadType table.ThreadType
	}{
		{"direct text", fbMessage(dm, "100", consumerTextMessage("hi")), true, 100, table.ENCRYPTED_OVER_WA_ONE_TO_ONE},
		{"group text", fbMessage(group, "200", consumerTextMessage("hey all")), true, 900, table.ENCRYPTED_OVER_WA_GROUP},
		{"empty text", fbMessage(dm, "100", consumerTextMessage("")), false, 0, 0},
		{"bad sender", fbMessage(dm, "not-a-number", consumerTextMessage("hi")), false, 0, 0},
	}
	for _, tt := range tests {
		got, ok := decodeLiveMessage(tt.evt)
		if ok != tt.ok {
			t.Fatalf("%s: ok = %v", tt.name, ok)
		}
		if !ok {
			continue
		}
		if got.message.ThreadID != tt.threadID || got.threadType != tt.threadType || got.message.ID != "wa-1" {
			t.Errorf("%s: got %+v", tt.name, got)
		}
	}
}

func TestApplyLiveMessageUpdatesThread(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()
	if err := s.UpsertThread(ctx, store.Thread{ID: 100, Title: "Dana"}); err != nil {
		t.Fatalf("UpsertThread: %v", err)
	}
	live, ok := decodeLiveMessage(fbMessage(types.JID{User: "100", Server: types.MessengerServer}, "100", consumerTextMessage("hi")))
	if !ok {
		t.Fatal("decode failed")
	}
	changed, err := ApplyEvent(ctx, s, Event{Type: "e2ee_message", live: &live})
	if err != nil || !changed {
		t.Fatalf("ApplyEvent: changed=%v err=%v", changed, err)
	}
	thread, err := s.GetThread(ctx, 100)
	if err != nil {
		t.Fatalf("GetThread: %v", err)
	}
	if thread.Title != "Dana" || thread.LastPreview != "hi" || thread.ThreadType != int64(table.ENCRYPTED_OVER_WA_ONE_TO_ONE) {
		t.Fatalf("thread: %+v", thread)
	}
	msgs, err := s.ListMessages(ctx, 100)
	if err != nil || len(msgs) != 1 || msgs[0].Text != "hi" {
		t.Fatalf("messages: %+v err=%v", msgs, err)
	}
}

func TestReactionsFromTableAndLiveEvent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()
	if err := s.EnsureThread(ctx, 100); err != nil {
		t.Fatalf("EnsureThread: %v", err)
	}
	if err := s.UpsertMessage(ctx, store.Message{ID: "target", ThreadID: 100, SenderID: 1, Text: "hi", Timestamp: time.UnixMilli(1)}); err != nil {
		t.Fatalf("UpsertMessage: %v", err)
	}
	if err := ApplyTable(ctx, s, &table.LSTable{LSUpsertReaction: []*table.LSUpsertReaction{
		{ThreadKey: 100, MessageId: "target", ActorId: 7, Reaction: "👍"},
	}}); err != nil {
		t.Fatalf("ApplyTable: %v", err)
	}
	live := consumerContent(&waConsumerApplication.ConsumerApplication_Content{
		Content: &waConsumerApplication.ConsumerApplication_Content_ReactionMessage{
			ReactionMessage: &waConsumerApplication.ConsumerApplication_ReactionMessage{
				Key:  &waCommon.MessageKey{ID: proto.String("target")},
				Text: proto.String("❤"),
			},
		},
	})
	evt := fbMessage(types.JID{User: "100", Server: types.MessengerServer}, "100", live)
	if _, ok := decodeLiveMessage(evt); ok {
		t.Fatal("reaction must not decode as a text message")
	}
	reaction, ok := decodeLiveReaction(evt)
	if !ok {
		t.Fatal("decodeLiveReaction failed")
	}
	if _, err := ApplyEvent(ctx, s, Event{reaction: &reaction}); err != nil {
		t.Fatalf("ApplyEvent: %v", err)
	}
	msgs, err := s.ListMessages(ctx, 100)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if got := msgs[0].Reactions; len(got) != 2 || got[0].Emoji != "👍" || got[1].Emoji != "❤" || got[1].ActorID != 100 {
		t.Fatalf("reactions: %+v", got)
	}
}
