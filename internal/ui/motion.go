package ui

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

const maxMotionCount = 9999

type pendingMotion struct {
	count  int
	prefix string
}

func (p pendingMotion) label() string {
	var b strings.Builder
	if p.count > 0 {
		b.WriteString(strconv.Itoa(p.count))
	}
	b.WriteString(p.prefix)
	return b.String()
}

func (m *Model) motionKey(k tea.KeyMsg) (tea.Cmd, bool) {
	if m.focus == focusComposer || k.Paste {
		return nil, false
	}
	pending := m.pending
	m.pending = pendingMotion{}
	s := k.String()
	if pending.prefix != "" {
		return m.chord(pending, s)
	}
	if len(s) == 1 && s[0] >= '0' && s[0] <= '9' && (pending.count > 0 || s[0] >= '4') {
		m.pending.count = min(pending.count*10+int(s[0]-'0'), maxMotionCount)
		return nil, true
	}
	count := max(pending.count, 1)
	switch {
	case s == "g" || s == "z" || s == "[" || s == "]":
		m.pending = pendingMotion{count: pending.count, prefix: s}
		return nil, true
	case s == "/" && m.focus == focusMessages:
		return m.promptChatSearch(), true
	case s == "n" && m.focus == focusMessages && m.search != "":
		return m.jumpMatch(count), true
	case s == "N" && m.focus == focusMessages && m.search != "":
		return m.jumpMatch(-count), true
	case pending.count > 0 && key.Matches(k, m.keys.Down):
		return m.step(count), true
	case pending.count > 0 && key.Matches(k, m.keys.Up):
		return m.step(-count), true
	case key.Matches(k, m.keys.HalfDown):
		return m.page(1, 2), true
	case key.Matches(k, m.keys.HalfUp):
		return m.page(-1, 2), true
	case key.Matches(k, m.keys.PageDown):
		return m.page(1, 1), true
	case key.Matches(k, m.keys.PageUp):
		return m.page(-1, 1), true
	case m.focus == focusMessages && (s == "H" || s == "M" || s == "L"):
		m.cursorToView(s)
		return nil, true
	}
	return nil, false
}

func (m *Model) chord(p pendingMotion, s string) (tea.Cmd, bool) {
	count := max(p.count, 1)
	switch p.prefix + s {
	case "gg":
		return m.toEdge(true), true
	case "zz", "zt", "zb":
		m.alignCursor(p.prefix + s)
		return nil, true
	case "]m":
		m.jumpMedia(count)
		return nil, true
	case "[m":
		m.jumpMedia(-count)
		return nil, true
	case "]u":
		return m.jumpUnread(count), true
	case "[u":
		return m.jumpUnread(-count), true
	}
	return nil, true
}

func (m *Model) step(delta int) tea.Cmd {
	if m.focus == focusMessages {
		m.moveCursor(delta)
		if delta < 0 {
			return m.loadOlderAtTop()
		}
		return nil
	}
	for range max(delta, -delta) {
		if delta > 0 {
			m.list.CursorDown()
		} else {
			m.list.CursorUp()
		}
	}
	return m.refreshList()
}

func (m *Model) toEdge(top bool) tea.Cmd {
	if m.focus == focusMessages {
		if top {
			m.moveCursor(-len(m.order))
			return m.loadOlderAtTop()
		}
		m.moveCursor(len(m.order))
		return nil
	}
	if top {
		m.list.Select(0)
	} else {
		m.list.Select(len(m.list.VisibleItems()) - 1)
	}
	return m.refreshList()
}

func (m *Model) refreshList() tea.Cmd {
	next, cmd := m.updateList(nil)
	*m = next
	return cmd
}

func (m *Model) page(dir, divisor int) tea.Cmd {
	if m.focus == focusList {
		return m.step(dir * max(m.list.Height()/divisor, 1))
	}
	if dir > 0 {
		m.viewport.ScrollDown(max(m.viewport.Height/divisor, 1))
	} else {
		m.viewport.ScrollUp(max(m.viewport.Height/divisor, 1))
	}
	m.keepCursorInView()
	if dir < 0 {
		return m.loadOlderAtTop()
	}
	return nil
}

