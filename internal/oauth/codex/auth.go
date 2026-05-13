package codex

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// OAuth endpoints for OpenAI/Codex
const (
	AuthURL  = "https://auth.openai.com/oauth/authorize"
	TokenURL = "https://auth.openai.com/oauth/token"
	ClientID = "app_EMoamEEZ73f0CkXaXp7hrann"
)

// Auth handles Codex/OpenAI OAuth authentication
type Auth struct {
	httpClient *http.Client
}

// NewAuth creates a new Codex authenticator
func NewAuth() *Auth {
	return &Auth{
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// TokenResponse represents the OAuth token response from OpenAI
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope"`
}

// TokenData represents stored Codex token data for AuthJson.Content
type TokenData struct {
	AccessToken  string `json:"AccessToken"`
	RefreshToken string `json:"RefreshToken"`
	IDToken      string `json:"IDToken,omitempty"`
	Email        string `json:"Email,omitempty"`
	ExpiresAt    int64  `json:"ExpiresAt"`
}

// GetAuthURL generates the authorization URL for user to visit
func (a *Auth) GetAuthURL(state, codeChallenge, redirectURI string) string {
	params := url.Values{
		"client_id":                  {ClientID},
		"response_type":              {"code"},
		"redirect_uri":               {redirectURI},
		"scope":                      {"openid email profile offline_access"},
		"state":                      {state},
		"code_challenge":             {codeChallenge},
		"code_challenge_method":      {"S256"},
		"prompt":                     {"login"},
		"id_token_add_organizations": {"true"},
		"codex_cli_simplified_flow":  {"true"},
	}

	return AuthURL + "?" + params.Encode()
}

// ExchangeCode exchanges the authorization code for tokens
func (a *Auth) ExchangeCode(ctx context.Context, code, codeVerifier, redirectURI string) (*TokenResponse, error) {
	data := url.Values{
		"client_id":     {ClientID},
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"code_verifier": {codeVerifier},
	}

	return a.doTokenRequest(ctx, data.Encode())
}

// RefreshToken refreshes the access token using refresh token
func (a *Auth) RefreshToken(ctx context.Context, refreshToken string) (*TokenResponse, error) {
	data := url.Values{
		"client_id":     {ClientID},
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
	}

	return a.doTokenRequest(ctx, data.Encode())
}

// doTokenRequest makes a token request to OpenAI
func (a *Auth) doTokenRequest(ctx context.Context, body string) (*TokenResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, TokenURL, strings.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token request failed: status=%d body=%s", resp.StatusCode, string(respBody))
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(respBody, &tokenResp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return &tokenResp, nil
}

// BuildTokenData builds TokenData from TokenResponse
func (a *Auth) BuildTokenData(resp *TokenResponse, email string) *TokenData {
	expiresAt := time.Now().Add(time.Duration(resp.ExpiresIn) * time.Second).Unix()

	return &TokenData{
		AccessToken:  resp.AccessToken,
		RefreshToken: resp.RefreshToken,
		IDToken:      resp.IDToken,
		Email:        email,
		ExpiresAt:    expiresAt,
	}
}

// ToJSON converts TokenData to JSON string
func (td *TokenData) ToJSON() (string, error) {
	data, err := json.Marshal(td)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ParseTokenData parses JSON string to TokenData
func ParseTokenData(jsonStr string) (*TokenData, error) {
	var td TokenData
	if err := json.Unmarshal([]byte(jsonStr), &td); err != nil {
		return nil, err
	}
	return &td, nil
}

// ExtractEmailFromIDToken extracts email from ID token (JWT)
func ExtractEmailFromIDToken(idToken string) string {
	// ID token is a JWT: header.payload.signature
	parts := strings.Split(idToken, ".")
	if len(parts) != 3 {
		return ""
	}

	decoded, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}

	var claims struct {
		Email string `json:"email"`
	}
	if err := json.Unmarshal(decoded, &claims); err != nil {
		return ""
	}

	return claims.Email
}
