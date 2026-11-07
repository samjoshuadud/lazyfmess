package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const StateFileName = "messagix-state.json"

func StatePath(dataDir string) string {
	return filepath.Join(dataDir, StateFileName)
}

func LoadState(dataDir string) (json.RawMessage, error) {
	data, err := os.ReadFile(StatePath(dataDir))
	if err != nil {
		return nil, fmt.Errorf("read state: %w", err)
	}
	if !json.Valid(data) {
		return nil, fmt.Errorf("read state: invalid json")
	}
	return json.RawMessage(data), nil
}

func SaveState(dataDir string, state json.RawMessage) error {
	if len(state) == 0 {
		return nil
	}
	path := StatePath(dataDir)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, state, 0o600); err != nil {
		return fmt.Errorf("write state: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename state: %w", err)
	}
	return nil
}
