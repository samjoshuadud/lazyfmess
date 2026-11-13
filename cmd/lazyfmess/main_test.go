package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRotateLog(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "lazyfmess.log")
	if err := rotateLog(path, 10); err != nil {
		t.Fatalf("missing log: %v", err)
	}
	if err := os.WriteFile(path, []byte("short"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := rotateLog(path, 10); err != nil {
		t.Fatalf("small log: %v", err)
	}
	if _, err := os.Stat(path + ".1"); err == nil {
		t.Fatal("a small log should not rotate")
	}
	if err := os.WriteFile(path, []byte("much longer than ten bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := rotateLog(path, 10); err != nil {
		t.Fatalf("large log: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("the large log should have moved")
	}
	if data, err := os.ReadFile(path + ".1"); err != nil || string(data) != "much longer than ten bytes" {
		t.Fatalf("rotated log: %q %v", data, err)
	}
}
