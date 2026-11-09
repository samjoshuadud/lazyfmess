package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"lazyfmess/internal/store"
)

type focusPane int

const (
	focusList focusPane = iota
	focusMessages
	focusComposer
)

type SendResult struct {
	MessageID string
	Timestamp time.Time
}

type Sender interface {
	SendText(ctx context.Context, thread store.Thread, text string, replyTo store.Message) (SendResult, error)
	SendFile(ctx context.Context, thread store.Thread, path string) (SendResult, error)
	AccountID() int64
}

type HistoryLoader interface {
	LoadHistory(ctx context.Context, thread store.Thread) error
}

type HistoryUnlocker interface {
	NeedsPIN(ctx context.Context) (bool, error)
	Unlock(ctx context.Context, pin string) error
}

type MediaOpener interface {
	OpenMedia(ctx context.Context, msg store.Message) error
}

type ReadMarker interface {
	MarkRead(ctx context.Context, thread store.Thread) error
}

type Services struct {
	Sender   Sender
	History  HistoryLoader
	Unlocker HistoryUnlocker
	Searcher PersonSearcher
	Media    MediaOpener
	Reader   ReadMarker
	Typing   TypingNotifier
	Reactor  Reactor
	Editor   MessageEditor
	Muter    Muter
}

type Config struct {
	PrefillPIN string
	OnReady    func()
}

type threadItem struct {
	thread store.Thread
}

func (t threadItem) FilterValue() string { return t.thread.Title }

type Model struct {
	store          *store.Store
	sender         Sender
	history        HistoryLoader
	unlocker       HistoryUnlocker
	searcher       PersonSearcher
	media          MediaOpener
	reader         ReadMarker
	typingNotifier TypingNotifier
	reactor        Reactor
	messageEditor  MessageEditor
	muter          Muter
	typing         map[int64]map[int64]time.Time
	typingThread   int64
	typingSentAt   time.Time
	onReady        func()
	prefillPIN     string
	ctx            context.Context
	keys           keyMap
	list           list.Model
	viewport       viewport.Model
	spinner        spinner.Model
	inFlight       int
	popup          *popup
	composer       textarea.Model
	pinInput       textinput.Model
	searchInput    textinput.Model
	focus          focusPane
	ready          bool
	showPIN        bool
	showHelp       bool
	showSearch     bool
	pinErr         string
	searchErr      string
	searchQuery    string
	searchResults  []PersonHit
	searchIndex    int
	selectedID     int64
	openedID       int64
	lastMark       readMark
	loadedID       int64
	cursorID       string
	replyTo        store.Message
	editing        store.Message
	spans          map[string]lineSpan
	order          []string
	status         string
	toast          string
	width          int
	height         int
	leftWidth      int
	rightWidth     int
	messages       []store.Message
	readMarks      []time.Time
}

type readMark struct {
	threadID int64
	activity int64
}

type markReadMsg struct {
	err error
}

type workDoneMsg struct {
	msg tea.Msg
}

type historyFinishedMsg struct {
	threadID int64
	err      error
}

type bootMsg struct {
	askPIN bool
	err    error
}

func New(ctx context.Context, s *store.Store, svc Services, cfg Config) Model {
	composer := newComposer()
	composer.Blur()
	return Model{
		store:          s,
		sender:         svc.Sender,
		history:        svc.History,
		unlocker:       svc.Unlocker,
		searcher:       svc.Searcher,
		media:          svc.Media,
		reader:         svc.Reader,
		typingNotifier: svc.Typing,
		reactor:        svc.Reactor,
		messageEditor:  svc.Editor,
		muter:          svc.Muter,
		typing:         make(map[int64]map[int64]time.Time),
		onReady:        cfg.OnReady,
		prefillPIN:     strings.TrimSpace(cfg.PrefillPIN),
		ctx:            ctx,
		keys:           newKeyMap(),
		list:           newThreadList(),
		viewport:       viewport.New(1, 1),
		spinner:        spinner.New(spinner.WithSpinner(spinner.MiniDot), spinner.WithStyle(lipgloss.NewStyle().Foreground(accent))),
		composer:       composer,
		pinInput:       newPINInput(),
		focus:          focusList,
		status:         "loading",
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.loadThreads, m.boot)
}

