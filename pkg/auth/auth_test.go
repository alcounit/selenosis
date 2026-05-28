package auth

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
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

func TestReloadUpdatesCredentials(t *testing.T) {
	path := writeTempFile(t, `[{"user":"alice","pass":"old"}]`)
	store, err := LoadFromJSONFile(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if err := os.WriteFile(path, []byte(`[{"user":"alice","pass":"new"}]`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := reload(store); err != nil {
		t.Fatalf("reload: %v", err)
	}

	if store.Authenticate("alice", "old") {
		t.Fatal("old password should no longer work")
	}
	if !store.Authenticate("alice", "new") {
		t.Fatal("new password should work")
	}
}

func TestReloadPreservesCredentialsOnError(t *testing.T) {
	path := writeTempFile(t, `[{"user":"alice","pass":"ok"}]`)
	store, err := LoadFromJSONFile(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if err := os.WriteFile(path, []byte(`{broken`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := reload(store); err == nil {
		t.Fatal("expected error on bad JSON")
	}

	if !store.Authenticate("alice", "ok") {
		t.Fatal("credentials should be preserved after failed reload")
	}
}

func TestReloadConcurrent(t *testing.T) {
	path := writeTempFile(t, `[{"user":"u","pass":"p"}]`)
	store, err := LoadFromJSONFile(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			store.Authenticate("u", "p")
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = reload(store)
		}()
	}
	wg.Wait()
}

func pollUntil(t *testing.T, timeout time.Duration, cond func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}

func TestWatchDirectWrite(t *testing.T) {
	path := writeTempFile(t, `[{"user":"alice","pass":"old"}]`)
	store, err := LoadFromJSONFile(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go Watch(ctx, store)

	time.Sleep(50 * time.Millisecond)

	if err := os.WriteFile(path, []byte(`[{"user":"alice","pass":"new"}]`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	if !pollUntil(t, 2*time.Second, func() bool { return store.Authenticate("alice", "new") }) {
		t.Fatal("store was not reloaded after direct file write")
	}
}

func TestWatchKubernetesStyleRotation(t *testing.T) {
	// mirrors how kubelet mounts Secrets:
	// mountDir/
	//   ..data/auth.json   <- actual file
	//   auth.json          -> ..data/auth.json (symlink)
	mountDir := t.TempDir()
	dataDir := filepath.Join(mountDir, "..data")
	if err := os.Mkdir(dataDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	dataFile := filepath.Join(dataDir, "auth.json")
	if err := os.WriteFile(dataFile, []byte(`[{"user":"alice","pass":"old"}]`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	authPath := filepath.Join(mountDir, "auth.json")
	if err := os.Symlink(dataFile, authPath); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	store, err := LoadFromJSONFile(authPath)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go Watch(ctx, store)

	time.Sleep(50 * time.Millisecond)

	// simulate kubelet atomic swap: write new data dir, rename over ..data
	newDataDir := filepath.Join(mountDir, "..data_tmp")
	if err := os.Mkdir(newDataDir, 0o755); err != nil {
		t.Fatalf("mkdir tmp: %v", err)
	}
	if err := os.WriteFile(filepath.Join(newDataDir, "auth.json"), []byte(`[{"user":"alice","pass":"new"}]`), 0o600); err != nil {
		t.Fatalf("write tmp: %v", err)
	}
	if err := os.RemoveAll(dataDir); err != nil {
		t.Fatalf("remove old datadir: %v", err)
	}
	if err := os.Rename(newDataDir, dataDir); err != nil {
		t.Fatalf("rename: %v", err)
	}

	if !pollUntil(t, 2*time.Second, func() bool { return store.Authenticate("alice", "new") }) {
		t.Fatal("store was not reloaded after Kubernetes-style Secret rotation")
	}
}

func TestWatchStopsOnContextCancel(t *testing.T) {
	path := writeTempFile(t, `[{"user":"u","pass":"p"}]`)
	store, err := LoadFromJSONFile(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		Watch(ctx, store)
		close(done)
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Watch did not stop after context cancellation")
	}
}
