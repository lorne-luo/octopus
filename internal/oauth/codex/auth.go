package codex

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// OAuth endpoints for OpenAI/Codex
const (
	AuthURL              = "https://auth.openai.com/oauth/authorize"
	TokenURL             = "https://auth.openai.com/oauth/token"
	ClientID             = "app_EMoamEEZ73f0CkXaXp7hrann"
	DeviceUserCodeURL    = "https://auth.openai.com/api/accounts/deviceauth/usercode"
	DeviceTokenURL       = "https://auth.openai.com/api/accounts/deviceauth/token"
	DeviceVerificationURL = "https://auth.openai.com/codex/device"
	DeviceTokenExchangeRedirectURI = "https://auth.openai.com/deviceauth/callback"
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

	payload := parts[1]
	// Add padding if needed (JWT uses base64url without padding)
	if l := len(payload) % 4; l > 0 {
		payload += strings.Repeat("=", 4-l)
	}

	// Decode using URLEncoding (handles URL-safe base64)
	decoded, err := base64.URLEncoding.DecodeString(payload)
	if err != nil {
		// Fallback: try with standard encoding after replacing URL-safe chars
		payload = strings.ReplaceAll(payload, "-", "+")
		payload = strings.ReplaceAll(payload, "_", "/")
		decoded, err = base64.StdEncoding.DecodeString(payload)
		if err != nil {
			return ""
		}
	}

	var claims struct {
		Email string `json:"email"`
	}
	if err := json.Unmarshal(decoded, &claims); err != nil {
		return ""
	}

	return claims.Email
}

// DeviceFlowUserCodeResponse represents the response from the device code endpoint
type DeviceFlowUserCodeResponse struct {
	DeviceAuthID string `json:"device_auth_id"`
	UserCode     string `json:"user_code"`
	UserCodeAlt  string `json:"usercode"`
	Interval     json.RawMessage `json:"interval"`
}

// DeviceFlowTokenResponse represents the response from the device token polling endpoint
type DeviceFlowTokenResponse struct {
	AuthorizationCode string `json:"authorization_code"`
	CodeVerifier      string `json:"code_verifier"`
	CodeChallenge     string `json:"code_challenge"`
}

// RequestDeviceCode requests a device code from OpenAI for the device flow
func (a *Auth) RequestDeviceCode(ctx context.Context) (*DeviceFlowUserCodeResponse, error) {
	body, err := json.Marshal(map[string]string{"client_id": ClientID})
	if err != nil {
		return nil, fmt.Errorf("marshal device code request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, DeviceUserCodeURL, strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("create device code request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("device code request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read device code response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("device code request failed: status=%d body=%s", resp.StatusCode, string(respBody))
	}

	var result DeviceFlowUserCodeResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("parse device code response: %w", err)
	}

	return &result, nil
}

// PollDeviceToken polls OpenAI for the device authorization result.
// Returns the authorization code + PKCE codes once the user completes verification.
func (a *Auth) PollDeviceToken(ctx context.Context, deviceAuthID, userCode string, interval time.Duration) (*DeviceFlowTokenResponse, error) {
	deadline := time.Now().Add(15 * time.Minute)

	for {
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("device auth timeout after 15 minutes")
		}

		body, err := json.Marshal(map[string]string{
			"device_auth_id": deviceAuthID,
			"user_code":      userCode,
		})
		if err != nil {
			return nil, fmt.Errorf("marshal poll request: %w", err)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, DeviceTokenURL, strings.NewReader(string(body)))
		if err != nil {
			return nil, fmt.Errorf("create poll request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")

		resp, err := a.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("poll request: %w", err)
		}

		respBody, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("read poll response: %w", readErr)
		}

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			var result DeviceFlowTokenResponse
			if err := json.Unmarshal(respBody, &result); err != nil {
				return nil, fmt.Errorf("parse poll response: %w", err)
			}
			return &result, nil
		}

		if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusNotFound {
			// Not yet authorized, keep polling
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(interval):
				continue
			}
		}

		return nil, fmt.Errorf("device poll failed: status=%d body=%s", resp.StatusCode, string(respBody))
	}
}

// ExchangeCodeWithRedirect exchanges an authorization code using a specific redirect URI
func (a *Auth) ExchangeCodeWithRedirect(ctx context.Context, code, codeVerifier, redirectURI string) (*TokenResponse, error) {
	data := url.Values{
		"client_id":     {ClientID},
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"code_verifier": {codeVerifier},
	}

	return a.doTokenRequest(ctx, data.Encode())
}

// ParseDevicePollInterval parses the interval from the device code response
func ParseDevicePollInterval(raw json.RawMessage) time.Duration {
	defaultInterval := 5 * time.Second
	if len(raw) == 0 {
		return defaultInterval
	}

	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		if seconds, convErr := strconv.Atoi(strings.TrimSpace(asString)); convErr == nil && seconds > 0 {
			return time.Duration(seconds) * time.Second
		}
	}

	var asInt int
	if err := json.Unmarshal(raw, &asInt); err == nil && asInt > 0 {
		return time.Duration(asInt) * time.Second
	}

	return defaultInterval
}