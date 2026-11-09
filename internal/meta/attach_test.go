package meta

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestReadAttachment(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 800, 200))); err != nil {
		t.Fatalf("png.Encode: %v", err)
	}
	files := map[string][]byte{
		"photo.png": buf.Bytes(),
		"clip.mp4":  []byte("not really a video"),
		"notes.txt": []byte("hello"),
		"anim.gif":  []byte("GIF89a"),
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}
	tests := []struct {
		name, kind string
		w, h       int
	}{
		{"photo.png", "image", 800, 200},
		{"clip.mp4", "video", 0, 0},
		{"notes.txt", "file", 0, 0},
		{"anim.gif", "gif", 0, 0},
	}
	for _, tt := range tests {
		a, err := readAttachment(filepath.Join(dir, tt.name))
		if err != nil {
			t.Fatalf("%s: %v", tt.name, err)
		}
		if a.kind != tt.kind || a.width != tt.w || a.height != tt.h || a.name != tt.name {
			t.Errorf("%s: got kind %q %dx%d name %q", tt.name, a.kind, a.width, a.height, a.name)
		}
	}
	if _, err := readAttachment(dir); err == nil {
		t.Error("directory should be rejected")
	}
	if w, h := thumbnailSize(800, 200); w != 400 || h != 100 {
		t.Errorf("thumbnailSize(800, 200) = %d, %d", w, h)
	}
}
