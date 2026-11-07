package ui

import "fmesstui/internal/store"

type threadsLoadedMsg struct {
	threads []store.Thread
	err     error
}

type messagesLoadedMsg struct {
	threadID int64
	messages []store.Message
	err      error
}

type sendFinishedMsg struct {
	pendingID string
	threadID  int64
	result    SendResult
	text      string
	err       error
}

type RefreshMsg struct{}

type toastExpiredMsg struct {
	message string
}

type toastMsg struct {
	message string
}
