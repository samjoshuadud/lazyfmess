package ui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"lazyfmess/internal/store"
)

type fileSender struct {
	stubSender
	mu   sync.Mutex
	path string
}

func (f *fileSender) SendFile(_ context.Context, _ store.Thread, path string) (SendResult, error) {
	f.mu.Lock()
	f.path = path
	f.mu.Unlock()
	return SendResult{MessageID: "sent-file", Timestamp: time.UnixMilli(1_700_000_900_000)}, nil
}

func TestAttachPromptSendsFile(t *testing.T) {
	t.Parallel()
	m := newTestModel(t, 1, 1)
	sender := &fileSender{stubSender: stubSender{id: 1}}
	m.sender = sender
	m = drain(t, m, keyPress("2"))
	model, _ := m.Update(keyPress("A"))
	m = model.(Model)
	if m.popup == nil || m.popup.kind != popupPrompt {
		t.Fatalf("expected prompt, got %+v", m.popup)
	}
	for _, r := range "~/notes.txt" {
		model, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = model.(Model)
	}
	m = drain(t, m, keyPress("enter"))
	home, _ := os.UserHomeDir()
	if want := filepath.Join(home, "notes.txt"); sender.path != want {
		t.Fatalf("sent path %q, want %q", sender.path, want)
	}
	last := m.messages[len(m.messages)-1]
	if last.ID != "sent-file" || last.MediaName != "notes.txt" {
		t.Fatalf("sent message: %+v", last)
	}
	if !strings.Contains(ansi.Strip(m.viewport.View()), "[file notes.txt]") {
		t.Fatalf("file placeholder missing:\n%s", ansi.Strip(m.viewport.View()))
	}
	if thread, _ := m.currentThread(); thread.LastPreview != "You sent notes.txt" {
		t.Fatalf("preview: %q", thread.LastPreview)
	}
}
