package session

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/pbkdf2"
	"crypto/sha1"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	_ "modernc.org/sqlite"
)

type browserKind int

const (
	firefoxStore browserKind = iota
	chromiumStore
)

type BrowserStore struct {
	Name string
	Path string
	kind browserKind
}

var errKeyringCookies = errors.New("this browser keeps its cookies in the system keyring, which lazyfmess cannot read yet; paste a cookie export instead")

func FindBrowserStores(home string) []BrowserStore {
	if runtime.GOOS != "linux" {
		return nil
	}
	var stores []BrowserStore
	firefoxRoots := []string{
		filepath.Join(home, ".mozilla/firefox"),
		filepath.Join(home, "snap/firefox/common/.mozilla/firefox"),
		filepath.Join(home, ".var/app/org.mozilla.firefox/.mozilla/firefox"),
	}
	for _, root := range firefoxRoots {
		matches, _ := filepath.Glob(filepath.Join(root, "*", "cookies.sqlite"))
		for _, path := range matches {
			stores = append(stores, BrowserStore{Name: "Firefox (" + filepath.Base(filepath.Dir(path)) + ")", Path: path, kind: firefoxStore})
		}
	}
	chromiumRoots := []struct{ name, dir string }{
		{"Chromium", ".config/chromium"},
		{"Google Chrome", ".config/google-chrome"},
		{"Brave", ".config/BraveSoftware/Brave-Browser"},
		{"Microsoft Edge", ".config/microsoft-edge"},
		{"Vivaldi", ".config/vivaldi"},
	}
	for _, root := range chromiumRoots {
		for _, rel := range []string{"*/Network/Cookies", "*/Cookies"} {
			matches, _ := filepath.Glob(filepath.Join(home, root.dir, rel))
			for _, path := range matches {
				profile := filepath.Base(filepath.Dir(path))
				if profile == "Network" {
					profile = filepath.Base(filepath.Dir(filepath.Dir(path)))
				}
				stores = append(stores, BrowserStore{Name: root.name + " (" + profile + ")", Path: path, kind: chromiumStore})
			}
		}
	}
	return stores
}

func ImportBrowserCookies(ctx context.Context, store BrowserStore) (*Cookies, error) {
	dir, err := os.MkdirTemp("", "lazyfmess-cookies-*")
	if err != nil {
		return nil, fmt.Errorf("import cookies: %w", err)
	}
	defer os.RemoveAll(dir)
	copyPath := filepath.Join(dir, "cookies.db")
	for _, suffix := range []string{"", "-wal"} {
		if err := copyFile(store.Path+suffix, copyPath+suffix); err != nil && (suffix == "" || !errors.Is(err, os.ErrNotExist)) {
			return nil, fmt.Errorf("import cookies: %w", err)
		}
	}
	db, err := sql.Open("sqlite", "file:"+copyPath+"?mode=ro")
	if err != nil {
		return nil, fmt.Errorf("import cookies: %w", err)
	}
	defer db.Close()
	var rows []browserCookie
	switch store.kind {
	case firefoxStore:
		rows, err = readFirefoxCookies(ctx, db)
	default:
		rows, err = readChromiumCookies(ctx, db)
	}
	if err != nil {
		return nil, fmt.Errorf("import cookies from %s: %w", store.Name, err)
	}
	return cookiesFromBrowserRows(store.Name, rows)
}

func cookiesFromBrowserRows(name string, rows []browserCookie) (*Cookies, error) {
	for _, platform := range []string{"messenger", "facebook"} {
		values := make(map[string]string)
		for _, row := range rows {
			if platformFromDomain(row.Domain) == platform {
				values[row.Name] = row.Value
			}
		}
		if values["c_user"] != "" && values["xs"] != "" {
			return cookiesFromFile(cookieFile{Platform: platform, Cookies: values})
		}
	}
	return nil, fmt.Errorf("import cookies from %s: no logged-in messenger.com or facebook.com session found", name)
}

func readFirefoxCookies(ctx context.Context, db *sql.DB) ([]browserCookie, error) {
	rows, err := db.QueryContext(ctx, `
SELECT name, value, host FROM moz_cookies
WHERE host LIKE '%messenger.com' OR host LIKE '%facebook.com'
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []browserCookie
	for rows.Next() {
		var c browserCookie
		if err := rows.Scan(&c.Name, &c.Value, &c.Domain); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func readChromiumCookies(ctx context.Context, db *sql.DB) ([]browserCookie, error) {
	var version int
	_ = db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key = 'version'`).Scan(&version)
	rows, err := db.QueryContext(ctx, `
SELECT name, value, encrypted_value, host_key FROM cookies
WHERE host_key LIKE '%messenger.com' OR host_key LIKE '%facebook.com'
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []browserCookie
	for rows.Next() {
		var c browserCookie
		var encrypted []byte
		if err := rows.Scan(&c.Name, &c.Value, &encrypted, &c.Domain); err != nil {
			return nil, err
		}
		if c.Value == "" && len(encrypted) > 0 {
			value, err := decryptChromiumValue(encrypted, version >= 24)
			if err != nil {
				return nil, err
			}
			c.Value = value
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func decryptChromiumValue(encrypted []byte, hasDomainHash bool) (string, error) {
	switch {
	case bytes.HasPrefix(encrypted, []byte("v11")):
		return "", errKeyringCookies
	case !bytes.HasPrefix(encrypted, []byte("v10")):
		return "", fmt.Errorf("unknown cookie encryption")
	}
	key, err := pbkdf2.Key(sha1.New, "peanuts", []byte("saltysalt"), 1, 16)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	data := encrypted[3:]
	if len(data) == 0 || len(data)%aes.BlockSize != 0 {
		return "", fmt.Errorf("malformed encrypted cookie")
	}
	plain := make([]byte, len(data))
	cipher.NewCBCDecrypter(block, bytes.Repeat([]byte{' '}, aes.BlockSize)).CryptBlocks(plain, data)
	pad := int(plain[len(plain)-1])
	if pad == 0 || pad > aes.BlockSize || pad > len(plain) {
		return "", fmt.Errorf("malformed encrypted cookie")
	}
	plain = plain[:len(plain)-pad]
	if hasDomainHash {
		if len(plain) < 32 {
			return "", fmt.Errorf("malformed encrypted cookie")
		}
		plain = plain[32:]
	}
	return string(plain), nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
