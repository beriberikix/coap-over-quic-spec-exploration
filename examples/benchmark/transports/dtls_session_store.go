package transports

import (
	"fmt"
	"sync"

	piondtls "github.com/pion/dtls/v3"
)

// InMemorySessionStore implements pion/dtls SessionStore interface for session caching
// This enables DTLS session resumption (abbreviated handshake) for faster reconnections
type InMemorySessionStore struct {
	sessions map[string]*piondtls.Session
	mutex    sync.RWMutex
}

// NewInMemorySessionStore creates a new in-memory session store
func NewInMemorySessionStore() *InMemorySessionStore {
	return &InMemorySessionStore{
		sessions: make(map[string]*piondtls.Session),
	}
}

// Set stores a session with the given key
// For clients, the key is the server address
// For servers, the key is the session ID
func (s *InMemorySessionStore) Set(key []byte, session piondtls.Session) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	keyStr := string(key)
	s.sessions[keyStr] = &session

	// Debug logging
	fmt.Printf("[SessionStore DEBUG] Set session - key length: %d, key hex: %x, key string: %q, total sessions: %d\n",
		len(key), key, keyStr, len(s.sessions))

	return nil
}

// Get retrieves a session by key
// Returns the session if found, error otherwise
func (s *InMemorySessionStore) Get(key []byte) (piondtls.Session, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	keyStr := string(key)

	// Debug logging
	fmt.Printf("[SessionStore DEBUG] Get session - key length: %d, key hex: %x, key string: %q, total sessions: %d\n",
		len(key), key, keyStr, len(s.sessions))

	// List all stored keys for debugging
	fmt.Printf("[SessionStore DEBUG] Stored keys:\n")
	for k := range s.sessions {
		fmt.Printf("  - key string: %q, hex: %x\n", k, []byte(k))
	}

	if session, exists := s.sessions[keyStr]; exists {
		fmt.Printf("[SessionStore DEBUG] Session FOUND!\n")
		return *session, nil
	}

	// Return empty session and error if not found
	fmt.Printf("[SessionStore DEBUG] Session NOT FOUND\n")
	return piondtls.Session{}, fmt.Errorf("session not found")
}

// Del removes a session by key
func (s *InMemorySessionStore) Del(key []byte) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	keyStr := string(key)
	delete(s.sessions, keyStr)

	return nil
}

// Count returns the number of cached sessions (for debugging/testing)
func (s *InMemorySessionStore) Count() int {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	return len(s.sessions)
}

// Clear removes all cached sessions
func (s *InMemorySessionStore) Clear() {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.sessions = make(map[string]*piondtls.Session)
}
