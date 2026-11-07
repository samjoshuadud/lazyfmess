package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"go.mau.fi/mautrix-meta/pkg/messagix/securehistory"
)

const HistoryDeviceFileName = "history-device.json"

func HistoryDevicePath(dataDir string) string {
	return filepath.Join(dataDir, HistoryDeviceFileName)
}

func LoadHistoryDevice(dataDir string) (*securehistory.Device, error) {
	data, err := os.ReadFile(HistoryDevicePath(dataDir))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read history device: %w", err)
	}
	var device securehistory.Device
	if err := json.Unmarshal(data, &device); err != nil {
		return nil, fmt.Errorf("parse history device: %w", err)
	}
	if device.ID == "" || device.BackupID == "" {
		return nil, nil
	}
	return &device, nil
}

func SaveHistoryDevice(dataDir string, device *securehistory.Device) error {
	if device == nil || device.ID == "" {
		return nil
	}
	data, err := json.Marshal(device)
	if err != nil {
		return fmt.Errorf("marshal history device: %w", err)
	}
	path := HistoryDevicePath(dataDir)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write history device: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename history device: %w", err)
	}
	return nil
}
