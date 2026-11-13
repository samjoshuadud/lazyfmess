package meta

import (
	"testing"

	"go.mau.fi/mautrix-meta/pkg/messagix/securehistory"
	"go.mau.fi/whatsmeow/proto/waArmadilloBackupCommon"
	"go.mau.fi/whatsmeow/proto/waArmadilloBackupMessage"
	"go.mau.fi/whatsmeow/proto/waArmadilloMiTransportAdminMessage"
	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waConsumerApplication"
	"go.mau.fi/whatsmeow/proto/waMsgApplication"
	"google.golang.org/protobuf/proto"
)

func backupOf(t *testing.T, id, sender string, content *waConsumerApplication.ConsumerApplication_Content) *waArmadilloBackupMessage.BackupMessage {
	t.Helper()
	consumer, err := proto.Marshal(consumerContent(content))
	if err != nil {
		t.Fatal(err)
	}
	app, err := proto.Marshal(&waMsgApplication.MessageApplication{
		Payload: &waMsgApplication.MessageApplication_Payload{
			Content: &waMsgApplication.MessageApplication_Payload_SubProtocol{
				SubProtocol: &waMsgApplication.MessageApplication_SubProtocolPayload{
					SubProtocol: &waMsgApplication.MessageApplication_SubProtocolPayload_ConsumerMessage{
						ConsumerMessage: &waCommon.SubProtocol{Payload: consumer, Version: proto.Int32(1)},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return &waArmadilloBackupMessage.BackupMessage{
		Metadata: &waArmadilloBackupCommon.Metadata{SenderID: proto.String(sender), MessageID: proto.String(id)},
		Payload:  &waArmadilloBackupMessage.BackupMessage_EncryptedTransportMessage{EncryptedTransportMessage: app},
	}
}

func TestDecodeContentTypes(t *testing.T) {
	t.Parallel()
	text := func(s string) *waCommon.MessageText { return &waCommon.MessageText{Text: proto.String(s)} }
	tests := []struct {
		name    string
		content *waConsumerApplication.ConsumerApplication_Content
		want    messageContent
		ok      bool
	}{
		{"text", &waConsumerApplication.ConsumerApplication_Content{Content: &waConsumerApplication.ConsumerApplication_Content_MessageText{MessageText: text("hi")}}, messageContent{text: "hi"}, true},
		{"image with caption", &waConsumerApplication.ConsumerApplication_Content{Content: &waConsumerApplication.ConsumerApplication_Content_ImageMessage{ImageMessage: &waConsumerApplication.ConsumerApplication_ImageMessage{Caption: text("sunset")}}}, messageContent{text: "sunset", mediaKind: "image"}, true},
		{"document", &waConsumerApplication.ConsumerApplication_Content{Content: &waConsumerApplication.ConsumerApplication_Content_DocumentMessage{DocumentMessage: &waConsumerApplication.ConsumerApplication_DocumentMessage{FileName: proto.String("report.pdf")}}}, messageContent{mediaKind: "file", mediaName: "report.pdf"}, true},
		{"voice", &waConsumerApplication.ConsumerApplication_Content{Content: &waConsumerApplication.ConsumerApplication_Content_AudioMessage{AudioMessage: &waConsumerApplication.ConsumerApplication_AudioMessage{PTT: proto.Bool(true)}}}, messageContent{mediaKind: "voice message"}, true},
		{"location", &waConsumerApplication.ConsumerApplication_Content{Content: &waConsumerApplication.ConsumerApplication_Content_LocationMessage{LocationMessage: &waConsumerApplication.ConsumerApplication_LocationMessage{Location: &waConsumerApplication.ConsumerApplication_Location{Name: proto.String("Cafe")}, Address: proto.String("Main St")}}}, messageContent{text: "[location] Cafe, Main St"}, true},
		{"poll", &waConsumerApplication.ConsumerApplication_Content{Content: &waConsumerApplication.ConsumerApplication_Content_PollCreationMessage{PollCreationMessage: &waConsumerApplication.ConsumerApplication_PollCreationMessage{Name: proto.String("Lunch?")}}}, messageContent{text: "[poll] Lunch?"}, true},
		{"empty text", &waConsumerApplication.ConsumerApplication_Content{Content: &waConsumerApplication.ConsumerApplication_Content_MessageText{MessageText: text("")}}, messageContent{}, false},
		{"reaction is not a message", &waConsumerApplication.ConsumerApplication_Content{Content: &waConsumerApplication.ConsumerApplication_Content_ReactionMessage{ReactionMessage: &waConsumerApplication.ConsumerApplication_ReactionMessage{Text: proto.String("👍")}}}, messageContent{}, false},
	}
	for _, tt := range tests {
		got, ok := decodeContent(tt.content)
		if ok != tt.ok || got != tt.want {
			t.Errorf("%s: got %+v ok %v", tt.name, got, ok)
		}
	}
}

func TestDecodeArchivedAppliesSupplementals(t *testing.T) {
	t.Parallel()
	parent := backupOf(t, "m1", "100", &waConsumerApplication.ConsumerApplication_Content{
		Content: &waConsumerApplication.ConsumerApplication_Content_MessageText{MessageText: &waCommon.MessageText{Text: proto.String("helo")}},
	})
	edit := backupOf(t, "e1", "100", &waConsumerApplication.ConsumerApplication_Content{
		Content: &waConsumerApplication.ConsumerApplication_Content_EditMessage{EditMessage: &waConsumerApplication.ConsumerApplication_EditMessage{
			Key: &waCommon.MessageKey{ID: proto.String("m1")}, Message: &waCommon.MessageText{Text: proto.String("hello")},
		}},
	})
	reaction := func(id, actor, emoji string) *waArmadilloBackupMessage.BackupMessage {
		return backupOf(t, id, actor, &waConsumerApplication.ConsumerApplication_Content{
			Content: &waConsumerApplication.ConsumerApplication_Content_ReactionMessage{ReactionMessage: &waConsumerApplication.ConsumerApplication_ReactionMessage{
				Key: &waCommon.MessageKey{ID: proto.String("m1")}, Text: proto.String(emoji),
			}},
		})
	}
	d := &securehistory.DecryptedMessage{
		Message: &securehistory.DecryptedProtobuf{Message: parent, TimestampMS: 1000},
		Supplementals: map[string]*securehistory.DecryptedProtobuf{
			"e1": {Message: edit, TimestampMS: 2000},
			"r1": {Message: reaction("r1", "200", "😆"), TimestampMS: 3000},
			"r2": {Message: reaction("r2", "200", "❤"), TimestampMS: 4000},
			"r3": {Message: reaction("r3", "300", "👍"), TimestampMS: 3500},
		},
	}
	got, ok := decodeArchived(d, 42)
	if !ok {
		t.Fatal("decodeArchived failed")
	}
	if got.message.Text != "hello" || !got.message.Edited || got.message.ThreadID != 42 || got.message.SenderID != 100 {
		t.Fatalf("message: %+v", got.message)
	}
	emojis := map[int64]string{}
	for _, r := range got.reactions {
		emojis[r.ActorID] = r.Emoji
	}
	if emojis[200] != "❤" || emojis[300] != "👍" || len(emojis) != 2 {
		t.Fatalf("reactions: %+v", got.reactions)
	}
}

func TestDecodeArchivedAdminMessage(t *testing.T) {
	t.Parallel()
	payload, err := proto.Marshal(&waArmadilloMiTransportAdminMessage.MiTransportAdminMessage{
		Content: &waArmadilloMiTransportAdminMessage.MiTransportAdminMessage_GroupNameChanged_{
			GroupNameChanged: &waArmadilloMiTransportAdminMessage.MiTransportAdminMessage_GroupNameChanged{GroupName: proto.String("Hikers")},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	admin := &waArmadilloBackupMessage.BackupMessage{
		Metadata: &waArmadilloBackupCommon.Metadata{SenderID: proto.String("100"), MessageID: proto.String("a1")},
		Payload: &waArmadilloBackupMessage.BackupMessage_MiTransportAdminMessage{
			MiTransportAdminMessage: &waArmadilloBackupCommon.Subprotocol{Payload: payload},
		},
	}
	got, ok := decodeArchived(&securehistory.DecryptedMessage{Message: &securehistory.DecryptedProtobuf{Message: admin, TimestampMS: 5}}, 9)
	if !ok || !got.message.System || got.message.Text != "named the group Hikers" {
		t.Fatalf("admin: %+v ok %v", got.message, ok)
	}
}
