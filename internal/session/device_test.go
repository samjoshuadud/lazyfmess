package session

import (
	"context"
	"testing"
)

func TestLoadOrCreateDeviceMarksEmptyStoreAsNew(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	container, err := OpenDeviceStore(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("OpenDeviceStore: %v", err)
	}
	defer container.Close()

	device, isNew, err := LoadOrCreateDevice(ctx, container)
	if err != nil {
		t.Fatalf("LoadOrCreateDevice: %v", err)
	}
	if !isNew {
		t.Fatal("expected new device for empty store")
	}
	if device == nil {
		t.Fatal("expected device")
	}
	if device.ID != nil {
		t.Fatal("new device should not have a jid yet")
	}
}
