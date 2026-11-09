package main

import (
	"context"
	"sync"

	"lazyfmess/internal/meta"
	"lazyfmess/internal/store"
	"lazyfmess/internal/ui"
)

type clientHolder struct {
	mu     sync.Mutex
	client *meta.Client
	ready  chan struct{}
}

func newClientHolder() *clientHolder {
	return &clientHolder{ready: make(chan struct{})}
}

func (h *clientHolder) set(c *meta.Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.client = c
	close(h.ready)
}

func (h *clientHolder) clear() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.client != nil {
		h.client = nil
		h.ready = make(chan struct{})
	}
}

func (h *clientHolder) current() *meta.Client {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.client
}

func (h *clientHolder) wait(ctx context.Context) (*meta.Client, error) {
	for {
		h.mu.Lock()
		client, ready := h.client, h.ready
		h.mu.Unlock()
		if client != nil {
			return client, nil
		}
		select {
		case <-ready:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

type backend struct {
	clients *clientHolder
	db      *store.Store
}

func (b backend) AccountID() int64 {
	if c := b.clients.current(); c != nil {
		return c.AccountID()
	}
	return 0
}

func (b backend) SendText(ctx context.Context, thread store.Thread, text string, replyTo store.Message) (ui.SendResult, error) {
	c, err := b.clients.wait(ctx)
	if err != nil {
		return ui.SendResult{}, err
	}
	result, err := c.SendText(ctx, thread, text, replyTo)
	return ui.SendResult{MessageID: result.MessageID, Timestamp: result.Timestamp}, err
}

func (b backend) SendFile(ctx context.Context, thread store.Thread, path string) (ui.SendResult, error) {
	c, err := b.clients.wait(ctx)
	if err != nil {
		return ui.SendResult{}, err
	}
	result, err := c.SendFile(ctx, thread, path)
	return ui.SendResult{MessageID: result.MessageID, Timestamp: result.Timestamp}, err
}

func (b backend) LoadHistory(ctx context.Context, thread store.Thread) error {
	c, err := b.clients.wait(ctx)
	if err != nil {
		return err
	}
	return c.SyncThreadHistory(ctx, b.db, thread)
}

func (b backend) NeedsPIN(ctx context.Context) (bool, error) {
	c, err := b.clients.wait(ctx)
	if err != nil {
		return false, err
	}
	return c.NeedsHistoryPIN(ctx)
}

func (b backend) Unlock(ctx context.Context, pin string) error {
	c, err := b.clients.wait(ctx)
	if err != nil {
		return err
	}
	return c.UnlockHistory(ctx, pin)
}

func (b backend) OpenMedia(ctx context.Context, msg store.Message) error {
	c, err := b.clients.wait(ctx)
	if err != nil {
		return err
	}
	return c.OpenMedia(ctx, msg)
}

func (b backend) MarkRead(ctx context.Context, thread store.Thread) error {
	c, err := b.clients.wait(ctx)
	if err != nil {
		return err
	}
	return c.MarkRead(ctx, b.db, thread)
}

func (b backend) SetTyping(ctx context.Context, thread store.Thread, active bool) error {
	c, err := b.clients.wait(ctx)
	if err != nil {
		return err
	}
	return c.SetTyping(ctx, thread, active)
}

func (b backend) React(ctx context.Context, thread store.Thread, msg store.Message, emoji string) error {
	c, err := b.clients.wait(ctx)
	if err != nil {
		return err
	}
	return c.React(ctx, b.db, thread, msg, emoji)
}

func (b backend) Edit(ctx context.Context, thread store.Thread, msg store.Message, text string) error {
	c, err := b.clients.wait(ctx)
	if err != nil {
		return err
	}
	return c.Edit(ctx, b.db, thread, msg, text)
}

func (b backend) Unsend(ctx context.Context, thread store.Thread, msg store.Message) error {
	c, err := b.clients.wait(ctx)
	if err != nil {
		return err
	}
	return c.Unsend(ctx, b.db, thread, msg)
}

func (b backend) SetMuted(ctx context.Context, thread store.Thread, muted bool) error {
	c, err := b.clients.wait(ctx)
	if err != nil {
		return err
	}
	return c.SetMuted(ctx, b.db, thread, muted)
}

func (b backend) SearchPeople(ctx context.Context, query string) ([]ui.PersonHit, error) {
	c, err := b.clients.wait(ctx)
	if err != nil {
		return nil, err
	}
	people, err := c.SearchPeople(ctx, query)
	if err != nil {
		return nil, err
	}
	hits := make([]ui.PersonHit, 0, len(people))
	for _, person := range people {
		hits = append(hits, ui.PersonHit{
			ID:         person.ID,
			Name:       person.Name,
			Context:    person.Context,
			ThreadType: person.ThreadType,
		})
	}
	return hits, nil
}

func (b backend) OpenPerson(ctx context.Context, person ui.PersonHit) (int64, error) {
	c, err := b.clients.wait(ctx)
	if err != nil {
		return 0, err
	}
	thread, err := c.OpenPerson(ctx, b.db, meta.Person{
		ID:         person.ID,
		Name:       person.Name,
		Context:    person.Context,
		ThreadType: person.ThreadType,
	})
	if err != nil {
		return 0, err
	}
	return thread.ID, nil
}
