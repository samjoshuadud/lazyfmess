package session

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSaveAndLoadState(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	want := json.RawMessage(`{"packets_sent":1,"timestamp":"2026-01-02T03:04:05Z"}`)
	if err := SaveState(dir, want); err != nil {
		t.Fatalf("SaveState: %v", err)
	}
	got, err := LoadState(dir)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if !bytes.Equal(want, got) {
		t.Fatalf("state mismatch: got %s want %s", got, want)
	}
	info, err := os.Stat(filepath.Join(dir, StateFileName))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("state file permissions too open: %v", info.Mode())
	}
}

func TestLoadStateMissing(t *testing.T) {
	t.Parallel()
	_, err := LoadState(t.TempDir())
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestSaveStateEmpty(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := SaveState(dir, nil); err != nil {
		t.Fatalf("SaveState: %v", err)
	}
	if _, err := os.Stat(StatePath(dir)); !os.IsNotExist(err) {
		t.Fatalf("expected no file, err=%v", err)
	}
}
