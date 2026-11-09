package ui

import (
	"context"
	"testing"
)

type stubUnlocker struct {
	needs bool
	pin   string
}

func (s *stubUnlocker) NeedsPIN(context.Context) (bool, error) { return s.needs, nil }
func (s *stubUnlocker) Unlock(_ context.Context, pin string) error {
	s.pin = pin
	return nil
}

func TestUnlockKeyOpensPINPrompt(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		needs    bool
		wantOpen bool
	}{
		{"locked backup", true, true},
		{"already unlocked", false, false},
	}
	for _, tt := range tests {
		m := newTestModel(t, 1, 0)
		unlocker := &stubUnlocker{needs: tt.needs}
		m.unlocker = unlocker
		synced := 0
		m.backgroundSync = func(context.Context) error { synced++; return nil }
		m.syncStarted = true
		m = drain(t, m, keyPress("U"))
		if m.showPIN != tt.wantOpen {
			t.Fatalf("%s: showPIN = %v", tt.name, m.showPIN)
		}
		if !tt.wantOpen {
			if m.toast == "" {
				t.Fatalf("%s: expected a notice", tt.name)
			}
			continue
		}
		m.pinInput.SetValue("123456")
		m = drain(t, m, keyPress("enter"))
		if unlocker.pin != "123456" || m.showPIN {
			t.Fatalf("%s: pin %q showPIN %v", tt.name, unlocker.pin, m.showPIN)
		}
		if synced != 1 {
			t.Fatalf("%s: history sync should run again after unlock, ran %d", tt.name, synced)
		}
	}
}
