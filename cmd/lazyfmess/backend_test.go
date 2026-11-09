package main

import (
	"context"
	"testing"
	"time"

	"lazyfmess/internal/meta"
	"lazyfmess/internal/session"
)

func TestClientHolderWaitsForClient(t *testing.T) {
	t.Parallel()
	h := newClientHolder()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := h.wait(ctx); err == nil {
		t.Fatal("wait must block until a client is set")
	}
	client, err := meta.New(&session.Cookies{Platform: "messenger", Values: map[string]string{"c_user": "1", "xs": "x", "datr": "d"}}, t.TempDir(), nil, nil)
	if err != nil {
		t.Fatalf("meta.New: %v", err)
	}
	got := make(chan *meta.Client, 1)
	go func() {
		c, _ := h.wait(context.Background())
		got <- c
	}()
	h.set(client)
	if c := <-got; c != client || h.current() != client {
		t.Fatal("waiting caller did not receive the client")
	}
	h.clear()
	if h.current() != nil {
		t.Fatal("clear must drop the client")
	}
	h.set(client)
}
