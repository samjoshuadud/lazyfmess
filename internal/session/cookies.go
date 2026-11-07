package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const CookiesFileName = "cookies.json"

type Cookies struct {
	Platform string
	Values   map[string]string
}

type cookieFile struct {
	Platform string            `json:"platform"`
	Cookies  map[string]string `json:"cookies"`
}

func CookiesPath(dataDir string) string {
	return filepath.Join(dataDir, CookiesFileName)
}

func LoadCookies(dataDir string) (*Cookies, error) {
	path := CookiesPath(dataDir)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read cookies: %w", err)
	}
	var file cookieFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("parse cookies: %w", err)
	}
	switch file.Platform {
	case "messenger", "facebook", "facebook-tor", "messenger-lite", "messenger-lite-android":
	default:
		return nil, fmt.Errorf("parse cookies: unsupported platform %q", file.Platform)
	}
	if len(file.Cookies) == 0 {
		return nil, fmt.Errorf("parse cookies: cookie map is empty")
	}
	required := []string{"xs", "c_user", "datr"}
	for _, name := range required {
		if file.Cookies[name] == "" {
			return nil, fmt.Errorf("parse cookies: missing required cookie %q", name)
		}
	}
	return &Cookies{Platform: file.Platform, Values: file.Cookies}, nil
}