func (m Model) boot() tea.Msg {
	if m.prefillPIN != "" {
		if m.unlocker == nil {
			return bootMsg{}
		}
		if err := m.unlocker.Unlock(m.ctx, m.prefillPIN); err != nil {
			return bootMsg{err: err}
		}
		return bootMsg{}
	}
	if m.unlocker == nil {
		return bootMsg{}
	}
	needs, err := m.unlocker.NeedsPIN(m.ctx)
	if err != nil {
		return bootMsg{err: err}
	}
	return bootMsg{askPIN: needs}
}

func (m *Model) kickHistorySync() {
	if m.onReady != nil {
		m.onReady()
		m.onReady = nil
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case workDoneMsg:
		m.inFlight--
		return m.Update(msg.msg)
	case spinner.TickMsg:
		if m.inFlight == 0 {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.layout()
		m.ready = true
		m.refreshViewport()
		return m, nil
	case errorMsg:
		m.showError(msg.text)
		return m, nil
	case bootMsg:
		if msg.err != nil {
			m.showError(msg.err.Error())
			m.kickHistorySync()
			return m, nil
		}
		if msg.askPIN {
			m.openPINModal()
			return m, textinput.Blink
		}
		m.kickHistorySync()
		return m, nil
	case pinResultMsg:
		if msg.err != nil {
			m.pinErr = msg.err.Error()
			m.showPIN = true
			return m, nil
		}
		m.closePINModal()
		if msg.skipped {
			m.status = "history unlock skipped"
		} else {
			m.status = "history unlocked"
		}
		m.kickHistorySync()
		return m, nil
	case searchResultMsg:
		return m.updateSearch(msg)
	case openPersonMsg:
		return m.updateSearch(msg)
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
			m.showError(msg.err.Error())
			return m, nil
		}
		selected := m.selectedID
		items := make([]list.Item, 0, len(msg.threads))
		selectedIndex := 0
		for i, t := range msg.threads {
			t.Title = threadTitle(t)
			items = append(items, threadItem{thread: t})
			if t.ID == selected {
				selectedIndex = i
			}
		}
		m.list.SetItems(items)
		if len(items) > 0 {
			m.list.Select(selectedIndex)
		}
		m.status = threadSummary(msg.threads)
		if selected == 0 && len(items) > 0 {
			if item, ok := m.list.SelectedItem().(threadItem); ok {
				m.selectedID = item.thread.ID
				return m, m.loadMessages(item.thread.ID)
			}
		}
		return m, m.maybeMarkRead()
	case messagesLoadedMsg:
		if msg.err != nil {
			m.showError(msg.err.Error())
			return m, nil
		}
		if msg.threadID != m.selectedID {
			return m, nil
		}
		newThread := msg.threadID != m.loadedID
		follow := newThread || m.viewport.AtBottom()
		cursorAtEnd := len(m.order) == 0 || m.cursorID == m.order[len(m.order)-1]
		m.loadedID = msg.threadID
		m.messages = msg.messages
		m.readMarks = msg.readMarks
		m.refreshViewport()
		m.placeCursor(newThread || cursorAtEnd)
		m.refreshViewport()
		if follow {
			m.viewport.GotoBottom()
		}
		return m, m.maybeMarkRead()
	case typingMsg:
		return m, m.applyTyping(msg)
	case typingExpiredMsg:
		m.expireTyping(time.Now())
		return m, nil
	case typingSentMsg:
		if msg.err != nil {
			m.status = "typing indicator failed"
		}
		return m, nil
	case mutedMsg:
		if msg.err != nil {
			m.showError(msg.err.Error())
			return m, nil
		}
		notice := "chat unmuted"
		if msg.muted {
			notice = "chat muted"
		}
		model, cmd := m.Update(toastMsg{message: notice})
		return model, tea.Batch(cmd, m.loadThreads)
	case editedMsg:
		if msg.err != nil {
			m.showError(msg.err.Error())
			return m, nil
		}
		return m, tea.Batch(m.loadThreads, m.loadMessages(msg.threadID))
	case reactedMsg:
		if msg.err != nil {
			m.showError("react: " + msg.err.Error())
			return m, nil
		}
		return m, m.loadMessages(msg.threadID)
	case markReadMsg:
		if msg.err != nil {
			return m.Update(toastMsg{message: "mark read: " + msg.err.Error()})
		}
		return m, m.loadThreads
	case editorFinishedMsg:
		if msg.err != nil {
			m.showError(msg.err.Error())
			return m, nil
		}
		m.composer.SetValue(msg.text)
		return m, nil
	case tea.MouseMsg:
		if m.popup != nil || m.showPIN || m.showSearch || m.showHelp {
			return m, nil
		}
		return m.handleMouse(msg)
	case mediaOpenedMsg:
		if msg.err != nil {
			m.showError(msg.err.Error())
			return m, nil
		}
		m.status = "opened media"
		return m, nil
	case historyFinishedMsg:
		if msg.threadID != m.selectedID {
			return m, nil
		}
		if msg.err != nil {
			m.status = "history sync failed"
			return m.Update(toastMsg{message: "history: " + msg.err.Error()})
		}
		m.status = "history synced"
		return m, tea.Batch(m.loadThreads, m.loadMessages(msg.threadID))
	case sendFinishedMsg:
		m.status = "sent"
		if msg.err != nil {
			m.status = "send failed"
			m.showError(msg.err.Error())
		}
		return m, tea.Batch(m.loadThreads, m.loadMessages(msg.threadID))
	}

	if m.popup != nil {
		return m.updatePopup(msg)
	}
	if m.showPIN {
		return m.updatePIN(msg)
	}
	if m.showSearch {
		return m.updateSearch(msg)
	}
	if m.showHelp {
		return m.updateHelp(msg)
	}
	m, cmd := m.updateMain(msg)
	return m, tea.Batch(cmd, m.updateOwnTyping())
}

