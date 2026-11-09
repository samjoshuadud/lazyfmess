package meta

import (
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"go.mau.fi/mautrix-meta/pkg/messagix/httpclient"
	"go.mau.fi/mautrix-meta/pkg/messagix/methods"
	"go.mau.fi/mautrix-meta/pkg/messagix/socket"
	"go.mau.fi/mautrix-meta/pkg/messagix/table"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waConsumerApplication"
	"go.mau.fi/whatsmeow/proto/waMediaTransport"
	"google.golang.org/protobuf/proto"

	"lazyfmess/internal/store"
)

const maxAttachmentBytes = 25 << 20

type attachment struct {
	name   string
	mime   string
	kind   string
	data   []byte
	width  int
	height int
}

func AttachmentKind(path string) string {
	return mediaKindForMime(mime.TypeByExtension(strings.ToLower(filepath.Ext(path))))
}

func mediaKindForMime(mimeType string) string {
	switch {
	case mimeType == "image/gif":
		return "gif"
	case strings.HasPrefix(mimeType, "image/"):
		return "image"
	case strings.HasPrefix(mimeType, "video/"):
		return "video"
	default:
		return "file"
	}
}

func readAttachment(path string) (attachment, error) {
	info, err := os.Stat(path)
	if err != nil {
		return attachment{}, fmt.Errorf("read attachment: %w", err)
	}
	if info.IsDir() {
		return attachment{}, fmt.Errorf("read attachment: %s is a directory", path)
	}
	if info.Size() > maxAttachmentBytes {
		return attachment{}, fmt.Errorf("read attachment: %s is larger than 25 MB", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return attachment{}, fmt.Errorf("read attachment: %w", err)
	}
	mimeType := mime.TypeByExtension(strings.ToLower(filepath.Ext(path)))
	if mimeType == "" {
		mimeType = http.DetectContentType(data)
	}
	mimeType, _, _ = strings.Cut(mimeType, ";")
	a := attachment{name: filepath.Base(path), mime: mimeType, kind: mediaKindForMime(mimeType), data: data}
	if a.kind == "image" || a.kind == "gif" {
		if cfg, _, err := image.DecodeConfig(bytes.NewReader(data)); err == nil {
			a.width, a.height = cfg.Width, cfg.Height
		}
	}
	return a, nil
}

func (c *Client) SendFile(ctx context.Context, thread store.Thread, path string) (SendResult, error) {
	if err := c.waitReady(ctx); err != nil {
		return SendResult{}, err
	}
	a, err := readAttachment(path)
	if err != nil {
		return SendResult{}, err
	}
	if table.ThreadType(thread.ThreadType).IsWhatsApp() {
		return c.sendE2EEFile(ctx, thread, a)
	}
	resp, err := c.mx.GetHTTP().SendMercuryUploadRequest(ctx, thread.ID, &httpclient.MercuryUploadMedia{
		Filename:  a.name,
		MimeType:  a.mime,
		MediaData: a.data,
	})
	if err != nil {
		return SendResult{}, fmt.Errorf("upload attachment: %w", err)
	}
	if resp.Payload.RealMetadata == nil || resp.Payload.RealMetadata.GetFbId() == 0 {
		return SendResult{}, fmt.Errorf("upload attachment: no attachment id in response")
	}
	return c.sendLightspeed(ctx, &socket.SendMessageTask{
		ThreadId:        thread.ID,
		SendType:        table.MEDIA,
		AttachmentFBIds: []int64{resp.Payload.RealMetadata.GetFbId()},
	})
}

func (c *Client) sendE2EEFile(ctx context.Context, thread store.Thread, a attachment) (SendResult, error) {
	if c.e2ee == nil {
		return SendResult{}, fmt.Errorf("e2ee client is not connected")
	}
	mediaType := whatsmeow.MediaDocument
	switch a.kind {
	case "image":
		mediaType = whatsmeow.MediaImage
	case "video", "gif":
		mediaType = whatsmeow.MediaVideo
	}
	uploaded, err := c.e2ee.Upload(ctx, a.data, mediaType)
	if err != nil {
		return SendResult{}, fmt.Errorf("upload e2ee attachment: %w", err)
	}
	thumbW, thumbH := thumbnailSize(a.width, a.height)
	if thumbW == 0 && a.kind == "image" {
		thumbW, thumbH = 400, 400
	}
	transport := &waMediaTransport.WAMediaTransport{
		Integral: &waMediaTransport.WAMediaTransport_Integral{
			FileSHA256:        uploaded.FileSHA256,
			MediaKey:          uploaded.MediaKey,
			FileEncSHA256:     uploaded.FileEncSHA256,
			DirectPath:        proto.String(uploaded.DirectPath),
			MediaKeyTimestamp: proto.Int64(time.Now().Unix()),
		},
		Ancillary: &waMediaTransport.WAMediaTransport_Ancillary{
			FileLength: proto.Uint64(uint64(len(a.data))),
			Mimetype:   proto.String(a.mime),
			Thumbnail: &waMediaTransport.WAMediaTransport_Ancillary_Thumbnail{
				ThumbnailWidth:  proto.Uint32(uint32(thumbW)),
				ThumbnailHeight: proto.Uint32(uint32(thumbH)),
			},
			ObjectID: proto.String(uploaded.ObjectID),
		},
	}
	content, err := wrapMedia(a, transport)
	if err != nil {
		return SendResult{}, err
	}
	otid := methods.GenerateEpochID()
	msgID := strconv.FormatInt(otid, 10)
	resp, err := c.e2ee.SendFBMessage(ctx, chatJID(thread), consumerContent(content), nil, whatsmeow.SendRequestExtra{ID: msgID})
	if err != nil {
		return SendResult{}, fmt.Errorf("send e2ee attachment: %w", err)
	}
	ts := resp.Timestamp
	if ts.IsZero() {
		ts = time.Now()
	}
	return SendResult{MessageID: msgID, Timestamp: ts}, nil
}

func wrapMedia(a attachment, transport *waMediaTransport.WAMediaTransport) (*waConsumerApplication.ConsumerApplication_Content, error) {
	size := func() (*uint32, *uint32) { return proto.Uint32(uint32(a.height)), proto.Uint32(uint32(a.width)) }
	switch a.kind {
	case "image":
		msg := &waConsumerApplication.ConsumerApplication_ImageMessage{}
		h, w := size()
		if err := msg.Set(&waMediaTransport.ImageTransport{
			Integral:  &waMediaTransport.ImageTransport_Integral{Transport: transport},
			Ancillary: &waMediaTransport.ImageTransport_Ancillary{Height: h, Width: w},
		}); err != nil {
			return nil, fmt.Errorf("wrap image: %w", err)
		}
		return &waConsumerApplication.ConsumerApplication_Content{Content: &waConsumerApplication.ConsumerApplication_Content_ImageMessage{ImageMessage: msg}}, nil
	case "video", "gif":
		msg := &waConsumerApplication.ConsumerApplication_VideoMessage{}
		h, w := size()
		ancillary := &waMediaTransport.VideoTransport_Ancillary{Height: h, Width: w}
		if a.kind == "gif" {
			ancillary.GifPlayback = proto.Bool(true)
		}
		if err := msg.Set(&waMediaTransport.VideoTransport{
			Integral:  &waMediaTransport.VideoTransport_Integral{Transport: transport},
			Ancillary: ancillary,
		}); err != nil {
			return nil, fmt.Errorf("wrap video: %w", err)
		}
		return &waConsumerApplication.ConsumerApplication_Content{Content: &waConsumerApplication.ConsumerApplication_Content_VideoMessage{VideoMessage: msg}}, nil
	default:
		msg := &waConsumerApplication.ConsumerApplication_DocumentMessage{FileName: proto.String(a.name)}
		if err := msg.Set(&waMediaTransport.DocumentTransport{
			Integral:  &waMediaTransport.DocumentTransport_Integral{Transport: transport},
			Ancillary: &waMediaTransport.DocumentTransport_Ancillary{},
		}); err != nil {
			return nil, fmt.Errorf("wrap document: %w", err)
		}
		return &waConsumerApplication.ConsumerApplication_Content{Content: &waConsumerApplication.ConsumerApplication_Content_DocumentMessage{DocumentMessage: msg}}, nil
	}
}

func thumbnailSize(w, h int) (int, int) {
	if w <= 0 || h <= 0 {
		return 0, 0
	}
	scale := min(1, 400/float64(max(w, h)))
	return int(float64(w) * scale), int(float64(h) * scale)
}
