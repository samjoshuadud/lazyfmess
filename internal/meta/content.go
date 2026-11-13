package meta

import (
	"fmt"
	"strings"
	"time"

	"go.mau.fi/mautrix-meta/pkg/messagix/securehistory"
	"go.mau.fi/mautrix-meta/pkg/metaid"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waArmadilloBackupCommon"
	"go.mau.fi/whatsmeow/proto/waArmadilloBackupMessage"
	"go.mau.fi/whatsmeow/proto/waArmadilloMiTransportAdminMessage"
	"go.mau.fi/whatsmeow/proto/waConsumerApplication"
	"go.mau.fi/whatsmeow/proto/waMsgApplication"
	"google.golang.org/protobuf/proto"

	"lazyfmess/internal/store"
)

type messageContent struct {
	text      string
	mediaKind string
	mediaName string
}

func decodeContent(c *waConsumerApplication.ConsumerApplication_Content) (messageContent, bool) {
	if c == nil {
		return messageContent{}, false
	}
	var out messageContent
	switch {
	case c.GetMessageText() != nil:
		out.text = c.GetMessageText().GetText()
	case c.GetExtendedTextMessage() != nil:
		out.text = c.GetExtendedTextMessage().GetText().GetText()
	case c.GetImageMessage() != nil:
		out.mediaKind = "image"
		out.text = c.GetImageMessage().GetCaption().GetText()
	case c.GetVideoMessage() != nil:
		out.mediaKind = "video"
		out.text = c.GetVideoMessage().GetCaption().GetText()
	case c.GetViewOnceMessage() != nil:
		out.mediaKind = "view-once"
		if c.GetViewOnceMessage().GetVideoMessage() != nil {
			out.mediaKind = "view-once video"
		}
	case c.GetAudioMessage() != nil:
		out.mediaKind = "audio"
		if c.GetAudioMessage().GetPTT() {
			out.mediaKind = "voice message"
		}
	case c.GetStickerMessage() != nil:
		out.mediaKind = "sticker"
	case c.GetDocumentMessage() != nil:
		out.mediaKind = "file"
		out.mediaName = c.GetDocumentMessage().GetFileName()
	case c.GetLocationMessage() != nil:
		out.text = "[location] " + joinNonEmpty(c.GetLocationMessage().GetLocation().GetName(), c.GetLocationMessage().GetAddress())
	case c.GetLiveLocationMessage() != nil:
		out.text = "[live location]"
	case c.GetContactMessage() != nil:
		out.text = "[contact]"
	case c.GetContactsArrayMessage() != nil:
		out.text = "[contacts] " + c.GetContactsArrayMessage().GetDisplayName()
	case c.GetPollCreationMessage() != nil:
		out.text = "[poll] " + c.GetPollCreationMessage().GetName()
	case c.GetGroupInviteMessage() != nil:
		out.text = "[group invite] " + c.GetGroupInviteMessage().GetGroupName()
	default:
		return messageContent{}, false
	}
	out.text = strings.TrimSpace(out.text)
	return out, out.text != "" || out.mediaKind != ""
}

func joinNonEmpty(parts ...string) string {
	var kept []string
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, ", ")
}

func adminText(sub *waArmadilloBackupCommon.Subprotocol) (string, bool) {
	var admin waArmadilloMiTransportAdminMessage.MiTransportAdminMessage
	if sub == nil || proto.Unmarshal(sub.GetPayload(), &admin) != nil {
		return "", false
	}
	switch content := admin.GetContent().(type) {
	case *waArmadilloMiTransportAdminMessage.MiTransportAdminMessage_GroupNameChanged_:
		if name := content.GroupNameChanged.GetGroupName(); name != "" {
			return "named the group " + name, true
		}
		return "changed the group name", true
	case *waArmadilloMiTransportAdminMessage.MiTransportAdminMessage_XmatChangeThreadName_:
		return "changed the group name", true
	case *waArmadilloMiTransportAdminMessage.MiTransportAdminMessage_ChatThemeChanged_,
		*waArmadilloMiTransportAdminMessage.MiTransportAdminMessage_XmatThemeColor_:
		return "changed the chat theme", true
	case *waArmadilloMiTransportAdminMessage.MiTransportAdminMessage_NicknameChanged_,
		*waArmadilloMiTransportAdminMessage.MiTransportAdminMessage_XmatThreadNickname_:
		return "changed a nickname", true
	case *waArmadilloMiTransportAdminMessage.MiTransportAdminMessage_GroupParticipantChanged_,
		*waArmadilloMiTransportAdminMessage.MiTransportAdminMessage_XmatAddParticipants_,
		*waArmadilloMiTransportAdminMessage.MiTransportAdminMessage_XmatRemoveParticipant_:
		return "changed the group members", true
	case *waArmadilloMiTransportAdminMessage.MiTransportAdminMessage_GroupAdminChanged_,
		*waArmadilloMiTransportAdminMessage.MiTransportAdminMessage_XmatChangeThreadAdmins_:
		return "changed the group admins", true
	case *waArmadilloMiTransportAdminMessage.MiTransportAdminMessage_GroupImageChanged_,
		*waArmadilloMiTransportAdminMessage.MiTransportAdminMessage_IconChanged_,
		*waArmadilloMiTransportAdminMessage.MiTransportAdminMessage_XmatThreadIcon_:
		return "changed the chat photo", true
	case *waArmadilloMiTransportAdminMessage.MiTransportAdminMessage_MessagePinned_,
		*waArmadilloMiTransportAdminMessage.MiTransportAdminMessage_XmatPinMessageV2_:
		return "pinned a message", true
	case *waArmadilloMiTransportAdminMessage.MiTransportAdminMessage_XmatUnpinMessageV2_:
		return "unpinned a message", true
	case *waArmadilloMiTransportAdminMessage.MiTransportAdminMessage_QuickReactionChanged_,
		*waArmadilloMiTransportAdminMessage.MiTransportAdminMessage_XmatThreadQuickReaction_:
		return "changed the quick reaction", true
	case *waArmadilloMiTransportAdminMessage.MiTransportAdminMessage_DisappearingSettingChanged_,
		*waArmadilloMiTransportAdminMessage.MiTransportAdminMessage_XmatDisappearingSetting_:
		return "changed disappearing messages", true
	case *waArmadilloMiTransportAdminMessage.MiTransportAdminMessage_XmatGroupThreadCreated_:
		return "created the group", true
	}
	return "", false
}

