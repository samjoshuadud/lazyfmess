package ui

import (
	"slices"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok && k.Type == tea.KeyRunes && len(k.Runes) > 1 {
		k.Paste = true
		msg = k
	}
	switch msg := msg.(type) {
	case workDoneMsg:
		m.inFlight--
		return m.Update(msg.msg)
	}

	for _, handle := range []func(Model, tea.Msg) (tea.Model, tea.Cmd){Model.updateWindow, Model.updateSession, Model.updateNotices, Model.updateChats} {
		if model, cmd := handle(m, msg); model != nil {
			return model, cmd
		}
	}

	if m.login != nil {
		return m.updateLogin(msg)
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

func (m Model) updateWindow(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		if m.inFlight == 0 {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case tea.WindowSizeMsg:
		if m.ready && m.resizeGen == m.settledGen {
			m.pinBottom = m.viewport.AtBottom()
		}
		m.width = msg.Width
		m.height = msg.Height
		m.layout()
		m.resizeGen++
		if !m.ready {
			m.ready = true
			m.refreshViewport()
			return m, nil
		}
		gen := m.resizeGen
		return m, tea.Tick(resizeDelay, func(time.Time) tea.Msg { return resizeDoneMsg{gen: gen} })
	case resizeDoneMsg:
		if msg.gen != m.resizeGen {
			return m, nil
		}
		m.settledGen = msg.gen
		m.refreshViewport()
		if m.pinBottom {
			m.viewport.GotoBottom()
		} else if m.cursorID != "" {
			m.setCursor(m.cursorID)
		}
		return m, nil
	case tea.MouseMsg:
		if m.login != nil || m.popup != nil || m.showPIN || m.showSearch || m.showHelp {
			return m, nil
		}
		return m.handleMouse(msg)
	default:
		return nil, nil
	}
}

func (m Model) updateSession(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case needLoginMsg:
		m.openLogin(msg.reason)
		return m, nil
	case loginStepMsg:
		return m, m.applyLoginStep(msg)
	case cookiesUsedMsg:
		if m.login == nil {
			return m, nil
		}
		if msg.err != nil {
			m.login.screen = m.login.returnTo
			m.login.err = msg.err.Error()
			return m, nil
		}
		return m, m.finishLogin()
	case connectedMsg:
		m.connecting = false
		m.inFlight--
		m.refreshViewport()
		if msg.err != nil {
			m.showError("Could not connect to Messenger: " + msg.err.Error())
			return m, nil
		}
		return m, m.reload()
	case syncProgressMsg:
		m.syncDone, m.syncTotal = msg.done, msg.total
		return m, nil
	case backgroundSyncMsg:
		m.syncing = false
		m.refreshViewport()
		if msg.err != nil {
			model, cmd := m.Update(toastMsg{message: "sync: " + msg.err.Error()})
			return model, tea.Batch(cmd, m.reload())
		}
		return m, m.reload()
	case errorMsg:
		m.showError(msg.text)
		return m, nil
	case bootMsg:
		if msg.err != nil {
			m.showError(msg.err.Error())
			return m, m.kickHistorySync()
		}
		if msg.askPIN {
			m.openPINModal()
			return m, textinput.Blink
		}
		return m, m.kickHistorySync()
	case pinResultMsg:
		if msg.err != nil {
			m.pinErr = msg.err.Error()
			m.showPIN = true
			return m, nil
		}
		m.closePINModal()
		if msg.skipped {
			m.status = "history unlock skipped"
			return m, m.kickHistorySync()
		}
		m.status = "history unlocked"
		if m.syncStarted {
			return m, m.runBackgroundSync()
		}
		return m, m.kickHistorySync()
	case unlockCheckMsg:
		switch {
		case msg.err != nil:
			m.showError("encrypted history: " + msg.err.Error())
		case !msg.needs:
			return m.Update(toastMsg{message: "nothing to unlock: encrypted history is already unlocked or has no backup"})
		default:
			m.openPINModal()
			return m, textinput.Blink
		}
		return m, nil
	case searchResultMsg:
		return m.updateSearch(msg)
	case openPersonMsg:
		return m.updateSearch(msg)
	default:
		return nil, nil
	}
}

func (m Model) updateNotices(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case RefreshMsg:
		if m.refreshQueued {
			return m, nil
		}
		m.refreshQueued = true
		return m, tea.Tick(refreshDelay, func(time.Time) tea.Msg { return refreshNowMsg{} })
	case refreshNowMsg:
		m.refreshQueued = false
		return m, m.reload()
	case toastExpiredMsg:
		if msg.message != m.toast {
			return m, nil
		}
		m.toast = ""
		if len(m.toastQueue) == 0 {
			return m, nil
		}
		next := m.toastQueue[0]
		m.toastQueue = m.toastQueue[1:]
		return m, m.showToast(next)
	case toastMsg:
		if m.toast == "" {
			return m, m.showToast(msg.message)
		}
		if msg.message != m.toast && !slices.Contains(m.toastQueue, msg.message) && len(m.toastQueue) < 5 {
			m.toastQueue = append(m.toastQueue, msg.message)
		}
		return m, nil
	case editorFinishedMsg:
		if msg.err != nil {
			m.showError(msg.err.Error())
			return m, nil
		}
		m.composer.SetValue(msg.text)
		return m, nil
	case mediaOpenedMsg:
		if msg.err != nil {
			m.showError(msg.err.Error())
			return m, nil
		}
		m.status = "opened media"
		return m, nil
	default:
		return nil, nil
	}
}

