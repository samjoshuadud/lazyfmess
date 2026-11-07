package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"lazyfmess/internal/store"
)

type focusPane int

const (
	focusList focusPane = iota
	focusComposer
)

type SendResult struct {
	MessageID string
	Timestamp time.Time
}

type Sender interface {
	SendText(ctx context.Context, thread store.Thread, text string) (SendResult, error)
	AccountID() int64
}

type HistoryLoader interface {
	LoadHistory(ctx context.Context, thread store.Thread) error
}

type threadItem struct {
	thread store.Thread
}

func (t threadItem) Title() string { return t.thread.Title }
func (t threadItem) Description() string {
	if t.thread.LastPreview == "" {
		return " "
	}
	return t.thread.LastPreview
}
func (t threadItem) FilterValue() string { return t.thread.Title }

type Model struct {
	store      *store.Store
	sender     Sender
	history    HistoryLoader
	ctx        context.Context
	list       list.Model
	viewport   viewport.Model
	composer   textarea.Model
	focus      focusPane
	ready      bool
	selectedID int64
	status     string
	toast      string
	width      int
	height     int
	leftWidth  int
	rightWidth int
	messages   []store.Message
}

type historyFinishedMsg struct {
	threadID int64
	err      error
}

func New(ctx context.Context, s *store.Store, sender Sender, history HistoryLoader) Model {
	composer := newComposer()
	composer.Focus()
	return Model{
		store:    s,
		sender:   sender,
		history:  history,
		ctx:      ctx,
		list:     newThreadList(),
		viewport: viewport.New(1, 1),
		composer: composer,
		focus:    focusComposer,
		status:   "loading",
	}
}

func (m Model) Init() tea.Cmd {
	return m.loadThreads
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.layout()
		m.ready = true
		m.refreshViewport()
		return m, nil
	case RefreshMsg:
		if m.selectedID != 0 {
			return m, tea.Batch(m.loadThreads, m.loadMessages(m.selectedID))
		}
		return m, m.loadThreads
	case toastExpiredMsg:
		if msg.message == m.toast {
			m.toast = ""
		}
		return m, nil
	case toastMsg:
		m.toast = msg.message
		return m, tea.Tick(4*time.Second, func(time.Time) tea.Msg {
			return toastExpiredMsg{message: msg.message}
		})
	case threadsLoadedMsg:
		if msg.err != nil {
			m.status = msg.err.Error()
			return m, nil
		}
		selected := m.selectedID
		items := make([]list.Item, 0, len(msg.threads))
		selectedIndex := 0
		for i, t := range msg.threads {
			title := t.Title
			if title == "" {
				title = fmt.Sprintf("Chat %d", t.ID)
				t.Title = title
			}
			items = append(items, threadItem{thread: t})
			if t.ID == selected {
				selectedIndex = i
			}
		}
		m.list.SetItems(items)
		if len(items) > 0 {
			m.list.Select(selectedIndex)
		}
		m.status = fmt.Sprintf("%d chats", len(items))
		if selected == 0 && len(items) > 0 {
			if item, ok := m.list.SelectedItem().(threadItem); ok {
				m.selectedID = item.thread.ID
				return m, tea.Batch(m.loadMessages(item.thread.ID), m.fetchHistory(item.thread))
			}
		}
		return m, nil
	case messagesLoadedMsg:
		if msg.err != nil {
			m.status = msg.err.Error()
			return m, nil
		}
		if msg.threadID != m.selectedID {
			return m, nil
		}
		m.messages = msg.messages
		m.refreshViewport()
		m.viewport.GotoBottom()
		return m, nil
	case historyFinishedMsg:
		if msg.threadID != m.selectedID {
			return m, nil
		}
		if msg.err != nil {
			m.status = msg.err.Error()
			return m, nil
		}
		m.status = "history synced"
		return m, tea.Batch(m.loadThreads, m.loadMessages(msg.threadID))
	case sendFinishedMsg:
		if msg.err != nil {
			_ = m.store.UpsertMessage(m.ctx, store.Message{
				ID:        msg.pendingID,
				ThreadID:  msg.threadID,
				SenderID:  m.sender.AccountID(),
				Text:      msg.text,
				Timestamp: time.Now(),
				Pending:   false,
				Error:     msg.err.Error(),
			})
			m.status = msg.err.Error()
			return m, m.loadMessages(msg.threadID)
		}
		_ = m.store.DeleteMessage(m.ctx, msg.pendingID)
		_ = m.store.UpsertMessage(m.ctx, store.Message{
			ID:        msg.result.MessageID,
			ThreadID:  msg.threadID,
			SenderID:  m.sender.AccountID(),
			Text:      msg.text,
			Timestamp: msg.result.Timestamp,
		})
		_ = m.store.UpsertThread(m.ctx, store.Thread{
			ID:          msg.threadID,
			Title:       m.selectedTitle(),
			LastPreview: msg.text,
			ThreadType:  m.selectedThreadType(),
			UpdatedAt:   msg.result.Timestamp,
		})
		m.status = "sent"
		return m, tea.Batch(m.loadThreads, m.loadMessages(msg.threadID))
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			return m, tea.Quit
		case "tab":
			m.toggleFocus()
			return m, nil
		case "enter":
			if m.focus == focusComposer && m.selectedID != 0 {
				cmd, ok := m.queueSend()
				if ok {
					return m, tea.Batch(m.loadMessages(m.selectedID), cmd)
				}
				return m, nil
			}
		}
	}

	var cmds []tea.Cmd
	var cmd tea.Cmd
	switch m.focus {
	case focusList:
		prevSelected := m.selectedID
		m.list, cmd = m.list.Update(msg)
		cmds = append(cmds, cmd)
		if item, ok := m.list.SelectedItem().(threadItem); ok && item.thread.ID != prevSelected {
			m.selectedID = item.thread.ID
			cmds = append(cmds, m.loadMessages(item.thread.ID), m.fetchHistory(item.thread))
		}
		m.viewport, cmd = m.viewport.Update(msg)
		cmds = append(cmds, cmd)
	case focusComposer:
		m.composer, cmd = m.composer.Update(msg)
		cmds = append(cmds, cmd)
	}
	return m, tea.Batch(cmds...)
}

