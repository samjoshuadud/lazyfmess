package ui

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

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
	loginService   LoginService
	login          *loginState
	typing         map[int64]map[int64]time.Time
	typingThread   int64
	typingSentAt   time.Time
	backgroundSync func(context.Context) error
	syncing        bool
	syncStarted    bool
	syncDone       int
	syncTotal      int
	threadsLoaded  bool
	historyLoading map[int64]bool
	prefillPIN     string
	ctx            context.Context
	keys           keyMap
	list           list.Model
	viewport       viewport.Model
	spinner        spinner.Model
	inFlight       int
	connecting     bool
	refreshQueued  bool
	popup          *popup
	popupStack     []*popup
	toastQueue     []string
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
	unreadSince    time.Time
	pending        pendingMotion
	search         string
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
	messageLimit   int
	loads          *atomic.Int64
	lastLoad       int64
	resizeGen      int
	settledGen     int
	pinBottom      bool
	moreAbove      bool
	anchorID       string
	anchorOffset   int
	blocks         map[string]string
}

type readMark struct {
	threadID int64
	activity int64
}

type markReadMsg struct {
	err error
}

const refreshDelay = 150 * time.Millisecond

type refreshNowMsg struct{}

type unlockCheckMsg struct {
	needs bool
	err   error
}

type resizeDoneMsg struct {
	gen int
}

type syncProgressMsg struct {
	done  int
	total int
}

type backgroundSyncMsg struct {
	err error
}

type connectedMsg struct {
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
	m := Model{
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
		loginService:   svc.Login,
		typing:         make(map[int64]map[int64]time.Time),
		backgroundSync: cfg.BackgroundSync,
		historyLoading: make(map[int64]bool),
		blocks:         make(map[string]string),
		loads:          new(atomic.Int64),
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
	if cfg.Connecting {
		m.connecting = true
		m.inFlight = 1
	}
	return m
}

func (m Model) Init() tea.Cmd {
	if m.connecting {
		return tea.Batch(m.loadThreads, m.boot, m.spinner.Tick)
	}
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

func (m *Model) selectVisible(threadID int64) {
	for i, item := range m.list.VisibleItems() {
		if t, ok := item.(threadItem); ok && t.thread.ID == threadID {
			m.list.Select(i)
			return
		}
	}
}

func (m *Model) openThread(t store.Thread) tea.Cmd {
	m.selectedID = t.ID
	m.openedID = t.ID
	m.unreadSince = unreadSince(t)
	m.search = ""
	m.setFocus(focusMessages)
	return tea.Batch(m.loadMessages(t.ID), m.fetchHistory(t), m.maybeMarkRead())
}

func unreadSince(t store.Thread) time.Time {
	if !t.Unread() {
		return time.Time{}
	}
	return t.LastRead
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
