package meta

import (
	"errors"
	"fmt"
	"testing"

	"go.mau.fi/mautrix-meta/pkg/messagix/cookies"
	"go.mau.fi/mautrix-meta/pkg/messagix/httpclient"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"

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

func TestNativeSessionConversionAndRefreshDecision(t *testing.T) {
	t.Parallel()
	saved := &session.NativeSession{
		AccessToken:    "token",
		AppID:          "app",
		DeviceID:       "6f0f8a4e-2b7d-4b8e-9c3a-1d2e3f405162",
		FamilyDeviceID: "0b1c2d3e-4f50-4617-8293-a4b5c6d7e8f9",
	}
	native, err := toMessagixNative(saved)
	if err != nil {
		t.Fatalf("toMessagixNative: %v", err)
	}
	if back := fromMessagixNative(native); !equalNative(back, saved) {
		t.Fatalf("round trip: %+v", back)
	}
	if _, err := toMessagixNative(&session.NativeSession{DeviceID: "nope"}); err == nil {
		t.Fatal("invalid device id must fail")
	}
	expired := fmt.Errorf("load messages page: %w", httpclient.ErrTokenInvalidated)
	tests := []struct {
		name   string
		err    error
		native *types.NativeSession
		want   bool
	}{
		{"expired with saved login", expired, native, true},
		{"expired cookies only", expired, nil, false},
		{"other error", errors.New("network down"), native, false},
		{"connected", nil, native, false},
	}
	for _, tt := range tests {
		if got := shouldRefreshCookies(tt.err, tt.native); got != tt.want {
			t.Errorf("%s: got %v", tt.name, got)
		}
	}
}

func TestNativeSessionIsSavedWithCookies(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	sess := &session.Cookies{
		Platform: "messenger-lite-android",
		Values:   map[string]string{"xs": "x", "c_user": "42", "datr": "d"},
		NativeSession: &session.NativeSession{
			AccessToken: "token", AppID: "app",
			DeviceID: "6f0f8a4e-2b7d-4b8e-9c3a-1d2e3f405162", FamilyDeviceID: "0b1c2d3e-4f50-4617-8293-a4b5c6d7e8f9",
		},
	}
	if err := session.SaveCookies(dir, sess); err != nil {
		t.Fatalf("SaveCookies: %v", err)
	}
	loaded, err := session.LoadCookies(dir)
	if err != nil {
		t.Fatalf("LoadCookies: %v", err)
	}
	c, err := New(loaded, dir, nil, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	c.mx.GetCookies().UpdateValues(map[cookies.MetaCookieName]string{"xs": "y", "c_user": "42", "datr": "d"})
	c.persistCookies()
	again, err := session.LoadCookies(dir)
	if err != nil {
		t.Fatalf("LoadCookies: %v", err)
	}
	if again.Values["xs"] != "y" || !equalNative(again.NativeSession, sess.NativeSession) {
		t.Fatalf("saved: %+v native %+v", again.Values, again.NativeSession)
	}
}
