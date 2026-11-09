package meta

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"go.mau.fi/mautrix-meta/pkg/messagix/table"
	"go.mau.fi/mautrix-meta/pkg/messagix/useragent"

	"lazyfmess/internal/store"
)

func mediaFromWrapped(msg *table.WrappedMessage) (kind, url, mime, name string, durationMs int64) {
	if msg == nil {
		return "", "", "", "", 0
	}
	for _, att := range msg.Attachments {
		kind, url, mime, name, durationMs = mediaFromAttachment(
			att.AttachmentType,
			att.PlayableUrl, att.PlayableUrlFallback, att.ImageUrl, att.PreviewUrl,
			att.PlayableUrlMimeType, att.AttachmentMimeType, att.ImageUrlMimeType, att.PreviewUrlMimeType,
			att.Filename, att.PlayableDurationMs,
		)
		if kind != "" && url != "" {
			return kind, url, mime, name, durationMs
		}
	}
	for _, att := range msg.BlobAttachments {
		kind, url, mime, name, durationMs = mediaFromAttachment(
			att.AttachmentType,
			att.PlayableUrl, att.PlayableUrlFallback, "", att.PreviewUrl,
			att.PlayableUrlMimeType, att.AttachmentMimeType, "", att.PreviewUrlMimeType,
			att.Filename, att.PlayableDurationMs,
		)
		if kind != "" && url != "" {
			return kind, url, mime, name, durationMs
		}
	}
	for _, att := range msg.Stickers {
		url = firstNonEmpty(att.PlayableUrl, att.PlayableUrlFallback, att.ImageUrl, att.PreviewUrl)
		if url == "" {
			continue
		}
		mime = firstNonEmpty(att.PlayableUrlMimeType, att.ImageUrlMimeType, att.PreviewUrlMimeType, "image/webp")
		name = filepath.Base(att.AccessibilitySummaryText)
		if name == "" || name == "." {
			name = "sticker"
		}
		return "image", url, mime, name, 0
	}
	return "", "", "", "", 0
}

func mediaFromAttachment(
	attType table.AttachmentType,
	playable, playableFallback, imageURL, preview string,
	playableMime, attachmentMime, imageMime, previewMime string,
	filename string,
	durationMs int64,
) (kind, url, mime, name string, outDuration int64) {
	kind = mediaKind(attType)
	if kind == "" {
		return "", "", "", "", 0
	}
	url = firstNonEmpty(playable, playableFallback, imageURL, preview)
	if url == "" {
		return "", "", "", "", 0
	}
	mime = firstNonEmpty(playableMime, attachmentMime, imageMime, previewMime)
	name = strings.TrimSpace(filename)
	if name == "" {
		name = kind
	}
	return kind, url, mime, name, durationMs
}

func mediaKind(attType table.AttachmentType) string {
	switch attType {
	case table.AttachmentTypeImage, table.AttachmentTypeAnimatedImage,
		table.AttachmentTypeEphemeralImage, table.AttachmentTypeSticker,
		table.AttachmentTypeSelfieSticker, table.AttachmentTypeThirdPartySticker:
		return "image"
	case table.AttachmentTypeVideo, table.AttachmentTypeEphemeralVideo:
		return "video"
	default:
		return ""
	}
}

func (c *Client) OpenMedia(ctx context.Context, msg store.Message) error {
	if err := c.waitReady(ctx); err != nil {
		return err
	}
	if c != nil {
		c.purgeLegacyMediaCache()
	}
	if msg.MediaURL == "" || msg.MediaKind == "" {
		return fmt.Errorf("open media: message has no media")
	}
	path, err := downloadMediaTemp(ctx, msg)
	if err != nil {
		return err
	}
	if err := openLocalFile(path); err != nil {
		_ = os.Remove(path)
		return fmt.Errorf("open media: %w", err)
	}
	c.trackTempMedia(path)
	go c.removeTempMediaWhenIdle(path)
	return nil
}