func backupContent(message *waArmadilloBackupMessage.BackupMessage) (*waConsumerApplication.ConsumerApplication_Content, bool) {
	payload, ok := message.GetPayload().(*waArmadilloBackupMessage.BackupMessage_EncryptedTransportMessage)
	if !ok {
		return nil, false
	}
	var application waMsgApplication.MessageApplication
	if err := proto.Unmarshal(payload.EncryptedTransportMessage, &application); err != nil {
		return nil, false
	}
	decoded, err := whatsmeow.DecodeFBApplication(&application)
	if err != nil {
		return nil, false
	}
	return decoded.GetConsumerApplication().GetPayload().GetContent(), true
}

func backupSender(message *waArmadilloBackupMessage.BackupMessage) (int64, bool) {
	meta := message.GetMetadata()
	if meta.GetSenderID() == "" {
		return 0, false
	}
	id, err := metaid.ParseIDFromString(meta.GetSenderID())
	return id, err == nil
}

type archivedMessage struct {
	message   store.Message
	reactions []store.Reaction
}

func decodeArchived(d *securehistory.DecryptedMessage, threadID int64) (archivedMessage, bool) {
	if d == nil || d.Message == nil || d.Message.Message == nil {
		return archivedMessage{}, false
	}
	parent := d.Message.Message
	senderID, ok := backupSender(parent)
	id := parent.GetMetadata().GetMessageID()
	if !ok || id == "" {
		return archivedMessage{}, false
	}
	msg := store.Message{ID: id, ThreadID: threadID, SenderID: senderID, Timestamp: time.UnixMilli(d.Message.TimestampMS)}
	if content, ok := backupContent(parent); ok {
		decoded, ok := decodeContent(content)
		if !ok {
			return archivedMessage{}, false
		}
		msg.Text, msg.MediaKind, msg.MediaName = decoded.text, decoded.mediaKind, decoded.mediaName
	} else if text, ok := adminText(parent.GetMiTransportAdminMessage()); ok {
		msg.Text, msg.System = text, true
	} else {
		return archivedMessage{}, false
	}
	out := archivedMessage{message: msg}
	latest := make(map[int64]int64)
	editAt := int64(-1)
	for _, sup := range d.Supplementals {
		if sup == nil || sup.Message == nil {
			continue
		}
		content, ok := backupContent(sup.Message)
		actor, hasActor := backupSender(sup.Message)
		if !ok || !hasActor {
			continue
		}
		at := sup.TimestampMS
		if reaction := content.GetReactionMessage(); reaction != nil {
			if prev, seen := latest[actor]; seen && prev > at {
				continue
			}
			latest[actor] = at
			out.reactions = upsertReaction(out.reactions, store.Reaction{MessageID: id, ActorID: actor, Emoji: reaction.GetText()})
		} else if edit := content.GetEditMessage(); edit != nil && actor == senderID && at > editAt {
			editAt = at
			out.message.Text = edit.GetMessage().GetText()
			out.message.Edited = true
		}
	}
	return out, true
}

func upsertReaction(list []store.Reaction, r store.Reaction) []store.Reaction {
	for i := range list {
		if list[i].ActorID == r.ActorID {
			list[i] = r
			return list
		}
	}
	return append(list, r)
}

func previewOf(m store.Message) string {
	switch {
	case m.Text != "":
		return m.Text
	case m.MediaName != "":
		return fmt.Sprintf("[%s %s]", m.MediaKind, m.MediaName)
	case m.MediaKind != "":
		return "[" + m.MediaKind + "]"
	default:
		return ""
	}
}