func (m Model) visibleMessages() []string {
	top, bottom := m.viewport.YOffset, m.viewport.YOffset+m.viewport.Height
	var ids []string
	for _, id := range m.order {
		if span := m.spans[id]; span.end >= top && span.start < bottom {
			ids = append(ids, id)
		}
	}
	return ids
}

func (m *Model) keepCursorInView() {
	visible := m.visibleMessages()
	if len(visible) == 0 {
		return
	}
	for _, id := range visible {
		if id == m.cursorID {
			return
		}
	}
	if span := m.spans[m.cursorID]; span.start < m.viewport.YOffset {
		m.cursorID = visible[0]
	} else {
		m.cursorID = visible[len(visible)-1]
	}
	m.refreshViewport()
}

func (m *Model) cursorToView(which string) {
	visible := m.visibleMessages()
	if len(visible) == 0 {
		return
	}
	switch which {
	case "H":
		m.cursorID = visible[0]
	case "M":
		m.cursorID = visible[len(visible)/2]
	default:
		m.cursorID = visible[len(visible)-1]
	}
	m.refreshViewport()
}

func (m *Model) alignCursor(where string) {
	span, ok := m.spans[m.cursorID]
	if !ok {
		return
	}
	switch where {
	case "zt":
		m.viewport.SetYOffset(span.start)
	case "zb":
		m.viewport.SetYOffset(span.end - m.viewport.Height + 1)
	default:
		m.viewport.SetYOffset(span.start - (m.viewport.Height-(span.end-span.start+1))/2)
	}
}

func (m *Model) jumpUnread(delta int) tea.Cmd {
	items := m.list.VisibleItems()
	current := -1
	for i, item := range items {
		if t, ok := item.(threadItem); ok && t.thread.ID == m.selectedID {
			current = i
		}
	}
	dir := 1
	if delta < 0 {
		dir = -1
	}
	for n := 1; n <= len(items); n++ {
		i := ((current+dir*n)%len(items) + len(items)) % len(items)
		if t, ok := items[i].(threadItem); ok && t.thread.Unread() && t.thread.ID != m.selectedID {
			m.list.Select(i)
			return m.openThread(t.thread)
		}
	}
	m.status = "no unread chats"
	return nil
}

func (m *Model) promptChatSearch() tea.Cmd {
	return m.showPrompt("Search chat", "text to find", func(m *Model, query string) tea.Cmd {
		m.search = strings.ToLower(strings.TrimSpace(query))
		if m.search == "" {
			m.refreshViewport()
			return nil
		}
		if len(m.searchMatches()) == 0 {
			m.status = "no matches for " + m.search
			m.search = ""
			m.refreshViewport()
			return nil
		}
		return m.jumpMatch(1)
	})
}

func (m Model) searchMatches() map[string]bool {
	if m.search == "" {
		return nil
	}
	matches := make(map[string]bool)
	for _, msg := range m.messages {
		if !msg.System && strings.Contains(strings.ToLower(messageBody(msg)), m.search) {
			matches[msg.ID] = true
		}
	}
	return matches
}

func (m *Model) jumpMatch(delta int) tea.Cmd {
	matches := m.searchMatches()
	if len(matches) == 0 || len(m.order) == 0 {
		return nil
	}
	dir := 1
	if delta < 0 {
		dir = -1
	}
	at := m.cursorIndex()
	for n := 0; n < max(delta, -delta); n++ {
		for i := 1; i <= len(m.order); i++ {
			next := ((at+dir*i)%len(m.order) + len(m.order)) % len(m.order)
			if matches[m.order[next]] {
				at = next
				break
			}
		}
	}
	m.setCursor(m.order[at])
	m.status = "/" + m.search
	if at == 0 {
		return m.loadOlderAtTop()
	}
	return nil
}