func (m Model) updateChats(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case list.FilterMatchesMsg:
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		m.selectVisible(m.selectedID)
		return m, cmd
	case threadsLoadedMsg:
		m.threadsLoaded = true
		if msg.err != nil {
			m.showError(msg.err.Error())
			return m, nil
		}
		selected := m.selectedID
		items := make([]list.Item, 0, len(msg.threads))
		for _, t := range msg.threads {
			t.Title = threadTitle(t)
			items = append(items, threadItem{thread: t})
		}
		filterCmd := m.list.SetItems(items)
		m.selectVisible(selected)
		m.status = threadSummary(msg.threads)
		if selected == 0 && len(items) > 0 {
			if item, ok := m.list.SelectedItem().(threadItem); ok {
				m.selectedID = item.thread.ID
				return m, tea.Batch(filterCmd, m.loadMessages(item.thread.ID))
			}
		}
		return m, tea.Batch(filterCmd, m.maybeMarkRead())
	case messagesLoadedMsg:
		if msg.err != nil {
			m.showError(msg.err.Error())
			return m, nil
		}
		if msg.threadID != m.selectedID || msg.seq < m.lastLoad {
			return m, nil
		}
		m.lastLoad = msg.seq
		newThread := msg.threadID != m.loadedID
		if newThread {
			m.messageLimit = messagePage
			m.anchorID = ""
		}
		m.moreAbove = len(msg.messages) >= m.messageLimit
		follow := (newThread || m.viewport.AtBottom()) && m.anchorID == ""
		cursorAtEnd := len(m.order) == 0 || m.cursorID == m.order[len(m.order)-1]
		m.loadedID = msg.threadID
		m.messages = msg.messages
		m.readMarks = msg.readMarks
		m.order = messageOrder(m.messages)
		m.placeCursor((newThread || cursorAtEnd) && m.anchorID == "")
		m.refreshViewport()
		if span, ok := m.spans[m.anchorID]; ok {
			m.viewport.SetYOffset(span.start - m.anchorOffset)
			m.anchorID = ""
			m.status = ""
		} else if follow {
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
	case historyFinishedMsg:
		delete(m.historyLoading, msg.threadID)
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
	default:
		return nil, nil
	}
}
