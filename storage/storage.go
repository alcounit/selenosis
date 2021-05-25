package storage

import (
	"sync"

	"github.com/alcounit/selenosis/platform"
	"github.com/alcounit/selenosis/tools"
)

type sessions struct {
	m map[string]platform.Service
}

//Put ...
func (s *sessions) Put(sessionID string, service platform.Service) {
	if sessionID != "" {
		s.m[sessionID] = service
	}
}

//Delete ...
func (s *sessions) Delete(sessionID string) {
	delete(s.m, sessionID)
}

//List ...
func (s *sessions) List() map[string]platform.Service {
	return s.m

}

//Len ...
func (s *sessions) Len() int {
	return len(s.m)
}

type workers struct {
	m map[string]platform.Worker
}

//Put ...
func (w *workers) Put(name string, worker platform.Worker) {
	if name != "" {
		w.m[name] = worker
	}
}

//Delete ...
func (w *workers) Delete(name string) {
	delete(w.m, name)
}

//List ...
func (w *workers) List() []platform.Worker {
	var l []platform.Worker
	for _, w := range w.m {
		w.Uptime = tools.TimeElapsed(w.Started)
		l = append(l, w)
	}

	return l

}

//Len ...
func (s *workers) Len() int {
	return len(s.m)
}

type quota struct {
	w *workers
	q platform.Quota
}

//Put ...
func (q *quota) Put(quota platform.Quota) {
	q.q = quota
}

//Put ...
func (q *quota) Get() platform.Quota {
	return q.q
}

//Storage ...
type Storage struct {
	sessions *sessions
	workers  *workers
	quota    *quota
	sync.RWMutex
}

//New ...
func New() *Storage {
	sessions := &sessions{m: make(map[string]platform.Service)}
	workers := &workers{m: make(map[string]platform.Worker)}
	quota := &quota{w: workers}
	return &Storage{
		sessions: sessions,
		workers:  workers,
		quota:    quota,
	}
}

//Sessions ...
func (s *Storage) Sessions() *sessions {
	s.Lock()
	defer s.Unlock()
	return s.sessions
}

//Workers ...
func (s *Storage) Workers() *workers {
	s.Lock()
	defer s.Unlock()
	return s.workers
}

//Quota ...
func (s *Storage) Quota() *quota {
	s.Lock()
	defer s.Unlock()
	return s.quota
}
