package session

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/pbkdf2"
	"crypto/sha1"
	"crypto/sha256"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func chromiumEncrypt(t *testing.T, value, host string) []byte {
	t.Helper()
	key, err := pbkdf2.Key(sha1.New, "peanuts", []byte("saltysalt"), 1, 16)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256([]byte(host))
	plain := append(hash[:], value...)
	pad := aes.BlockSize - len(plain)%aes.BlockSize
	plain = append(plain, bytes.Repeat([]byte{byte(pad)}, pad)...)
	block, _ := aes.NewCipher(key)
	out := make([]byte, len(plain))
	cipher.NewCBCEncrypter(block, bytes.Repeat([]byte{' '}, aes.BlockSize)).CryptBlocks(out, plain)
	return append([]byte("v10"), out...)
}

func makeDB(t *testing.T, path string, stmts ...string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
}

func TestImportBrowserCookies(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("browser import is linux only")
	}
	t.Parallel()
	home := t.TempDir()
	makeDB(t, filepath.Join(home, ".mozilla/firefox/abc.default/cookies.sqlite"),
		`CREATE TABLE moz_cookies (name TEXT, value TEXT, host TEXT)`,
		`INSERT INTO moz_cookies VALUES ('c_user','42','.messenger.com'),('xs','fx','.messenger.com'),('datr','d','.messenger.com'),('sid','x','.example.com')`,
	)
	chromePath := filepath.Join(home, ".config/chromium/Default/Network/Cookies")
	makeDB(t, chromePath,
		`CREATE TABLE meta (key TEXT, value TEXT)`,
		`INSERT INTO meta VALUES ('version','24')`,
		`CREATE TABLE cookies (name TEXT, value TEXT, encrypted_value BLOB, host_key TEXT)`,
	)
	db, _ := sql.Open("sqlite", chromePath)
	for name, value := range map[string]string{"c_user": "77", "xs": "chrome-xs", "datr": "cd"} {
		if _, err := db.Exec(`INSERT INTO cookies VALUES (?, '', ?, '.facebook.com')`, name, chromiumEncrypt(t, value, ".facebook.com")); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()

	stores := FindBrowserStores(home)
	if len(stores) != 2 {
		t.Fatalf("stores: %+v", stores)
	}
	want := map[browserKind]struct{ platform, user, xs string }{
		firefoxStore:  {"messenger", "42", "fx"},
		chromiumStore: {"facebook", "77", "chrome-xs"},
	}
	for _, store := range stores {
		got, err := ImportBrowserCookies(context.Background(), store)
		if err != nil {
			t.Fatalf("%s: %v", store.Name, err)
		}
		w := want[store.kind]
		if got.Platform != w.platform || got.Values["c_user"] != w.user || got.Values["xs"] != w.xs {
			t.Errorf("%s: got %+v", store.Name, got)
		}
	}
}

func TestDecryptChromiumKeyringValue(t *testing.T) {
	t.Parallel()
	if _, err := decryptChromiumValue([]byte("v11abc"), false); !errors.Is(err, errKeyringCookies) {
		t.Fatalf("v11: %v", err)
	}
	if _, err := cookiesFromBrowserRows("Test", []browserCookie{{Name: "c_user", Value: "1", Domain: ".messenger.com"}}); err == nil {
		t.Fatal("missing xs must not import")
	}
}
