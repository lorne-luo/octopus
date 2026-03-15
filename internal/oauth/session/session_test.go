package session

import (
	"testing"
	"time"

	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/oauth/pkce"
)

func TestManagerCreateSession(t *testing.T) {
	m := NewManager()
	pkceCodes, _ := pkce.GeneratePKCECodes()

	session, err := m.CreateSession(model.OAuthProviderTypeCodex, CallbackModeAuto, pkceCodes, 1455)
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	if session.State == "" {
		t.Error("Session state should not be empty")
	}
	if session.ProviderType != model.OAuthProviderTypeCodex {
		t.Errorf("ProviderType = %v, want %v", session.ProviderType, model.OAuthProviderTypeCodex)
	}
	if session.CallbackMode != CallbackModeAuto {
		t.Errorf("CallbackMode = %v, want %v", session.CallbackMode, CallbackModeAuto)
	}
	if session.CallbackPort != 1455 {
		t.Errorf("CallbackPort = %d, want 1455", session.CallbackPort)
	}
	if session.PKCECodes == nil {
		t.Error("PKCECodes should not be nil")
	}
	if session.CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}
	if session.ExpiresAt.IsZero() {
		t.Error("ExpiresAt should not be zero")
	}
	if session.ExpiresAt.Before(session.CreatedAt) {
		t.Error("ExpiresAt should be after CreatedAt")
	}
}

func TestManagerValidateSession(t *testing.T) {
	m := NewManager()
	pkceCodes, _ := pkce.GeneratePKCECodes()

	session, _ := m.CreateSession(model.OAuthProviderTypeCodex, CallbackModeManual, pkceCodes, 0)

	// Valid state
	validated, err := m.ValidateSession(session.State)
	if err != nil {
		t.Fatalf("ValidateSession() error = %v", err)
	}
	if validated.State != session.State {
		t.Errorf("Validated state = %s, want %s", validated.State, session.State)
	}

	// Invalid state
	_, err = m.ValidateSession("invalid-state")
	if err == nil {
		t.Error("ValidateSession() should return error for invalid state")
	}
}

func TestManagerCompleteSession(t *testing.T) {
	m := NewManager()
	pkceCodes, _ := pkce.GeneratePKCECodes()

	session, _ := m.CreateSession(model.OAuthProviderTypeCodex, CallbackModeAuto, pkceCodes, 1455)

	// Complete the session
	m.CompleteSession(session.State)

	// Session should no longer be valid
	_, err := m.ValidateSession(session.State)
	if err == nil {
		t.Error("ValidateSession() should return error for completed session")
	}
}

func TestManagerSessionExpiry(t *testing.T) {
	// Create manager with very short TTL for testing
	m := NewManagerWithTTL(100 * time.Millisecond)
	pkceCodes, _ := pkce.GeneratePKCECodes()

	session, _ := m.CreateSession(model.OAuthProviderTypeCodex, CallbackModeManual, pkceCodes, 0)

	// Session should be valid immediately
	_, err := m.ValidateSession(session.State)
	if err != nil {
		t.Fatalf("ValidateSession() error = %v", err)
	}

	// Wait for expiry
	time.Sleep(150 * time.Millisecond)

	// Session should be expired
	_, err = m.ValidateSession(session.State)
	if err == nil {
		t.Error("ValidateSession() should return error for expired session")
	}
}

func TestManagerCleanupExpiredSessions(t *testing.T) {
	m := NewManagerWithTTL(50 * time.Millisecond)
	pkceCodes, _ := pkce.GeneratePKCECodes()

	// Create multiple sessions
	session1, _ := m.CreateSession(model.OAuthProviderTypeCodex, CallbackModeManual, pkceCodes, 0)
	session2, _ := m.CreateSession(model.OAuthProviderTypeCodex, CallbackModeManual, pkceCodes, 0)

	// Wait for expiry
	time.Sleep(100 * time.Millisecond)

	// Cleanup should remove expired sessions
	m.CleanupExpiredSessions()

	// Both sessions should be removed
	_, err1 := m.ValidateSession(session1.State)
	_, err2 := m.ValidateSession(session2.State)

	if err1 == nil || err2 == nil {
		t.Error("CleanupExpiredSessions() should remove expired sessions")
	}
}