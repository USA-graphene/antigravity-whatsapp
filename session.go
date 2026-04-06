package main

import (
	"sync"
	"time"
)

// Session tracks a single WhatsApp conversation's state.
type Session struct {
	ID         string
	History    []ChatMessage
	LastActive time.Time
	mu         sync.Mutex
}

// ChatMessage is a single message in the conversation history.
type ChatMessage struct {
	Role    string // "user" or "model"
	Content string
	Time    time.Time
}

// SessionManager maintains per-conversation sessions.
type SessionManager struct {
	sessions   map[string]*Session
	maxHistory int
	mu         sync.RWMutex
}

// NewSessionManager creates a new session manager.
func NewSessionManager(maxHistory int) *SessionManager {
	sm := &SessionManager{
		sessions:   make(map[string]*Session),
		maxHistory: maxHistory,
	}
	// Start cleanup goroutine
	go sm.cleanupLoop()
	return sm
}

// Get returns (or creates) a session for the given conversation ID.
func (sm *SessionManager) Get(id string) *Session {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	s, ok := sm.sessions[id]
	if !ok {
		s = &Session{
			ID:         id,
			History:    make([]ChatMessage, 0),
			LastActive: time.Now(),
		}
		sm.sessions[id] = s
	}
	s.LastActive = time.Now()
	return s
}

// AddMessage adds a message to the session history, enforcing max length.
func (s *Session) AddMessage(role, content string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.History = append(s.History, ChatMessage{
		Role:    role,
		Content: content,
		Time:    time.Now(),
	})
	s.LastActive = time.Now()
}

// TrimHistory trims the session to maxHistory messages, keeping the
// most recent ones.
func (s *Session) TrimHistory(maxHistory int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.History) > maxHistory {
		s.History = s.History[len(s.History)-maxHistory:]
	}
}

// Clear resets the conversation history.
func (s *Session) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.History = make([]ChatMessage, 0)
	s.LastActive = time.Now()
}

// cleanupLoop periodically removes stale sessions (inactive > 1 hour).
func (sm *SessionManager) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		sm.mu.Lock()
		now := time.Now()
		for id, s := range sm.sessions {
			if now.Sub(s.LastActive) > 1*time.Hour {
				delete(sm.sessions, id)
			}
		}
		sm.mu.Unlock()
	}
}

// Reset deletes a specific session (e.g., when user sends /reset).
func (sm *SessionManager) Reset(id string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	delete(sm.sessions, id)
}
