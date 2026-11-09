package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"lazyfmess/internal/store"
	"lazyfmess/internal/ui"
)

const demoSelf int64 = 100001

type demoContact struct {
	id   int64
	name string
}

func demoContacts() []demoContact {
	return []demoContact{
		{100002, "Maya Chen"},
		{100003, "Leo Martins"},
		{100004, "Priya Shah"},
		{100005, "Sam Okafor"},
	}
}

type demoMessage struct {
	sender  int64
	ago     time.Duration
	text    string
	replyTo int
	media   string
}

type demoThread struct {
	id       int64
	title    string
	group    bool
	unread   bool
	muted    bool
	seenBy   []int64
	messages []demoMessage
}

func demoThreads() []demoThread {
	return []demoThread{
		{id: 200001, title: "Maya Chen", unread: true, messages: []demoMessage{
			{sender: 100002, ago: 26 * time.Hour, text: "Did you finish the trail map?"},
			{sender: demoSelf, ago: 25 * time.Hour, text: "Almost, adding the water stops tonight"},
			{sender: 100002, ago: 40 * time.Minute, text: "Saturday still on? Weather says sunny all day"},
			{sender: 100002, ago: 38 * time.Minute, text: "I can drive if you bring snacks"},
		}},
		{id: 200002, title: "Weekend Hikers", group: true, unread: true, messages: []demoMessage{
			{sender: 100003, ago: 3 * time.Hour, text: "Who is in for the ridge loop?"},
			{sender: 100004, ago: 170 * time.Minute, text: "Me! Starting at 7?"},
			{sender: demoSelf, ago: 160 * time.Minute, text: "In. 7 works", replyTo: 1},
			{sender: 100003, ago: 2 * time.Hour, media: "image"},
			{sender: 100003, ago: 119 * time.Minute, text: "View from the top last time"},
			{sender: 100005, ago: 15 * time.Minute, text: "Bringing the good coffee this time"},
		}},
		{id: 200003, title: "Leo Martins", seenBy: []int64{100003}, messages: []demoMessage{
			{sender: 100003, ago: 50 * time.Hour, text: "Thanks for the book recommendation"},
			{sender: demoSelf, ago: 49 * time.Hour, text: "Tell me what you think of the ending"},
			{sender: 100003, ago: 5 * time.Hour, text: "Finished it. That twist!"},
			{sender: demoSelf, ago: 4 * time.Hour, text: "Right?? Next one is even better"},
		}},
		{id: 200004, title: "Priya Shah", muted: true, messages: []demoMessage{
			{sender: 100004, ago: 80 * time.Hour, text: "Sending the slides tonight"},
			{sender: demoSelf, ago: 79 * time.Hour, text: "Perfect, thank you"},
		}},
		{id: 200005, title: "Sam Okafor", messages: []demoMessage{
			{sender: 100005, ago: 8 * 24 * time.Hour, text: "Congrats on the new job!"},
			{sender: demoSelf, ago: 8 * 24 * time.Hour, text: "Thanks Sam, starting Monday"},
		}},
	}
}

func seedDemo(ctx context.Context, db *store.Store, now time.Time) error {
	for _, c := range demoContacts() {
		if err := db.UpsertContact(ctx, c.id, c.name); err != nil {
			return err
		}
	}
	for _, t := range demoThreads() {
		threadType := int64(1)
		if t.group {
			threadType = 2
		}
		ids := make([]string, len(t.messages))
		var last store.Message
		for i, dm := range t.messages {
			msg := store.Message{
				ID:        fmt.Sprintf("demo-%d-%d", t.id, i),
				ThreadID:  t.id,
				SenderID:  dm.sender,
				Text:      dm.text,
				Timestamp: now.Add(-dm.ago),
				MediaKind: dm.media,
			}
			if dm.media != "" {
				msg.MediaURL = "demo"
			}
			if dm.replyTo > 0 {
				msg.ReplyToID = ids[dm.replyTo]
			}
			ids[i] = msg.ID
			if err := db.UpsertMessage(ctx, msg); err != nil {
				return err
			}
			last = msg
		}
		preview := last.Text
		if preview == "" {
			preview = "sent a photo"
		}
		if err := db.UpsertThread(ctx, store.Thread{ID: t.id, Title: t.title, LastPreview: preview, ThreadType: threadType, UpdatedAt: last.Timestamp}); err != nil {
			return err
		}
		readAt := last.Timestamp
		if t.unread {
			readAt = last.Timestamp.Add(-time.Hour)
		}
		if err := db.SetLastRead(ctx, t.id, readAt); err != nil {
			return err
		}
		for _, reader := range t.seenBy {
			if err := db.SetParticipantRead(ctx, t.id, reader, now); err != nil {
				return err
			}
		}
		if t.muted {
			if err := db.SetMute(ctx, t.id, -1); err != nil {
				return err
			}
		}
	}
	reactions := []store.Reaction{
		{MessageID: "demo-200002-1", ActorID: 100003, Emoji: "👍"},
		{MessageID: "demo-200002-4", ActorID: 100004, Emoji: "❤"},
		{MessageID: "demo-200002-4", ActorID: 100005, Emoji: "❤"},
		{MessageID: "demo-200003-2", ActorID: demoSelf, Emoji: "😆"},
	}
	for _, r := range reactions {
		if err := db.SetReaction(ctx, r); err != nil {
			return err
		}
	}
	return nil
}

