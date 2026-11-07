package session

import (
	"os"
	"path/filepath"
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
