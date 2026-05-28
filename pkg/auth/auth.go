package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	logctx "github.com/alcounit/browser-controller/pkg/log"
	"github.com/fsnotify/fsnotify"
)

type User struct {
	User string `json:"user"`
	Pass string `json:"pass"`
}

type AuthStore struct {
	mu    sync.RWMutex
	users map[string]string
	path  string
}

func (s *AuthStore) Authenticate(user, pass string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	expected, exists := s.users[user]
	return exists && pass == expected
}

func reload(s *AuthStore) error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return fmt.Errorf("read auth file: %w", err)
	}

	var list []User
	if err := json.Unmarshal(data, &list); err != nil {
		return fmt.Errorf("parse auth file: %w", err)
	}

	users := make(map[string]string, len(list))
	for _, u := range list {
		if u.User == "" {
			continue
		}
		users[u.User] = u.Pass
	}

	if len(users) == 0 {
		return fmt.Errorf("auth file is empty")
	}

	s.mu.Lock()
	s.users = users
	s.mu.Unlock()
	return nil
}

func Watch(ctx context.Context, as *AuthStore) {
	log := logctx.FromContext(ctx)

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Error().Err(err).Msg("auth watcher: failed to create")
		return
	}
	defer watcher.Close()

	dir := filepath.Dir(as.path)
	if err := watcher.Add(dir); err != nil {
		log.Error().Err(err).Str("dir", dir).Msg("auth watcher: failed to watch directory")
		return
	}

	base := filepath.Base(as.path)

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			name := filepath.Base(event.Name)
			k8sRotation := name == "..data" && event.Has(fsnotify.Create)
			directWrite := name == base && (event.Has(fsnotify.Write) || event.Has(fsnotify.Create))
			if k8sRotation || directWrite {
				if err := reload(as); err != nil {
					log.Error().Err(err).Msg("auth watcher: reload failed")
				} else {
					log.Info().Msg("auth file reloaded")
				}
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Error().Err(err).Msg("auth watcher error")
		}
	}
}

func LoadFromJSONFile(path string) (*AuthStore, error) {
	store := &AuthStore{path: path}
	if err := reload(store); err != nil {
		return nil, err
	}
	return store, nil
}
