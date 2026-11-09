package ui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"lazyfmess/internal/store"
)

type Reactor interface {
	React(ctx context.Context, thread store.Thread, msg store.Message, emoji string) error
}

type reactedMsg struct {
	threadID int64
	err      error
}

var quickReactions = []struct{ emoji, name string }{
	{"👍", "like"},
	{"❤", "love"},
	{"😆", "haha"},
	{"😮", "wow"},
	{"😢", "sad"},
	{"😠", "angry"},
}

func (m *Model) openReactionMenu(msg store.Message) {
	if msg.Pending || msg.Error != "" {
		return
	}
	own := ownReaction(msg, m.selfAccountID())
	items := make([]menuItem, 0, len(quickReactions)+1)
	for _, q := range quickReactions {
		emoji, label := q.emoji, q.emoji+"  "+q.name
		if emoji == own {
			emoji, label = "", label+" (remove)"
		}
		items = append(items, menuItem{label, func(m *Model) tea.Cmd { return m.react(msg, emoji) }})
	}
	if own != "" && !isQuickReaction(own) {
		items = append(items, menuItem{"remove " + own, func(m *Model) tea.Cmd { return m.react(msg, "") }})
	}
	m.showMenu("React", items)
}

func (m *Model) react(msg store.Message, emoji string) tea.Cmd {
	thread, ok := m.threadByID(msg.ThreadID)
	if m.reactor == nil || !ok {
		return func() tea.Msg { return reactedMsg{err: fmt.Errorf("reactions are unavailable")} }
	}
	return m.track(func() tea.Msg {
		return reactedMsg{threadID: thread.ID, err: m.reactor.React(m.ctx, thread, msg, emoji)}
	})
}

func ownReaction(msg store.Message, selfID int64) string {
	for _, r := range msg.Reactions {
		if r.ActorID == selfID {
			return r.Emoji
		}
	}
	return ""
}

func isQuickReaction(emoji string) bool {
	for _, q := range quickReactions {
		if q.emoji == emoji {
			return true
		}
	}
	return false
}

func reactionSummary(reactions []store.Reaction) string {
	var order []string
	counts := make(map[string]int)
	for _, r := range reactions {
		if counts[r.Emoji] == 0 {
			order = append(order, r.Emoji)
		}
		counts[r.Emoji]++
	}
	parts := make([]string, len(order))
	for i, emoji := range order {
		parts[i] = fmt.Sprintf("%s %d", emoji, counts[emoji])
	}
	return strings.Join(parts, "  ")
}