func (m Model) updateHelp(msg tea.Msg) (Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch {
		case key.Matches(k, m.keys.ForceQuit):
			return m, tea.Quit
		case key.Matches(k, m.keys.Close):
			m.showHelp = false
		}
	}
	return m, nil
}

func (m Model) updateMain(msg tea.Msg) (Model, tea.Cmd) {
	k, isKey := msg.(tea.KeyMsg)
	if !isKey || m.list.FilterState() == list.Filtering {
		return m.updateFocused(msg)
	}
	keys := m.keys
	switch {
	case key.Matches(k, keys.ForceQuit):
		return m, tea.Quit
	case key.Matches(k, keys.NextPane):
		m.cycleFocus(1)
		return m, nil
	case key.Matches(k, keys.PrevPane):
		m.cycleFocus(-1)
		return m, nil
	case key.Matches(k, keys.Back) && m.focus == focusComposer && m.replyTo.ID != "":
		m.replyTo = store.Message{}
		return m, nil
	case key.Matches(k, keys.Back) && m.focus != focusList:
		m.setFocus(focusList)
		return m, nil
	case key.Matches(k, keys.Back) && m.focus == focusComposer && m.editing.ID != "":
		m.cancelEdit()
		return m, nil
	case m.focus == focusComposer:
		if key.Matches(k, keys.Send) && m.editing.ID != "" {
			return m, m.submitEdit()
		}
		if key.Matches(k, keys.Send) {
			return m, m.queueSend()
		}
		return m.updateFocused(msg)
	case key.Matches(k, keys.Quit):
		return m, tea.Quit
	case key.Matches(k, keys.JumpChats):
		m.setFocus(focusList)
		return m, nil
	case key.Matches(k, keys.JumpChat):
		m.setFocus(focusMessages)
		return m, nil
	case key.Matches(k, keys.JumpInput) && m.selectedID != 0:
		m.setFocus(focusComposer)
		return m, nil
	case key.Matches(k, keys.Help):
		m.showHelp = true
		return m, nil
	case key.Matches(k, keys.Compose) && m.selectedID != 0:
		m.setFocus(focusComposer)
		return m, nil
	case key.Matches(k, keys.Mute) && m.focus == focusList:
		return m, m.toggleMute()
	case key.Matches(k, keys.Attach) && m.selectedID != 0:
		return m, m.showPrompt("Send a file", "path to an image, video, or file", func(m *Model, path string) tea.Cmd {
			return m.queueFile(path)
		})
	case key.Matches(k, keys.People):
		m.openSearchModal()
		return m, textinput.Blink
	case key.Matches(k, keys.Filter):
		m.setFocus(focusList)
		return m.updateFocused(msg)
	case key.Matches(k, keys.PaneRight):
		if m.focus == focusList {
			m.setFocus(focusMessages)
		} else if m.selectedID != 0 {
			m.setFocus(focusComposer)
		}
		return m, nil
	case key.Matches(k, keys.PaneLeft):
		m.setFocus(focusList)
		return m, nil
	case m.focus == focusList && key.Matches(k, keys.OpenChat):
		item, ok := m.list.SelectedItem().(threadItem)
		if !ok {
			return m, nil
		}
		m.selectedID = item.thread.ID
		m.openedID = item.thread.ID
		m.setFocus(focusMessages)
		return m, tea.Batch(m.loadMessages(item.thread.ID), m.track(m.fetchHistory(item.thread)), m.maybeMarkRead())
	}
	return m.updateFocused(msg)
}

