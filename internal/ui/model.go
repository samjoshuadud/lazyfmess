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
	"github.com/charmbracelet/lipgloss"

	"fmesstui/internal/store"
)

type SendResult struct {
	MessageID string
	Timestamp time.Time
}

type Sender interface {
	SendText(ctx context.Context, thread store.Thread, text string) (SendResult, error)
	AccountID() int64
}

type threadItem struct {
	thread store.Thread
}

func (t threadItem) Title() string { return t.thread.Title }
func (t threadItem) Description() string {
	if t.thread.LastPreview == "" {
		return fmt.Sprintf("%d", t.thread.ID)
	}
	return t.thread.LastPreview
}
func (t threadItem) FilterValue() string { return t.thread.Title }

type Model struct {
	store      *store.Store
	sender     Sender
	ctx        context.Context
	list       list.Model
	viewport   viewport.Model
	composer   textarea.Model
	ready      bool
	selectedID int64
	status     string
	width      int
	height     int
}

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

func New(ctx context.Context, s *store.Store, sender Sender) Model {
	delegate := list.NewDefaultDelegate()
	l := list.New(nil, delegate, 0, 0)
	l.Title = "Threads"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.DisableQuitKeybindings()
	ta := textarea.New()
	ta.Placeholder = "message"
	ta.ShowLineNumbers = false
	ta.Prompt = "> "
	ta.SetHeight(2)
	ta.Focus()
	return Model{
		store:    s,
		sender:   sender,
		ctx:      ctx,
		list:     l,
		composer: ta,
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
		return m, nil
	case RefreshMsg:
		cmds := []tea.Cmd{m.loadThreads}
		if m.selectedID != 0 {
			cmds = append(cmds, m.loadMessages(m.selectedID))
		}
		return m, tea.Batch(cmds...)
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
				title = fmt.Sprintf("thread %d", t.ID)
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
		m.status = fmt.Sprintf("%d threads", len(items))
		if selected == 0 && len(items) > 0 {
			if item, ok := m.list.SelectedItem().(threadItem); ok {
				m.selectedID = item.thread.ID
				return m, m.loadMessages(item.thread.ID)
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
		m.viewport.SetContent(renderMessages(msg.messages))
		m.viewport.GotoBottom()
		return m, nil
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
		case "enter":
			if m.composer.Focused() && m.selectedID != 0 {
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
	prevSelected := m.selectedID
	m.list, cmd = m.list.Update(msg)
	cmds = append(cmds, cmd)
	if item, ok := m.list.SelectedItem().(threadItem); ok && item.thread.ID != prevSelected {
		m.selectedID = item.thread.ID
		cmds = append(cmds, m.loadMessages(item.thread.ID))
	}
	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)
	m.composer, cmd = m.composer.Update(msg)
	cmds = append(cmds, cmd)
	return m, tea.Batch(cmds...)
}

func (m Model) View() string {
	if !m.ready {
		return "loading"
	}
	left := m.list.View()
	right := lipgloss.JoinVertical(lipgloss.Left, m.viewport.View(), m.composer.View(), m.status)
	return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
}

func (m *Model) layout() {
	leftWidth := m.width / 3
	if leftWidth < 20 {
		leftWidth = 20
	}
	if leftWidth > m.width-20 {
		leftWidth = m.width / 2
	}
	rightWidth := m.width - leftWidth
	if rightWidth < 1 {
		rightWidth = 1
	}
	composerHeight := 3
	statusHeight := 1
	listHeight := m.height
	viewHeight := m.height - composerHeight - statusHeight
	if viewHeight < 1 {
		viewHeight = 1
	}
	m.list.SetSize(leftWidth, listHeight)
	m.viewport = viewport.New(rightWidth, viewHeight)
	m.composer.SetWidth(rightWidth)
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

func renderMessages(messages []store.Message) string {
	if len(messages) == 0 {
		return "no messages"
	}
	var b strings.Builder
	for _, msg := range messages {
		ts := msg.Timestamp.Local().Format(time.Kitchen)
		line := fmt.Sprintf("[%s] %d: %s", ts, msg.SenderID, msg.Text)
		if msg.Pending {
			line += " (pending)"
		}
		if msg.Error != "" {
			line += " error=" + msg.Error
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

type App struct {
	program *tea.Program
}

func NewApp(ctx context.Context, s *store.Store, sender Sender) *App {
	p := tea.NewProgram(New(ctx, s, sender), tea.WithAltScreen(), tea.WithContext(ctx))
	return &App{program: p}
}

func (a *App) Notify() {
	if a == nil || a.program == nil {
		return
	}
	a.program.Send(RefreshMsg{})
}

func (a *App) Run() error {
	_, err := a.program.Run()
	if err != nil {
		return fmt.Errorf("run tui: %w", err)
	}
	return nil
}
