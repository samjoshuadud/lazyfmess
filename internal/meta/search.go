package meta

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"go.mau.fi/mautrix-meta/pkg/messagix/socket"
	"go.mau.fi/mautrix-meta/pkg/messagix/table"

	"lazyfmess/internal/store"
)

type Person struct {
	ID         int64
	Name       string
	Context    string
	ThreadType int64
}

func (c *Client) SearchPeople(ctx context.Context, query string) ([]Person, error) {
	if err := c.waitReady(ctx); err != nil {
		return nil, err
	}
	if c == nil || c.mx == nil {
		return nil, fmt.Errorf("search people: client is nil")
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	task := &socket.SearchUserTask{
		Query: query,
		SupportedTypes: []table.SearchType{
			table.SearchTypeContact,
			table.SearchTypeNonContact,
			table.SearchTypeGroup,
			table.SearchTypePage,
			table.SearchTypeIGContactFollowing,
			table.SearchTypeIGContactNonFollowing,
			table.SearchTypeIGNonContactFollowing,
			table.SearchTypeIGNonContactNonFollowing,
			table.SearchTypeCommunityMessagingThread,
		},
		SurfaceType: 5,
	}
	secondary := *task
	secondary.Secondary = true
	go func() {
		_, _ = c.mx.ExecuteTasks(ctx, &secondary)
	}()
	tbl, err := c.mx.ExecuteTasks(ctx, task)
	if err != nil {
		return nil, fmt.Errorf("search people: %w", err)
	}
	if tbl == nil {
		return nil, nil
	}
	out := make([]Person, 0, len(tbl.LSInsertSearchResult))
	seen := make(map[int64]struct{}, len(tbl.LSInsertSearchResult))
	for _, result := range tbl.LSInsertSearchResult {
		if result == nil {
			continue
		}
		person, ok := personFromSearch(result)
		if !ok {
			continue
		}
		if _, exists := seen[person.ID]; exists {
			continue
		}
		seen[person.ID] = struct{}{}
		out = append(out, person)
	}
	return out, nil
}

func (c *Client) OpenPerson(ctx context.Context, s *store.Store, person Person) (store.Thread, error) {
	if err := c.waitReady(ctx); err != nil {
		return store.Thread{}, err
	}
	if c == nil || c.mx == nil || s == nil || person.ID == 0 {
		return store.Thread{}, fmt.Errorf("open person: missing client, store, or person")
	}
	if err := s.UpsertContact(ctx, person.ID, person.Name); err != nil {
		return store.Thread{}, err
	}
	threadType := person.ThreadType
	if threadType == 0 {
		threadType = int64(table.ONE_TO_ONE)
	}
	thread := store.Thread{
		ID:         person.ID,
		Title:      person.Name,
		ThreadType: threadType,
		UpdatedAt:  time.Now(),
	}
	if err := s.UpsertThread(ctx, thread); err != nil {
		return store.Thread{}, err
	}
	tbl, err := c.mx.ExecuteTasks(ctx, &socket.CreateThreadTask{
		ThreadFBID: person.ID,
		SyncGroup:  1,
	})
	if err != nil {
		return thread, fmt.Errorf("open person thread: %w", err)
	}
	if err := ApplyTable(ctx, s, tbl); err != nil {
		return thread, err
	}
	if stored, err := s.GetThread(ctx, person.ID); err == nil {
		if stored.Title == "" && person.Name != "" {
			stored.Title = person.Name
			_ = s.UpsertThread(ctx, stored)
		}
		return stored, nil
	}
	return thread, nil
}

func personFromSearch(result *table.LSInsertSearchResult) (Person, bool) {
	switch result.Type_ {
	case table.SearchTypeContact, table.SearchTypeNonContact, table.SearchTypePage,
		table.SearchTypeIGContactFollowing, table.SearchTypeIGContactNonFollowing,
		table.SearchTypeIGNonContactFollowing, table.SearchTypeIGNonContactNonFollowing,
		table.SearchTypeAIBot:
	case table.SearchTypeGroup, table.SearchTypeCommunityMessagingThread, table.SearchTypeTAMThread:
		id, err := strconv.ParseInt(result.ResultId, 10, 64)
		if err != nil || id == 0 || result.DisplayName == "" {
			return Person{}, false
		}
		return Person{
			ID:         id,
			Name:       result.DisplayName,
			Context:    firstNonEmpty(result.ContextLine, result.SecondaryContextLine, "group"),
			ThreadType: int64(result.ThreadType),
		}, true
	default:
		return Person{}, false
	}
	canMessage := result.CanViewerMessage || (result.OtherUserId != 0 && result.BlockedByViewerStatus == 0)
	id := result.GetFBID()
	if id == 0 || result.DisplayName == "" || (!canMessage && !result.ThreadType.IsOneToOne()) {
		return Person{}, false
	}
	if !result.ThreadType.IsOneToOne() && result.ThreadType != table.UNKNOWN_THREAD_TYPE {
		return Person{}, false
	}
	return Person{
		ID:         id,
		Name:       result.DisplayName,
		Context:    firstNonEmpty(result.ContextLine, result.SecondaryContextLine, "person"),
		ThreadType: int64(table.ONE_TO_ONE),
	}, true
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
