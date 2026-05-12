package codex

import (
	"testing"
)

func TestGetAuthURL(t *testing.T) {
	auth := NewAuth()

	state := "test-state-123"
	codeChallenge := "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
	redirectURI := "http://localhost:1455/auth/callback"

	authURL := auth.GetAuthURL(state, codeChallenge, redirectURI)

	// Verify URL contains required parameters
	if authURL == "" {
		t.Fatal("AuthURL is empty")
	}

	tests := []struct {
		name     string
		contains string
	}{
		{"client_id", "client_id=" + ClientID},
		{"response_type", "response_type=code"},
		{"redirect_uri", "redirect_uri="},
		{"scope", "scope=openid"},
		{"state", "state=" + state},
		{"code_challenge", "code_challenge=" + codeChallenge},
		{"code_challenge_method", "code_challenge_method=S256"},
		{"id_token_add_organizations", "id_token_add_organizations=true"},
		{"codex_cli_simplified_flow", "codex_cli_simplified_flow=true"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !contains(authURL, tt.contains) {
				t.Errorf("AuthURL missing %s: %s", tt.name, authURL)
			}
		})
	}
}

func TestBuildTokenData(t *testing.T) {
	auth := NewAuth()

	tokenResp := &TokenResponse{
		AccessToken:  "test-access-token",
		RefreshToken: "test-refresh-token",
		IDToken:      "test-id-token",
		TokenType:    "Bearer",
		ExpiresIn:    3600,
	}

	tokenData := auth.BuildTokenData(tokenResp, "test@example.com")

	if tokenData.AccessToken != "test-access-token" {
		t.Errorf("AccessToken = %s, want test-access-token", tokenData.AccessToken)
	}
	if tokenData.RefreshToken != "test-refresh-token" {
		t.Errorf("RefreshToken = %s, want test-refresh-token", tokenData.RefreshToken)
	}
	if tokenData.IDToken != "test-id-token" {
		t.Errorf("IDToken = %s, want test-id-token", tokenData.IDToken)
	}
	if tokenData.Email != "test@example.com" {
		t.Errorf("Email = %s, want test@example.com", tokenData.Email)
	}
	if tokenData.ExpiresAt == 0 {
		t.Error("ExpiresAt should be set")
	}
}

func TestTokenDataJSON(t *testing.T) {
	tokenData := &TokenData{
		AccessToken:  "access",
		RefreshToken: "refresh",
		IDToken:      "id",
		Email:        "test@example.com",
		ExpiresAt:    1234567890,
	}

	// Test marshaling
	jsonData, err := tokenData.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON() error = %v", err)
	}
	if jsonData == "" {
		t.Error("ToJSON() returned empty string")
	}

	// Test unmarshaling
	parsed, err := ParseTokenData(jsonData)
	if err != nil {
		t.Fatalf("ParseTokenData() error = %v", err)
	}
	if parsed.AccessToken != tokenData.AccessToken {
		t.Errorf("AccessToken = %s, want %s", parsed.AccessToken, tokenData.AccessToken)
	}
	if parsed.RefreshToken != tokenData.RefreshToken {
		t.Errorf("RefreshToken = %s, want %s", parsed.RefreshToken, tokenData.RefreshToken)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}