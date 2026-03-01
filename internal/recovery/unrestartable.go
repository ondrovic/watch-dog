// Package recovery implements the restart flow and in-memory unrestartable set
// (see specs/005-fix-recovery-stale-container/data-model.md).
package recovery

import (
	"sync"
)

const defaultUnrestartableCap = 100

// Set is an in-memory set of container IDs for which restart (or inspect during
// wait-for-healthy) has failed with an unrestartable error. It is bounded in size
// and thread-safe. Call Add to record an ID, Contains to check, and Prune to
// remove IDs not in the current container list.
type Set struct {
	mu    sync.Mutex
	ids   map[string]struct{}
	order []string // FIFO for cap eviction
	cap   int
}

// NewSet returns an unrestartable set with the given maximum size (0 = defaultUnrestartableCap).
func NewSet(cap int) *Set {
	if cap <= 0 {
		cap = defaultUnrestartableCap
	}
	return &Set{ids: make(map[string]struct{}), cap: cap}
}

// Contains reports whether containerID is in the set.
func (s *Set) Contains(containerID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.ids[containerID]
	return ok
}

// Add adds containerID to the set. If the set would exceed its cap, an existing
// entry is removed: prefer one not in currentList (if provided), otherwise the oldest (FIFO).
func (s *Set) Add(containerID string, currentList []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.ids[containerID]; exists {
		return
	}
	for len(s.ids) >= s.cap && len(s.order) > 0 {
		// Choose which to evict: one not in current list, or oldest
		evict := ""
		if len(currentList) > 0 {
			currentSet := make(map[string]struct{}, len(currentList))
			for _, id := range currentList {
				currentSet[id] = struct{}{}
			}
			for _, id := range s.order {
				if _, in := currentSet[id]; !in {
					evict = id
					break
				}
			}
		}
		if evict == "" {
			evict = s.order[0]
		}
		// Remove evict from order and ids
		for i, id := range s.order {
			if id == evict {
				s.order = append(s.order[:i], s.order[i+1:]...)
				break
			}
		}
		delete(s.ids, evict)
		break
	}
	s.ids[containerID] = struct{}{}
	s.order = append(s.order, containerID)
}

// Prune removes from the set any ID that is not in currentIDs.
// Call after ListContainers to keep the set from growing indefinitely.
func (s *Set) Prune(currentIDs []string) {
	m := make(map[string]struct{}, len(currentIDs))
	for _, id := range currentIDs {
		m[id] = struct{}{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for id := range s.ids {
		if _, in := m[id]; !in {
			delete(s.ids, id)
		}
	}
	// Rebuild order to drop pruned IDs
	newOrder := make([]string, 0, len(s.ids))
	for _, id := range s.order {
		if _, in := s.ids[id]; in {
			newOrder = append(newOrder, id)
		}
	}
	s.order = newOrder
}