func (m Model) updateFocused(msg tea.Msg) (Model, tea.Cmd) {
	switch m.focus {
	case focusList:
		return m.updateList(msg)
	case focusMessages:
		return m.updateMessages(msg)
	default:
		return m.updateComposer(msg)
	}
}

func (m Model) updateList(msg tea.Msg) (Model, tea.Cmd) {
	prevSelected := m.selectedID
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	cmds := []tea.Cmd{cmd}
	if item, ok := m.list.SelectedItem().(threadItem); ok && item.thread.ID != prevSelected {
		m.selectedID = item.thread.ID
		cmds = append(cmds, m.loadMessages(item.thread.ID))
	}
	return m, tea.Batch(cmds...)
}

func (m Model) updateMessages(msg tea.Msg) (Model, tea.Cmd) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	}
	keys := m.keys
	switch {
	case key.Matches(k, keys.Down):
		m.moveCursor(1)
	case key.Matches(k, keys.Up):
		m.moveCursor(-1)
	case key.Matches(k, keys.Top):
		m.moveCursor(-len(m.order))
	case key.Matches(k, keys.Bottom):
		m.moveCursor(len(m.order))
	case key.Matches(k, keys.HalfDown):
		m.viewport.HalfPageDown()
	case key.Matches(k, keys.HalfUp):
		m.viewport.HalfPageUp()
	case key.Matches(k, keys.NextMedia):
		m.jumpMedia(1)
	case key.Matches(k, keys.PrevMedia):
		m.jumpMedia(-1)
	case key.Matches(k, keys.OpenMedia):
		msg, _ := m.cursorMessage()
		return m, m.track(m.openMedia(msg))
	case key.Matches(k, keys.Edit):
		if msg, ok := m.cursorMessage(); ok {
			m.startEdit(msg)
		}
	case key.Matches(k, keys.Unsend):
		if msg, ok := m.cursorMessage(); ok {
			m.confirmUnsend(msg)
		}
	case key.Matches(k, keys.React):
		if msg, ok := m.cursorMessage(); ok {
			m.openReactionMenu(msg)
		}
	case key.Matches(k, keys.Reply):
		if msg, ok := m.cursorMessage(); ok {
			m.startReply(msg)
		}
	case key.Matches(k, keys.Copy):
		if msg, ok := m.cursorMessage(); ok && strings.TrimSpace(msg.Text) != "" {
			return m, copyText(msg.Text)
		}
	case key.Matches(k, keys.Actions):
		msg, ok := m.cursorMessage()
		if !ok {
			return m, nil
		}
		if items := m.messageActions(msg); len(items) > 0 {
			m.showMenu("Message", items)
		}
	}
	return m, nil
}

func (m Model) updateComposer(msg tea.Msg) (Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok && key.Matches(k, m.keys.Editor) {
		return m, editDraft(m.composer.Value())
	}
	var cmd tea.Cmd
	m.composer, cmd = m.composer.Update(msg)
	return m, cmd
}

func (m *Model) cycleFocus(delta int) {
	next := int(m.focus) + delta
	if next < 0 {
		next = int(focusComposer)
	}
	if next > int(focusComposer) {
		next = int(focusList)
	}
	m.setFocus(focusPane(next))
}

func (m *Model) setFocus(pane focusPane) {
	m.focus = pane
	m.applyFocus()
	m.refreshViewport()
}

func (m *Model) applyFocus() {
	switch m.focus {
	case focusComposer:
		m.composer.Focus()
	default:
		m.composer.Blur()
	}
}

func (m Model) selfAccountID() int64 {
	if m.sender == nil {
		return 0
	}
	return m.sender.AccountID()
}

