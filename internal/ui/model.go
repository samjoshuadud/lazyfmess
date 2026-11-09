package ui

import (
	"context"
	"fmt"
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
	SendText(ctx context.Context, thread store.Thread, text string) (SendResult, error)
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

type Config struct {
	PrefillPIN string
	OnReady    func()
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
	store         *store.Store
	sender        Sender
	history       HistoryLoader
	unlocker      HistoryUnlocker
	searcher      PersonSearcher
	media         MediaOpener
	onReady       func()
	prefillPIN    string
	ctx           context.Context
	keys          keyMap
	list          list.Model
	viewport      viewport.Model
	spinner       spinner.Model
	inFlight      int
	composer      textarea.Model
	pinInput      textinput.Model
	searchInput   textinput.Model
	focus         focusPane
	ready         bool
	showPIN       bool
	showHelp      bool
	showSearch    bool
	pinErr        string
	searchErr     string
	searchQuery   string
	searchResults []PersonHit
	searchIndex   int
	selectedID    int64
	loadedID      int64
	mediaFocusID  string
	status        string
	toast         string
	width         int
	height        int
	leftWidth     int
	rightWidth    int
	messages      []store.Message
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

func New(ctx context.Context, s *store.Store, sender Sender, history HistoryLoader, unlocker HistoryUnlocker, searcher PersonSearcher, media MediaOpener, cfg Config) Model {
	composer := newComposer()
	composer.Blur()
	return Model{
		store:      s,
		sender:     sender,
		history:    history,
		unlocker:   unlocker,
		searcher:   searcher,
		media:      media,
		onReady:    cfg.OnReady,
		prefillPIN: strings.TrimSpace(cfg.PrefillPIN),
		ctx:        ctx,
		keys:       newKeyMap(),
		list:       newThreadList(),
		viewport:   viewport.New(1, 1),
		spinner:    spinner.New(spinner.WithSpinner(spinner.MiniDot), spinner.WithStyle(lipgloss.NewStyle().Foreground(accent))),
		composer:   composer,
		pinInput:   newPINInput(),
		focus:      focusList,
		status:     "loading",
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
	case bootMsg:
		if msg.err != nil {
			m.status = msg.err.Error()
			m.kickHistorySync()
			return m.Update(toastMsg{message: msg.err.Error()})
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
			m.status = msg.err.Error()
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
			m.status = msg.err.Error()
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
		m.status = fmt.Sprintf("%d chats", len(items))
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
		follow := msg.threadID != m.loadedID || m.viewport.AtBottom()
		m.loadedID = msg.threadID
		m.messages = msg.messages
		m.ensureMediaFocus()
		m.refreshViewport()
		if follow {
			m.viewport.GotoBottom()
		}
		return m, nil
	case mediaOpenedMsg:
		if msg.err != nil {
			m.status = msg.err.Error()
			return m, nil
		}
		m.status = "opened media"
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
		m.status = "sent"
		if msg.err != nil {
			m.status = msg.err.Error()
		}
		return m, tea.Batch(m.loadThreads, m.loadMessages(msg.threadID))
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
	return m.updateMain(msg)
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
	case key.Matches(k, keys.Back) && m.focus != focusList:
		m.setFocus(focusList)
		return m, nil
	case m.focus == focusComposer:
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
		m.setFocus(focusMessages)
		return m, tea.Batch(m.loadMessages(item.thread.ID), m.track(m.fetchHistory(item.thread)))
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
		m.viewport.LineDown(1)
	case key.Matches(k, keys.Up):
		m.viewport.LineUp(1)
	case key.Matches(k, keys.Top):
		m.viewport.GotoTop()
	case key.Matches(k, keys.Bottom):
		m.viewport.GotoBottom()
	case key.Matches(k, keys.HalfDown):
		m.viewport.HalfViewDown()
	case key.Matches(k, keys.HalfUp):
		m.viewport.HalfViewUp()
	case key.Matches(k, keys.NextMedia):
		m.cycleMediaFocus(1)
		m.refreshViewport()
		m.scrollToMediaFocus()
	case key.Matches(k, keys.PrevMedia):
		m.cycleMediaFocus(-1)
		m.refreshViewport()
		m.scrollToMediaFocus()
	case key.Matches(k, keys.OpenMedia):
		return m, m.track(m.openFocusedMedia())
	default:
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) updateComposer(msg tea.Msg) (Model, tea.Cmd) {
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
}

func (m *Model) applyFocus() {
	switch m.focus {
	case focusComposer:
		m.composer.Focus()
	default:
		m.composer.Blur()
	}
}

func (m *Model) refreshViewport() {
	m.viewport.SetContent(renderMessages(m.messages, m.viewport.Width, m.selfAccountID(), m.mediaFocusID))
}

func (m *Model) scrollToMediaFocus() {
	if m.mediaFocusID == "" || m.viewport.Height <= 0 {
		return
	}
	content := renderMessages(m.messages, m.viewport.Width, m.selfAccountID(), m.mediaFocusID)
	target := -1
	for i, line := range strings.Split(content, "\n") {
		if strings.Contains(line, "▸") {
			target = i
			break
		}
	}
	if target < 0 {
		return
	}
	offset := target - m.viewport.Height/2
	if offset < 0 {
		offset = 0
	}
	m.viewport.SetYOffset(offset)
}

func (m Model) selfAccountID() int64 {
	if m.sender == nil {
		return 0
	}
	return m.sender.AccountID()
}

func (m *Model) mediaMessages() []store.Message {
	out := make([]store.Message, 0)
	for _, msg := range m.messages {
		if msg.MediaKind != "" && msg.MediaURL != "" {
			out = append(out, msg)
		}
	}
	return out
}

func (m *Model) ensureMediaFocus() {
	media := m.mediaMessages()
	if len(media) == 0 {
		m.mediaFocusID = ""
		return
	}
	for _, msg := range media {
		if msg.ID == m.mediaFocusID {
			return
		}
	}
	m.mediaFocusID = media[len(media)-1].ID
}

func (m *Model) cycleMediaFocus(delta int) {
	media := m.mediaMessages()
	if len(media) == 0 {
		m.mediaFocusID = ""
		m.status = "no media in this chat"
		return
	}
	idx := len(media) - 1
	for i, msg := range media {
		if msg.ID == m.mediaFocusID {
			idx = i
			break
		}
	}
	idx = (idx + delta) % len(media)
	if idx < 0 {
		idx += len(media)
	}
	m.mediaFocusID = media[idx].ID
	m.status = fmt.Sprintf("media %d/%d · o open · %s", idx+1, len(media), mediaPlaceholder(media[idx], false))
}

func (m Model) openFocusedMedia() tea.Cmd {
	m.ensureMediaFocus()
	for _, msg := range m.messages {
		if msg.ID == m.mediaFocusID {
			return func() tea.Msg {
				if m.media == nil {
					return mediaOpenedMsg{err: fmt.Errorf("media open is unavailable")}
				}
				return mediaOpenedMsg{err: m.media.OpenMedia(m.ctx, msg)}
			}
		}
	}
	return func() tea.Msg {
		return mediaOpenedMsg{err: fmt.Errorf("no media in this chat")}
	}
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

func (m *Model) queueSend() tea.Cmd {
	text := strings.TrimSpace(m.composer.Value())
	if text == "" || m.sender == nil || m.selectedID == 0 {
		return nil
	}
	thread, err := m.store.GetThread(m.ctx, m.selectedID)
	if err != nil {
		m.status = fmt.Sprintf("send: %v", err)
		return nil
	}
	pending := store.Message{
		ID:        fmt.Sprintf("pending-%d", time.Now().UnixNano()),
		ThreadID:  thread.ID,
		SenderID:  m.sender.AccountID(),
		Text:      text,
		Timestamp: time.Now(),
		Pending:   true,
	}
	if err := m.store.UpsertMessage(m.ctx, pending); err != nil {
		m.status = fmt.Sprintf("send: %v", err)
		return nil
	}
	m.composer.Reset()
	m.status = "sending"
	send := func() tea.Msg {
		return sendFinishedMsg{threadID: thread.ID, err: m.sendPending(thread, pending)}
	}
	return tea.Batch(m.loadMessages(thread.ID), m.track(send))
}

func (m Model) sendPending(thread store.Thread, pending store.Message) error {
	result, err := m.sender.SendText(m.ctx, thread, pending.Text)
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
	if err := m.store.UpsertMessage(m.ctx, store.Message{
		ID:        result.MessageID,
		ThreadID:  thread.ID,
		SenderID:  pending.SenderID,
		Text:      pending.Text,
		Timestamp: result.Timestamp,
	}); err != nil {
		return fmt.Errorf("save sent message: %w", err)
	}
	thread.LastPreview = pending.Text
	thread.UpdatedAt = result.Timestamp
	if err := m.store.UpsertThread(m.ctx, thread); err != nil {
		return fmt.Errorf("update thread: %w", err)
	}
	return nil
}

func (m Model) currentThread() (store.Thread, bool) {
	for _, item := range m.list.Items() {
		if t, ok := item.(threadItem); ok && t.thread.ID == m.selectedID {
			return t.thread, true
		}
	}
	return store.Thread{}, false
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

func NewApp(ctx context.Context, s *store.Store, sender Sender, history HistoryLoader, unlocker HistoryUnlocker, searcher PersonSearcher, media MediaOpener, cfg Config) *App {
	p := tea.NewProgram(New(ctx, s, sender, history, unlocker, searcher, media, cfg), tea.WithAltScreen(), tea.WithContext(ctx))
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
