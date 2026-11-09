package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadCookies(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		wantErr bool
	}{
		{
			name: "valid messenger cookies",
			content: `{
				"platform": "messenger",
				"cookies": {
					"xs": "session-token",
					"c_user": "1234567890",
					"datr": "device-token"
				}
			}`,
		},
		{
			name: "valid facebook cookies",
			content: `{
				"platform": "facebook",
				"cookies": {
					"xs": "session-token",
					"c_user": "1234567890",
					"datr": "device-token"
				}
			}`,
		},
		{
			name: "missing required cookie",
			content: `{
				"platform": "messenger",
				"cookies": {
					"xs": "session-token",
					"c_user": "1234567890"
				}
			}`,
			wantErr: true,
		},
		{
			name: "unsupported platform",
			content: `{
				"platform": "instagram",
				"cookies": {
					"sessionid": "x",
					"csrftoken": "y",
					"ds_user_id": "1"
				}
			}`,
			wantErr: true,
		},
		{
			name:    "invalid json",
			content: `{`,
			wantErr: true,
		},
		{
			name: "empty cookie map",
			content: `{
				"platform": "messenger",
				"cookies": {}
			}`,
			wantErr: true,
		},
		{
			name: "browser cookie array",
			content: `[
				{"name":"xs","value":"session-token","domain":".messenger.com"},
				{"name":"c_user","value":"1234567890","domain":".messenger.com"},
				{"name":"datr","value":"device-token","domain":".messenger.com"}
			]`,
		},
		{
			name: "browser cookie array facebook domain",
			content: `[
				{"name":"xs","value":"session-token","domain":".facebook.com"},
				{"name":"c_user","value":"1234567890","domain":".facebook.com"},
				{"name":"datr","value":"device-token","domain":".facebook.com"}
			]`,
		},
		{
			name:    "empty browser cookie array",
			content: `[]`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			path := filepath.Join(dir, CookiesFileName)
			if err := os.WriteFile(path, []byte(tt.content), 0o600); err != nil {
				t.Fatalf("write fixture: %v", err)
			}
			got, err := LoadCookies(dir)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadCookies: %v", err)
			}
			if got.Platform == "" || got.Values["xs"] == "" {
				t.Fatal("expected platform and session cookie")
			}
			if tt.name == "browser cookie array facebook domain" && got.Platform != "facebook" {
				t.Fatalf("platform: got %q want facebook", got.Platform)
			}
			if tt.name == "browser cookie array" && got.Platform != "messenger" {
				t.Fatalf("platform: got %q want messenger", got.Platform)
			}
		})
	}
}

func TestLoadCookiesMissingFile(t *testing.T) {
	t.Parallel()
	_, err := LoadCookies(t.TempDir())
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestSaveCookiesRoundTrip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	array := `[{"name":"xs","value":"x1","domain":".messenger.com"},{"name":"c_user","value":"42","domain":".messenger.com"},{"name":"datr","value":"d1","domain":".messenger.com"}]`
	if err := os.WriteFile(CookiesPath(dir), []byte(array), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	loaded, err := LoadCookies(dir)
	if err != nil {
		t.Fatalf("LoadCookies: %v", err)
	}
	loaded.Values["xs"] = "x2"
	if err := SaveCookies(dir, loaded); err != nil {
		t.Fatalf("SaveCookies: %v", err)
	}
	info, err := os.Stat(CookiesPath(dir))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("cookies file mode: %v %v", info.Mode(), err)
	}
	data, _ := os.ReadFile(CookiesPath(dir))
	if !strings.HasPrefix(strings.TrimSpace(string(data)), "{") {
		t.Fatalf("saved file should use the object form: %s", data)
	}
	again, err := LoadCookies(dir)
	if err != nil {
		t.Fatalf("LoadCookies after save: %v", err)
	}
	if again.Platform != "messenger" || again.Values["xs"] != "x2" || again.Values["c_user"] != "42" {
		t.Fatalf("round trip: %+v", again)
	}
}
