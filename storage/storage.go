package storage

import (
	"sync"

	"github.com/alcounit/selenosis/platform"
	"github.com/alcounit/selenosis/tools"
)

//Storage ...
type Storage struct {
	sessions map[string]*platform.Service
	sync.RWMutex
}

//New ...
func New() *Storage {
	return &Storage{
		sessions: make(map[string]*platform.Service),
	}
}

//Put ...
func (s *Storage) Put(sessionID string, service *platform.Service) {
	s.Lock()
	defer s.Unlock()
	if sessionID != "" {
		s.sessions[sessionID] = service
	}
}

//Delete ...
func (s *Storage) Delete(sessionID string) {
	s.Lock()
	defer s.Unlock()
	delete(s.sessions, sessionID)
}

//List ...
func (s *Storage) List() []platform.Service {
	s.Lock()
	defer s.Unlock()
	var l []platform.Service

	for _, p := range s.sessions {
		c := *p
		c.Uptime = tools.TimeElapsed(c.Started)
		l = append(l, c)
	}
	return l

}

//Len ...
func (s *Storage) Len() int {
	s.Lock()
	defer s.Unlock()

	return len(s.sessions)
}
