package ui

import (
	"lazyfmess/internal/store"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

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
	if cmd, ok := m.motionKey(k); ok {
		return m, cmd
	}
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
	case key.Matches(k, keys.Back) && m.focus == focusComposer && m.editing.ID != "":
		m.cancelEdit()
		return m, nil
	case key.Matches(k, keys.Back) && m.focus != focusList:
		m.setFocus(focusList)
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
	case key.Matches(k, keys.UnlockPIN):
		return m, m.checkUnlock()
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
		return m, m.openThread(item.thread)
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
		return m, m.loadOlderAtTop()
	case key.Matches(k, keys.Top):
		return m, m.toEdge(true)
	case key.Matches(k, keys.Bottom):
		m.moveCursor(len(m.order))
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
	m.pending = pendingMotion{}
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