func downloadMediaTemp(ctx context.Context, msg store.Message) (string, error) {
	name := sanitizeMediaName(msg.MediaName, msg.MediaKind, msg.MediaMime)
	f, err := os.CreateTemp("", "lazyfmess-*-"+name)
	if err != nil {
		return "", fmt.Errorf("create temp media: %w", err)
	}
	path := f.Name()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, msg.MediaURL, nil)
	if err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return "", fmt.Errorf("prepare media request: %w", err)
	}
	addMediaHeaders(req.Header, msg.MediaMime)
	client := &http.Client{Timeout: 2 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return "", fmt.Errorf("download media: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_ = f.Close()
		_ = os.Remove(path)
		return "", fmt.Errorf("download media: http %d", resp.StatusCode)
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return "", fmt.Errorf("write media file: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("close media file: %w", err)
	}
	return path, nil
}

func (c *Client) trackTempMedia(path string) {
	if c == nil || path == "" {
		return
	}
	c.mediaMu.Lock()
	c.tempMedia = append(c.tempMedia, path)
	c.mediaMu.Unlock()
}

func (c *Client) untrackTempMedia(path string) {
	if c == nil || path == "" {
		return
	}
	c.mediaMu.Lock()
	out := c.tempMedia[:0]
	for _, p := range c.tempMedia {
		if p != path {
			out = append(out, p)
		}
	}
	c.tempMedia = out
	c.mediaMu.Unlock()
}

func (c *Client) removeTempMediaWhenIdle(path string) {
	deadline := time.Now().Add(10 * time.Minute)
	for time.Now().Before(deadline) {
		time.Sleep(5 * time.Second)
		if err := os.Remove(path); err == nil || os.IsNotExist(err) {
			c.untrackTempMedia(path)
			return
		}
	}
	_ = os.Remove(path)
	c.untrackTempMedia(path)
}

func (c *Client) cleanupTempMedia() {
	if c == nil {
		return
	}
	c.mediaMu.Lock()
	paths := append([]string(nil), c.tempMedia...)
	c.tempMedia = nil
	c.mediaMu.Unlock()
	for _, path := range paths {
		_ = os.Remove(path)
	}
	c.purgeLegacyMediaCache()
}

func (c *Client) purgeLegacyMediaCache() {
	if c == nil || c.dataDir == "" {
		return
	}
	_ = os.RemoveAll(filepath.Join(c.dataDir, "media-cache"))
}

func addMediaHeaders(hdr http.Header, mime string) {
	hdr.Set("Accept", "*/*")
	switch strings.Split(mime, "/")[0] {
	case "image":
		hdr.Set("Accept", "image/avif,image/webp,*/*")
		hdr.Set("Sec-Fetch-Dest", "image")
	case "video":
		hdr.Set("Sec-Fetch-Dest", "video")
	default:
		hdr.Set("Sec-Fetch-Dest", "empty")
	}
	hdr.Set("Sec-Fetch-Mode", "no-cors")
	hdr.Set("Sec-Fetch-Site", "cross-site")
	hdr.Set("User-Agent", useragent.UserAgent)
	hdr.Set("sec-ch-ua", useragent.SecCHUserAgent)
	hdr.Set("sec-ch-ua-platform", useragent.SecCHPlatform)
}

func sanitizeMediaName(name, kind, mime string) string {
	name = filepath.Base(strings.TrimSpace(name))
	name = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '-', r == '_':
			return r
		default:
			return '-'
		}
	}, name)
	name = strings.Trim(name, "-.")
	if name == "" || name == "." {
		name = kind
		if name == "" {
			name = "media"
		}
	}
	if filepath.Ext(name) == "" {
		switch {
		case strings.HasPrefix(mime, "image/jpeg"):
			name += ".jpg"
		case strings.HasPrefix(mime, "image/png"):
			name += ".png"
		case strings.HasPrefix(mime, "image/webp"):
			name += ".webp"
		case strings.HasPrefix(mime, "image/gif"):
			name += ".gif"
		case strings.HasPrefix(mime, "video/mp4"):
			name += ".mp4"
		case strings.HasPrefix(mime, "video/"):
			name += ".mp4"
		case kind == "image":
			name += ".jpg"
		case kind == "video":
			name += ".mp4"
		}
	}
	return name
}

func openLocalFile(path string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "linux":
		cmd = exec.Command("xdg-open", path)
	case "darwin":
		cmd = exec.Command("open", path)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", path)
	default:
		return fmt.Errorf("unsupported platform %s", runtime.GOOS)
	}
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return err
	}
	_ = cmd.Process.Release()
	return nil
}
