package auth

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTempFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "auth.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	return path
}

func TestAuthStoreVerify(t *testing.T) {
	store := &AuthStore{users: map[string]string{"alice": "secret"}}
	if !store.Authenticate("alice", "secret") {
		t.Fatalf("expected valid credentials")
	}
	if store.Authenticate("alice", "wrong") {
		t.Fatalf("expected invalid password")
	}
	if store.Authenticate("bob", "secret") {
		t.Fatalf("expected unknown user to fail")
	}
}

func TestLoadFromJSONFileSuccess(t *testing.T) {
	path := writeTempFile(t, `[{"user":"alice","pass":"a"},{"user":"bob","pass":"b"}]`)
	store, err := LoadFromJSONFile(path)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !store.Authenticate("alice", "a") || !store.Authenticate("bob", "b") {
		t.Fatalf("expected loaded users to verify")
	}
}

func TestLoadFromJSONFileSkipsEmptyUser(t *testing.T) {
	path := writeTempFile(t, `[{"user":"","pass":"x"},{"user":"ok","pass":"p"}]`)
	store, err := LoadFromJSONFile(path)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if store.Authenticate("", "x") {
		t.Fatalf("expected empty user to be skipped")
	}
	if !store.Authenticate("ok", "p") {
		t.Fatalf("expected valid user to be kept")
	}
}

func TestLoadFromJSONFileReadError(t *testing.T) {
	_, err := LoadFromJSONFile(filepath.Join(t.TempDir(), "missing.json"))
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "read auth file") {
		t.Fatalf("expected read auth file error, got %v", err)
	}
}

func TestLoadFromJSONFileParseError(t *testing.T) {
	path := writeTempFile(t, "{")
	_, err := LoadFromJSONFile(path)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "parse auth file") {
		t.Fatalf("expected parse auth file error, got %v", err)
	}
}

func TestLoadFromJSONFileEmpty(t *testing.T) {
	path := writeTempFile(t, `[]`)
	_, err := LoadFromJSONFile(path)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "auth file is empty") {
		t.Fatalf("expected empty auth error, got %v", err)
	}
}

func TestLoadFromJSONFileOnlyEmptyUsers(t *testing.T) {
	path := writeTempFile(t, `[{"user":"","pass":"x"}]`)
	_, err := LoadFromJSONFile(path)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "auth file is empty") {
		t.Fatalf("expected empty auth error, got %v", err)
	}
}
