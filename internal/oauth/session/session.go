package session

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/oauth/pkce"
)

// CallbackMode represents the OAuth callback mode
type CallbackMode int

const (
	// CallbackModeAuto starts a local HTTP server for automatic callback
	CallbackModeAuto CallbackMode = iota
	// CallbackModeManual requires user to paste callback URL
	CallbackModeManual
)

// OAuthSession represents a pending OAuth flow
type OAuthSession struct {
	State        string                  // Random state for CSRF protection
	ProviderType model.OAuthProviderType // Provider type
	PKCECodes    *pkce.PKCECodes         // PKCE codes for security
	CallbackMode CallbackMode            // Callback mode
	CallbackPort int                     // Port for auto mode
	CreatedAt    time.Time               // Session creation time
	ExpiresAt    time.Time               // Session expiration time

	// Auto mode fields
	Completed    bool   `json:"completed"`     // Whether callback was received
	CallbackCode string `json:"callback_code"` // Authorization code from callback
	CallbackErr  string `json:"callback_err"`  // Error from callback if any
}

// Manager manages OAuth sessions in memory
type Manager struct {
	mu       sync.RWMutex
	sessions map[string]*OAuthSession
	ttl      time.Duration
}

var (
	ErrSessionNotFound = errors.New("session not found")
	ErrSessionExpired  = errors.New("session expired")
)

// NewManager creates a new session manager with default TTL
func NewManager() *Manager {
	return NewManagerWithTTL(10 * time.Minute)
}

// NewManagerWithTTL creates a new session manager with custom TTL
func NewManagerWithTTL(ttl time.Duration) *Manager {
	return &Manager{
		sessions: make(map[string]*OAuthSession),
		ttl:      ttl,
	}
}

// CreateSession creates a new OAuth session
func (m *Manager) CreateSession(
	providerType model.OAuthProviderType,
	mode CallbackMode,
	pkceCodes *pkce.PKCECodes,
	port int,
) (*OAuthSession, error) {
	state, err := generateState()
	if err != nil {
		return nil, err
	}

	now := time.Now()
	session := &OAuthSession{
		State:        state,
		ProviderType: providerType,
		PKCECodes:    pkceCodes,
		CallbackMode: mode,
		CallbackPort: port,
		CreatedAt:    now,
		ExpiresAt:    now.Add(m.ttl),
	}

	m.mu.Lock()
	m.sessions[state] = session
	m.mu.Unlock()

	return session, nil
}

// ValidateSession validates and returns the session for the given state
func (m *Manager) ValidateSession(state string) (*OAuthSession, error) {
	m.mu.RLock()
	session, exists := m.sessions[state]
	m.mu.RUnlock()

	if !exists {
		return nil, ErrSessionNotFound
	}

	if time.Now().After(session.ExpiresAt) {
		m.mu.Lock()
		delete(m.sessions, state)
		m.mu.Unlock()
		return nil, ErrSessionExpired
	}

	return session, nil
}

// CompleteSession marks a session as completed and removes it
func (m *Manager) CompleteSession(state string) {
	m.mu.Lock()
	delete(m.sessions, state)
	m.mu.Unlock()
}

// SetCallbackResult sets the callback result for a session (auto mode)
func (m *Manager) SetCallbackResult(state, code, errStr string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if session, exists := m.sessions[state]; exists {
		session.CallbackCode = code
		session.CallbackErr = errStr
		session.Completed = true
	}
}

// GetCallbackResult gets the callback result for a session
// Returns (completed, code, error)
func (m *Manager) GetCallbackResult(state string) (bool, string, string) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if session, exists := m.sessions[state]; exists {
		return session.Completed, session.CallbackCode, session.CallbackErr
	}
	return false, "", ""
}

// CleanupExpiredSessions removes all expired sessions
func (m *Manager) CleanupExpiredSessions() {
	now := time.Now()
	m.mu.Lock()
	defer m.mu.Unlock()

	for state, session := range m.sessions {
		if now.After(session.ExpiresAt) {
			delete(m.sessions, state)
		}
	}
}

// generateState generates a random state string for CSRF protection
func generateState() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}