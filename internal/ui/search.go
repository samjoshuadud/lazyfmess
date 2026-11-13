package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type searchResultMsg struct {
	query   string
	results []PersonHit
	err     error
}

type openPersonMsg struct {
	threadID int64
	err      error
}

type PersonHit struct {
	ID         int64
	Name       string
	Context    string
	ThreadType int64
}

type PersonSearcher interface {
	SearchPeople(ctx context.Context, query string) ([]PersonHit, error)
	OpenPerson(ctx context.Context, person PersonHit) (int64, error)
}

func newSearchInput() textinput.Model {
	input := textinput.New()
	input.Placeholder = "search people"
	input.CharLimit = 80
	input.Width = 36
	input.Prompt = "find> "
	input.PromptStyle = lipgloss.NewStyle().Foreground(accent).Bold(true)
	input.TextStyle = lipgloss.NewStyle().Foreground(text)
	input.PlaceholderStyle = lipgloss.NewStyle().Foreground(muted)
	input.Focus()
	return input
}

func (m *Model) openSearchModal() {
	m.showSearch = true
	m.searchErr = ""
	m.searchQuery = ""
	m.searchResults = nil
	m.searchIndex = 0
	m.searchInput = newSearchInput()
	m.composer.Blur()
}

func (m *Model) closeSearchModal() {
	m.showSearch = false
	m.searchErr = ""
	m.searchInput.Blur()
	m.applyFocus()
}

func (m Model) updateSearch(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case searchResultMsg:
		if msg.query != strings.TrimSpace(m.searchInput.Value()) {
			return m, nil
		}
		if msg.err != nil {
			m.searchErr = msg.err.Error()
			m.searchResults = nil
			m.status = msg.err.Error()
			return m, nil
		}
		m.searchErr = ""
		m.searchResults = msg.results
		m.searchIndex = 0
		if len(msg.results) == 0 {
			m.status = "no people found"
		} else {
			m.status = fmt.Sprintf("%d people found", len(msg.results))
		}
		return m, nil
	case openPersonMsg:
		if msg.err != nil {
			m.searchErr = msg.err.Error()
			m.status = msg.err.Error()
			return m, nil
		}
		m.closeSearchModal()
		m.selectedID = msg.threadID
		m.openedID = msg.threadID
		m.unreadSince = time.Time{}
		if thread, err := m.store.GetThread(m.ctx, msg.threadID); err == nil {
			m.unreadSince = unreadSince(thread)
		}
		m.setFocus(focusMessages)
		m.status = "opened chat"
		cmds := []tea.Cmd{m.loadThreads, m.loadMessages(msg.threadID)}
		if thread, err := m.store.GetThread(m.ctx, msg.threadID); err == nil {
			cmds = append(cmds, m.fetchHistory(thread))
		}
		return m, tea.Batch(cmds...)
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keys.ForceQuit):
			return m, tea.Quit
		case key.Matches(msg, m.keys.Skip):
			m.closeSearchModal()
			return m, nil
		case key.Matches(msg, m.keys.SearchUp):
			if m.searchIndex > 0 {
				m.searchIndex--
			}
			return m, nil
		case key.Matches(msg, m.keys.SearchDown):
			if m.searchIndex < len(m.searchResults)-1 {
				m.searchIndex++
			}
			return m, nil
		case key.Matches(msg, m.keys.SearchSubmit):
			query := strings.TrimSpace(m.searchInput.Value())
			if query == "" {
				m.searchErr = "type a name to search"
				return m, nil
			}
			if query == m.searchQuery && len(m.searchResults) > 0 {
				if m.searchIndex >= 0 && m.searchIndex < len(m.searchResults) {
					person := m.searchResults[m.searchIndex]
					m.status = "opening " + person.Name
					return m, m.track(m.openPerson(person))
				}
			}
			m.searchQuery = query
			m.searchErr = ""
			m.searchResults = nil
			m.searchIndex = 0
			m.status = "searching"
			return m, m.track(m.runSearch(query))
		}
	}
	var cmd tea.Cmd
	m.searchInput, cmd = m.searchInput.Update(msg)
	return m, cmd
}

func (m Model) runSearch(query string) tea.Cmd {
	return func() tea.Msg {
		if m.searcher == nil {
			return searchResultMsg{query: query, err: fmt.Errorf("people search is unavailable")}
		}
		results, err := m.searcher.SearchPeople(m.ctx, query)
		return searchResultMsg{query: query, results: results, err: err}
	}
}

func (m Model) openPerson(person PersonHit) tea.Cmd {
	return func() tea.Msg {
		if m.searcher == nil {
			return openPersonMsg{err: fmt.Errorf("people search is unavailable")}
		}
		threadID, err := m.searcher.OpenPerson(m.ctx, person)
		return openPersonMsg{threadID: threadID, err: err}
	}
}

func (m Model) renderSearchModal() string {
	width := 56
	if m.width > 0 && m.width < width+4 {
		width = m.width - 4
	}
	if width < 32 {
		width = 32
	}

	title := titleStyle.Render("Search people")
	hint := statusStyle.Render("Messenger people and groups. Enter searches, then Enter again opens.")
	body := m.searchInput.View()
	errLine := " "
	if m.searchErr != "" {
		errLine = errorStatusStyle.Render(m.searchErr)
	}

	var results strings.Builder
	if len(m.searchResults) == 0 {
		results.WriteString(statusStyle.Render("no results yet"))
	} else {
		maxRows := 8
		start := 0
		if m.searchIndex >= maxRows {
			start = m.searchIndex - maxRows + 1
		}
		end := start + maxRows
		if end > len(m.searchResults) {
			end = len(m.searchResults)
		}
		for i := start; i < end; i++ {
			item := m.searchResults[i]
			line := ansi.Truncate(item.Name+"  "+item.Context, width-6, "…")
			if i == m.searchIndex {
				results.WriteString(focusMark.Render("> " + line))
			} else {
				results.WriteString(statusStyle.Render("  " + line))
			}
			if i < end-1 {
				results.WriteByte('\n')
			}
		}
	}

	help := renderHints(m.contextBindings(), width-4)
	return modalStyle.Width(width).Render(
		lipgloss.JoinVertical(
			lipgloss.Left,
			title,
			"",
			hint,
			"",
			body,
			errLine,
			"",
			results.String(),
			"",
			help,
		),
	)
}