func (m *Model) track(cmd tea.Cmd) tea.Cmd {
	if cmd == nil {
		return nil
	}
	m.inFlight++
	work := func() tea.Msg { return workDoneMsg{msg: cmd()} }
	if m.inFlight == 1 {
		return tea.Batch(work, m.spinner.Tick)
	}
	return work
}

func (m Model) loadThreads() tea.Msg {
	threads, err := m.store.ListThreads(m.ctx)
	return threadsLoadedMsg{threads: threads, err: err}
}

func (m Model) loadMessages(threadID int64) tea.Cmd {
	return func() tea.Msg {
		msgs, err := m.store.ListMessages(m.ctx, threadID)
		if err != nil {
			return messagesLoadedMsg{threadID: threadID, err: err}
		}
		marks, err := m.store.ReadMarks(m.ctx, threadID, m.selfAccountID())
		return messagesLoadedMsg{threadID: threadID, messages: msgs, readMarks: marks, err: err}
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

func (m *Model) queueSend() tea.Cmd {
	text := strings.TrimSpace(m.composer.Value())
	if text == "" || m.sender == nil || m.selectedID == 0 {
		return nil
	}
	thread, err := m.store.GetThread(m.ctx, m.selectedID)
	if err != nil {
		m.showError(fmt.Sprintf("send: %v", err))
		return nil
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
	if err := m.store.UpsertMessage(m.ctx, pending); err != nil {
		m.showError(fmt.Sprintf("send: %v", err))
		return nil
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
	thread, err := m.store.GetThread(m.ctx, m.selectedID)
	if err != nil {
		m.showError(fmt.Sprintf("send: %v", err))
		return nil
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
	if err := m.store.UpsertMessage(m.ctx, pending); err != nil {
		m.showError(fmt.Sprintf("send: %v", err))
		return nil
	}
	m.status = "uploading " + pending.MediaName
	return m.startSend(thread, pending, func() (SendResult, error) {
		return m.sender.SendFile(m.ctx, thread, path)
	})
}

func (m *Model) startSend(thread store.Thread, pending store.Message, send func() (SendResult, error)) tea.Cmd {
	done := func() tea.Msg {
		return sendFinishedMsg{threadID: thread.ID, err: m.finishSend(thread, pending, send)}
	}
	return tea.Batch(m.loadMessages(thread.ID), m.track(done))
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

func (m Model) currentThread() (store.Thread, bool) {
	return m.threadByID(m.selectedID)
}

func (m *Model) maybeMarkRead() tea.Cmd {
	if m.reader == nil || m.openedID == 0 || m.openedID != m.selectedID {
		return nil
	}
	thread, ok := m.currentThread()
	if !ok || !thread.Unread() {
		return nil
	}
	mark := readMark{threadID: thread.ID, activity: thread.UpdatedAt.UnixMilli()}
	if mark == m.lastMark {
		return nil
	}
	m.lastMark = mark
	return func() tea.Msg {
		return markReadMsg{err: m.reader.MarkRead(m.ctx, thread)}
	}
}

func threadSummary(threads []store.Thread) string {
	unread := 0
	for _, t := range threads {
		if t.Unread() {
			unread++
		}
	}
	if unread == 0 {
		return fmt.Sprintf("%d chats", len(threads))
	}
	return fmt.Sprintf("%d chats · %d unread", len(threads), unread)
}

func threadTitle(t store.Thread) string {
	if t.Title != "" {
		return t.Title
	}
	return fmt.Sprintf("Chat %d", t.ID)
}

type App struct {
	program *tea.Program
}

func NewApp(ctx context.Context, s *store.Store, svc Services, cfg Config) *App {
	p := tea.NewProgram(New(ctx, s, svc, cfg), tea.WithAltScreen(), tea.WithMouseCellMotion(), tea.WithContext(ctx))
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

func (a *App) Typing(threadID, senderID int64, active bool) {
	if a == nil || a.program == nil {
		return
	}
	a.program.Send(typingMsg{threadID: threadID, senderID: senderID, active: active})
}

func (a *App) Error(message string) {
	if a == nil || a.program == nil || strings.TrimSpace(message) == "" {
		return
	}
	a.program.Send(errorMsg{text: message})
}

func (a *App) Run() error {
	_, err := a.program.Run()
	if err != nil {
		return fmt.Errorf("run tui: %w", err)
	}
	return nil
}
