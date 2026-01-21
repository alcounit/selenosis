package auth

import (
	"encoding/json"
	"fmt"
	"os"
)

type User struct {
	User string `json:"user"`
	Pass string `json:"pass"`
}

type AuthStore struct {
	users map[string]string
}

func (s *AuthStore) Verify(user, pass string) bool {
	expected, exists := s.users[user]
	return exists && pass == expected
}

func LoadFromJSONFile(path string) (*AuthStore, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read auth file: %w", err)
	}

	var list []User
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("parse auth file: %w", err)
	}

	users := make(map[string]string, len(list))
	for _, u := range list {
		if u.User == "" {
			continue
		}
		users[u.User] = u.Pass
	}

	if len(users) == 0 {
		return nil, fmt.Errorf("auth file is empty")
	}

	return &AuthStore{users: users}, nil
}
