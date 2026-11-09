package meta

import (
	"testing"

	"go.mau.fi/mautrix-meta/pkg/messagix/cookies"

	"lazyfmess/internal/session"
)

func TestPersistCookiesWritesRotatedValues(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	sess := &session.Cookies{Platform: "messenger", Values: map[string]string{"xs": "old", "c_user": "42", "datr": "d"}}
	if err := session.SaveCookies(dir, sess); err != nil {
		t.Fatalf("SaveCookies: %v", err)
	}
	c, err := New(sess, dir, nil, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	c.persistCookies()
	c.mx.GetCookies().UpdateValues(map[cookies.MetaCookieName]string{"xs": "rotated", "c_user": "42", "datr": "d"})
	c.persistCookies()
	got, err := session.LoadCookies(dir)
	if err != nil {
		t.Fatalf("LoadCookies: %v", err)
	}
	if got.Values["xs"] != "rotated" || got.Platform != "messenger" {
		t.Fatalf("saved cookies: %+v", got)
	}
	c.mx.GetCookies().UpdateValues(map[cookies.MetaCookieName]string{})
	c.persistCookies()
	if got, _ := session.LoadCookies(dir); got == nil || got.Values["xs"] != "rotated" {
		t.Fatal("logged-out cookies must not overwrite the saved session")
	}
}
