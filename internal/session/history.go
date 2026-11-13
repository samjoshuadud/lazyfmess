package session

import (
	"fmt"
	"os"
	"path/filepath"
)

const HistoryDeviceFileName = "history-device.json"

func HistoryDevicePath(dataDir string) string {
	return filepath.Join(dataDir, HistoryDeviceFileName)
}

func LoadHistoryDevice(dataDir string) ([]byte, error) {
	data, err := os.ReadFile(HistoryDevicePath(dataDir))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read history device: %w", err)
	}
	return data, nil
}

func SaveHistoryDevice(dataDir string, data []byte) error {
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