type demoBackend struct {
	db  *store.Store
	app *atomic.Pointer[ui.App]
	seq atomic.Int64
}

func (d *demoBackend) AccountID() int64 { return demoSelf }

func (d *demoBackend) SendText(ctx context.Context, thread store.Thread, text string, _ store.Message) (ui.SendResult, error) {
	go d.reply(context.WithoutCancel(ctx), thread)
	return d.result(), nil
}

func (d *demoBackend) SendFile(context.Context, store.Thread, string) (ui.SendResult, error) {
	return d.result(), nil
}

func (d *demoBackend) result() ui.SendResult {
	return ui.SendResult{MessageID: fmt.Sprintf("demo-sent-%d", d.seq.Add(1)), Timestamp: time.Now()}
}

func (d *demoBackend) reply(ctx context.Context, thread store.Thread) {
	sender := demoContacts()[0].id
	for _, c := range demoContacts() {
		if c.name == thread.Title {
			sender = c.id
		}
	}
	time.Sleep(time.Second)
	app := d.app.Load()
	app.Typing(thread.ID, sender, true)
	time.Sleep(2 * time.Second)
	now := time.Now()
	msg := store.Message{ID: fmt.Sprintf("demo-reply-%d", d.seq.Add(1)), ThreadID: thread.ID, SenderID: sender, Text: "Sounds great, see you there!", Timestamp: now}
	if d.db.UpsertMessage(ctx, msg) == nil {
		thread.LastPreview, thread.UpdatedAt = msg.Text, now
		_ = d.db.UpsertThread(ctx, thread)
	}
	app.Typing(thread.ID, sender, false)
	app.Notify()
}

func (d *demoBackend) LoadHistory(context.Context, store.Thread) error { return nil }
func (d *demoBackend) NeedsPIN(context.Context) (bool, error)          { return false, nil }
func (d *demoBackend) Unlock(context.Context, string) error            { return nil }
func (d *demoBackend) OpenMedia(context.Context, store.Message) error  { return nil }
func (d *demoBackend) SetTyping(context.Context, store.Thread, bool) error {
	return nil
}

func (d *demoBackend) MarkRead(ctx context.Context, thread store.Thread) error {
	return d.db.SetLastRead(ctx, thread.ID, thread.UpdatedAt)
}

func (d *demoBackend) React(ctx context.Context, _ store.Thread, msg store.Message, emoji string) error {
	return d.db.SetReaction(ctx, store.Reaction{MessageID: msg.ID, ActorID: demoSelf, Emoji: emoji})
}

func (d *demoBackend) Edit(ctx context.Context, _ store.Thread, msg store.Message, text string) error {
	return d.db.EditMessage(ctx, msg.ID, text)
}

func (d *demoBackend) Unsend(ctx context.Context, _ store.Thread, msg store.Message) error {
	return d.db.UnsendMessage(ctx, msg.ID)
}

func (d *demoBackend) SetMuted(ctx context.Context, thread store.Thread, muted bool) error {
	var until int64
	if muted {
		until = -1
	}
	return d.db.SetMute(ctx, thread.ID, until)
}

func (d *demoBackend) SearchPeople(_ context.Context, query string) ([]ui.PersonHit, error) {
	var hits []ui.PersonHit
	for _, c := range demoContacts() {
		if strings.Contains(strings.ToLower(c.name), strings.ToLower(query)) {
			hits = append(hits, ui.PersonHit{ID: c.id, Name: c.name, Context: "Friend", ThreadType: 1})
		}
	}
	return hits, nil
}

func (d *demoBackend) OpenPerson(ctx context.Context, person ui.PersonHit) (int64, error) {
	for _, t := range demoThreads() {
		if t.title == person.Name {
			return t.id, nil
		}
	}
	return person.ID, d.db.UpsertThread(ctx, store.Thread{ID: person.ID, Title: person.Name, ThreadType: 1, UpdatedAt: time.Now()})
}

func runDemo(ctx context.Context) error {
	dir, err := os.MkdirTemp("", "lazyfmess-demo-*")
	if err != nil {
		return fmt.Errorf("create demo directory: %w", err)
	}
	defer os.RemoveAll(dir)
	db, err := store.Open(ctx, dir)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := seedDemo(ctx, db, time.Now()); err != nil {
		return fmt.Errorf("seed demo data: %w", err)
	}
	var appRef atomic.Pointer[ui.App]
	b := &demoBackend{db: db, app: &appRef}
	app := ui.NewApp(ctx, db, ui.Services{
		Sender:   b,
		History:  b,
		Unlocker: b,
		Searcher: b,
		Media:    b,
		Reader:   b,
		Typing:   b,
		Reactor:  b,
		Editor:   b,
		Muter:    b,
	}, ui.Config{})
	appRef.Store(app)
	return app.Run()
}
