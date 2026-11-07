package session

import (
	"context"
	"fmt"
	"path/filepath"

	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	_ "modernc.org/sqlite"
)

const DeviceDBFileName = "device.db"

func DeviceDBPath(dataDir string) string {
	return filepath.Join(dataDir, DeviceDBFileName)
}

func OpenDeviceStore(ctx context.Context, dataDir string) (*sqlstore.Container, error) {
	path := DeviceDBPath(dataDir)
	dsn := fmt.Sprintf("file:%s?_pragma=foreign_keys(1)", path)
	container, err := sqlstore.New(ctx, "sqlite", dsn, nil)
	if err != nil {
		return nil, fmt.Errorf("open device store: %w", err)
	}
	return container, nil
}

func LoadOrCreateDevice(ctx context.Context, container *sqlstore.Container) (*store.Device, bool, error) {
	device, err := container.GetFirstDevice(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("get device: %w", err)
	}
	if device != nil {
		return device, false, nil
	}
	return container.NewDevice(), true, nil
}
