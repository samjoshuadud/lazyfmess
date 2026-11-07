package session

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

type browserCookie struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Domain string `json:"domain"`
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
	file, err := parseCookieFile(data)
	if err != nil {
		return nil, err
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

func parseCookieFile(data []byte) (cookieFile, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return cookieFile{}, fmt.Errorf("parse cookies: file is empty")
	}
	if trimmed[0] == '[' {
		return parseBrowserCookieArray(trimmed)
	}
	var file cookieFile
	if err := json.Unmarshal(trimmed, &file); err != nil {
		return cookieFile{}, fmt.Errorf("parse cookies: %w", err)
	}
	return file, nil
}

func parseBrowserCookieArray(data []byte) (cookieFile, error) {
	var items []browserCookie
	if err := json.Unmarshal(data, &items); err != nil {
		return cookieFile{}, fmt.Errorf("parse cookies: %w", err)
	}
	if len(items) == 0 {
		return cookieFile{}, fmt.Errorf("parse cookies: cookie array is empty")
	}
	values := make(map[string]string, len(items))
	platform := ""
	for _, item := range items {
		if item.Name == "" {
			continue
		}
		values[item.Name] = item.Value
		if platform == "" {
			platform = platformFromDomain(item.Domain)
		}
	}
	if platform == "" {
		platform = "messenger"
	}
	return cookieFile{Platform: platform, Cookies: values}, nil
}

func platformFromDomain(domain string) string {
	d := strings.ToLower(strings.TrimPrefix(domain, "."))
	switch {
	case strings.Contains(d, "messenger.com"):
		return "messenger"
	case strings.Contains(d, "facebook.com"):
		return "facebook"
	default:
		return ""
	}
}
