package state

import (
	"sync"
	"time"
)

type ActivePane struct {
	PaneID     string    `json:"pane_id"`
	SelectedAt time.Time `json:"selected_at"`
}

type Store struct {
	mu   sync.RWMutex
	pane *ActivePane
}

func NewStore() *Store {
	return &Store{}
}

func (s *Store) Get() *ActivePane {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.pane == nil {
		return nil
	}

	out := *s.pane
	return &out
}

func (s *Store) Set(paneID string) *ActivePane {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.pane = &ActivePane{
		PaneID:     paneID,
		SelectedAt: time.Now().UTC(),
	}

	out := *s.pane
	return &out
}

func (s *Store) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.pane = nil
}
