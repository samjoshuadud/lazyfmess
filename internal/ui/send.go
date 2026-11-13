package ui

import (
	"fmt"
	"lazyfmess/internal/store"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) queueSend() tea.Cmd {
	text := strings.TrimSpace(m.composer.Value())
	if text == "" || m.sender == nil || m.selectedID == 0 {
		return nil
	}
	thread, ok := m.currentThread()
	if !ok {
		thread = store.Thread{ID: m.selectedID}
	}
	pending := store.Message{
		ID:            fmt.Sprintf("pending-%d", time.Now().UnixNano()),
		ThreadID:      thread.ID,
		SenderID:      m.sender.AccountID(),
		Text:          text,
		Timestamp:     time.Now(),
		Pending:       true,
		ReplyToID:     m.replyTo.ID,
		ReplyText:     m.replyTo.Text,
		ReplySenderID: m.replyTo.SenderID,
	}
	m.composer.Reset()
	m.replyTo = store.Message{}
	m.status = "sending"
	replyTo := store.Message{ID: pending.ReplyToID, SenderID: pending.ReplySenderID}
	return m.startSend(thread, pending, func() (SendResult, error) {
		return m.sender.SendText(m.ctx, thread, pending.Text, replyTo)
	})
}

func (m *Model) queueFile(path string) tea.Cmd {
	if path == "" || m.sender == nil || m.selectedID == 0 {
		return nil
	}
	path, err := expandHome(path)
	if err != nil {
		m.showError(err.Error())
		return nil
	}
	thread, ok := m.currentThread()
	if !ok {
		thread = store.Thread{ID: m.selectedID}
	}
	pending := store.Message{
		ID:        fmt.Sprintf("pending-%d", time.Now().UnixNano()),
		ThreadID:  thread.ID,
		SenderID:  m.sender.AccountID(),
		Timestamp: time.Now(),
		Pending:   true,
		MediaKind: "file",
		MediaName: filepath.Base(path),
	}
	m.status = "uploading " + pending.MediaName
	return m.startSend(thread, pending, func() (SendResult, error) {
		return m.sender.SendFile(m.ctx, thread, path)
	})
}

func (m *Model) startSend(thread store.Thread, pending store.Message, send func() (SendResult, error)) tea.Cmd {
	m.messages = append(m.messages, pending)
	m.order = messageOrder(m.messages)
	m.cursorID = pending.ID
	m.refreshViewport()
	m.viewport.GotoBottom()
	return m.track(func() tea.Msg {
		if thread.ThreadType == 0 {
			if stored, err := m.store.GetThread(m.ctx, thread.ID); err == nil {
				thread = stored
			}
		}
		if err := m.store.UpsertMessage(m.ctx, pending); err != nil {
			return sendFinishedMsg{threadID: thread.ID, err: fmt.Errorf("send: %w", err)}
		}
		return sendFinishedMsg{threadID: thread.ID, err: m.finishSend(thread, pending, send)}
	})
}

func expandHome(path string) (string, error) {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("expand home directory: %w", err)
	}
	return filepath.Join(home, strings.TrimPrefix(path, "~")), nil
}

func (m Model) finishSend(thread store.Thread, pending store.Message, send func() (SendResult, error)) error {
	result, err := send()
	if err != nil {
		pending.Pending = false
		pending.Error = err.Error()
		if storeErr := m.store.UpsertMessage(m.ctx, pending); storeErr != nil {
			return fmt.Errorf("%w; save failed message: %v", err, storeErr)
		}
		return err
	}
	if err := m.store.DeleteMessage(m.ctx, pending.ID); err != nil {
		return fmt.Errorf("remove pending message: %w", err)
	}
	sent := pending
	sent.ID, sent.Timestamp, sent.Pending = result.MessageID, result.Timestamp, false
	if err := m.store.UpsertMessage(m.ctx, sent); err != nil {
		return fmt.Errorf("save sent message: %w", err)
	}
	thread.LastPreview = pending.Text
	if pending.Text == "" {
		thread.LastPreview = "You sent " + pending.MediaName
	}
	thread.UpdatedAt = result.Timestamp
	if err := m.store.UpsertThread(m.ctx, thread); err != nil {
		return fmt.Errorf("update thread: %w", err)
	}
	return m.store.SetLastRead(m.ctx, thread.ID, result.Timestamp)
}
