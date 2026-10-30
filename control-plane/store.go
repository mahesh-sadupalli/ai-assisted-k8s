package main

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
)

// Store keeps deployment intents in memory. Safe for concurrent HTTP handlers.
type Store struct {
	mu          sync.RWMutex
	deployments map[string]Deployment // by ID
}

func NewStore() *Store {
	return &Store{deployments: map[string]Deployment{}}
}

func newID() string {
	b := make([]byte, 6)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// Upsert runs check against the deployments already in d's namespace and, if it passes,
// stores d. A deployment with the same namespace+name is replaced and keeps its ID.
// Check and write happen under one lock so two requests can't both squeeze under quota.
func (s *Store) Upsert(d Deployment, check func(existing []Deployment) error) (Deployment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var existing []Deployment
	d.ID = ""
	for _, old := range s.deployments {
		if old.Namespace == d.Namespace {
			existing = append(existing, old)
			if old.Name == d.Name {
				d.ID = old.ID
			}
		}
	}
	if err := check(existing); err != nil {
		return Deployment{}, err
	}
	if d.ID == "" {
		d.ID = newID()
	}
	s.deployments[d.ID] = d
	return d, nil
}

func (s *Store) Get(id string) (Deployment, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	d, ok := s.deployments[id]
	return d, ok
}

// SetStatus updates the status of a stored deployment.
func (s *Store) SetStatus(id, status string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if d, ok := s.deployments[id]; ok {
		d.Status = status
		s.deployments[id] = d
	}
}