func (m *Model) toggleFocus() {
	if m.focus == focusComposer {
		m.focus = focusList
		m.composer.Blur()
		return
	}
	m.focus = focusComposer
	m.composer.Focus()
}

func (m *Model) refreshViewport() {
	m.viewport.SetContent(renderMessages(m.messages, m.rightWidth))
}

func (m Model) loadThreads() tea.Msg {
	threads, err := m.store.ListThreads(m.ctx)
	return threadsLoadedMsg{threads: threads, err: err}
}

func (m Model) loadMessages(threadID int64) tea.Cmd {
	return func() tea.Msg {
		msgs, err := m.store.ListMessages(m.ctx, threadID)
		return messagesLoadedMsg{threadID: threadID, messages: msgs, err: err}
	}
}

func (m Model) fetchHistory(thread store.Thread) tea.Cmd {
	if m.history == nil || thread.ID == 0 {
		return nil
	}
	return func() tea.Msg {
		err := m.history.LoadHistory(m.ctx, thread)
		return historyFinishedMsg{threadID: thread.ID, err: err}
	}
}

func (m *Model) queueSend() (tea.Cmd, bool) {
	text := strings.TrimSpace(m.composer.Value())
	if text == "" || m.sender == nil || m.selectedID == 0 {
		return nil, false
	}
	thread, err := m.store.GetThread(m.ctx, m.selectedID)
	if err != nil {
		return func() tea.Msg {
			return sendFinishedMsg{threadID: m.selectedID, text: text, err: err}
		}, true
	}
	pendingID := fmt.Sprintf("pending-%d", time.Now().UnixNano())
	_ = m.store.UpsertMessage(m.ctx, store.Message{
		ID:        pendingID,
		ThreadID:  thread.ID,
		SenderID:  m.sender.AccountID(),
		Text:      text,
		Timestamp: time.Now(),
		Pending:   true,
	})
	m.composer.Reset()
	m.status = "sending"
	return func() tea.Msg {
		result, err := m.sender.SendText(m.ctx, thread, text)
		return sendFinishedMsg{
			pendingID: pendingID,
			threadID:  thread.ID,
			result:    result,
			text:      text,
			err:       err,
		}
	}, true
}

func (m Model) selectedTitle() string {
	if item, ok := m.list.SelectedItem().(threadItem); ok {
		return item.thread.Title
	}
	return ""
}

func (m Model) selectedThreadType() int64 {
	if item, ok := m.list.SelectedItem().(threadItem); ok {
		return item.thread.ThreadType
	}
	return 0
}

type App struct {
	program *tea.Program
}

func NewApp(ctx context.Context, s *store.Store, sender Sender, history HistoryLoader) *App {
	p := tea.NewProgram(New(ctx, s, sender, history), tea.WithAltScreen(), tea.WithContext(ctx))
	return &App{program: p}
}

func (a *App) Notify() {
	if a == nil || a.program == nil {
		return
	}
	a.program.Send(RefreshMsg{})
}

func (a *App) Toast(message string) {
	if a == nil || a.program == nil || strings.TrimSpace(message) == "" {
		return
	}
	a.program.Send(toastMsg{message: message})
}

func (a *App) Run() error {
	_, err := a.program.Run()
	if err != nil {
		return fmt.Errorf("run tui: %w", err)
	}
	return nil
}
