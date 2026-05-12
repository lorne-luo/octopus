package oauth

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/bestruirui/octopus/internal/db"
	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/oauth/codex"
	"github.com/bestruirui/octopus/internal/oauth/kiro"
	"github.com/bestruirui/octopus/internal/oauth/pkce"
	"github.com/bestruirui/octopus/internal/oauth/session"
	"github.com/bestruirui/octopus/internal/op"
	"github.com/bestruirui/octopus/internal/utils/log"
)

type Manager struct {
	sessionManager *session.Manager
	mu             sync.Mutex
}

var (
	sharedManager *Manager
	once          sync.Once
)

func GetManager() *Manager {
	once.Do(func() {
		sharedManager = &Manager{
			sessionManager: session.NewManager(),
		}
	})
	return sharedManager
}

// OAuthFlowInfo contains information for initiating OAuth flow
type OAuthFlowInfo struct {
	AuthURL      string `json:"auth_url"`
	State        string `json:"state"`
	CallbackMode string `json:"callback_mode"`
	CallbackPort int    `json:"callback_port,omitempty"`
	ExpiresIn    int    `json:"expires_in"`
	Instructions string `json:"instructions"`

	// Device flow fields
	DeviceCode   string `json:"device_code,omitempty"`
	VerificationURL string `json:"verification_url,omitempty"`
}

// RefreshAPIKey refreshes the API key for an OAuth provider
// For Kiro: it uses refresh token to get a new access token
// For Codex: it uses refresh token to get a new access token
func (m *Manager) RefreshAPIKey(ctx context.Context, provider *model.OAuthProvider) error {
	switch provider.ProviderType {
	case model.OAuthProviderTypeKiro:
		return m.refreshKiroToken(ctx, provider)
	case model.OAuthProviderTypeCodex:
		return m.refreshCodexToken(ctx, provider)
	default:
		return fmt.Errorf("unsupported provider type: %s", provider.ProviderType)
	}
}

// refreshKiroToken handles Kiro-specific token refresh
func (m *Manager) refreshKiroToken(ctx context.Context, provider *model.OAuthProvider) error {
	// Get active AuthJson
	authJson := provider.GetActiveAuthJson()
	if authJson == nil {
		m.recordFailure(provider)
		return fmt.Errorf("no active auth_json found")
	}

	// Extract RefreshToken and Region from AuthJson
	refreshToken := authJson.GetRefreshToken()
	if refreshToken == "" {
		m.recordFailure(provider)
		m.recordAuthJsonFailure(authJson)
		return fmt.Errorf("RefreshToken not found in auth_json")
	}

	region := authJson.GetRegion()

	// Call Kiro refresh endpoint
	resp, err := kiro.RefreshToken(ctx, refreshToken, region)
	if err != nil {
		m.recordFailure(provider)
		m.recordAuthJsonFailure(authJson)
		return fmt.Errorf("refresh token: %w", err)
	}

	// Validate response has access token
	if resp.AccessToken == "" {
		m.recordFailure(provider)
		m.recordAuthJsonFailure(authJson)
		return fmt.Errorf("refresh response missing access token")
	}

	// Update provider with new data
	provider.APIKey = resp.AccessToken

	// Calculate expiration time with buffer
	expireAt := kiro.CalculateExpirationTime(resp.ExpiresIn)
	provider.APIKeyExpireAt = expireAt.Unix()

	provider.LastRefreshAt = time.Now().Unix()
	provider.RefreshFailCount = 0
	provider.Status = 1

	// Update AuthJson status
	authJson.StatusCode = 200
	authJson.LastUseTimeStamp = time.Now().Unix()

	// Save both provider and authJson
	tx := db.GetDB().Begin()
	if err := tx.Save(provider).Error; err != nil {
		tx.Rollback()
		return err
	}
	if err := tx.Save(authJson).Error; err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit().Error
}

func (m *Manager) ShouldRefresh(provider *model.OAuthProvider) bool {
	if provider.APIKey == "" {
		return true
	}
	// Refresh if expired or within 1 hour of expiration
	if time.Now().Unix() > provider.APIKeyExpireAt-3600 {
		return true
	}
	return false
}

func (m *Manager) recordFailure(provider *model.OAuthProvider) {
	provider.RefreshFailCount++
	// If failed too many times, mark as expired
	if provider.RefreshFailCount > 3 {
		provider.Status = 2 // Error/Expired
	}
	db.GetDB().Save(provider)
}

func (m *Manager) recordAuthJsonFailure(authJson *model.AuthJson) {
	authJson.StatusCode = 500
	db.GetDB().Save(authJson)
}

// InitiateOAuthFlow initiates a new OAuth flow for the given provider type
// Returns OAuthFlowInfo containing auth URL and session info
func (m *Manager) InitiateOAuthFlow(ctx context.Context, providerType model.OAuthProviderType, mode session.CallbackMode) (*OAuthFlowInfo, error) {
	// Generate PKCE codes
	pkceCodes, err := pkce.GeneratePKCECodes()
	if err != nil {
		return nil, fmt.Errorf("generate PKCE codes: %w", err)
	}

	// Create session
	sess, err := m.sessionManager.CreateSession(providerType, mode, pkceCodes, 0)
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}

	switch providerType {
	case model.OAuthProviderTypeCodex:
		return m.initiateCodexDeviceFlow(ctx, sess)
	default:
		return nil, fmt.Errorf("unsupported OAuth provider type: %s", providerType)
	}
}

// initiateCodexDeviceFlow initiates the Codex device flow
func (m *Manager) initiateCodexDeviceFlow(ctx context.Context, sess *session.OAuthSession) (*OAuthFlowInfo, error) {
	codexAuth := codex.NewAuth()

	// Request device code from OpenAI
	deviceResp, err := codexAuth.RequestDeviceCode(ctx)
	if err != nil {
		return nil, fmt.Errorf("request device code: %w", err)
	}

	deviceCode := strings.TrimSpace(deviceResp.UserCode)
	if deviceCode == "" {
		deviceCode = strings.TrimSpace(deviceResp.UserCodeAlt)
	}
	deviceAuthID := strings.TrimSpace(deviceResp.DeviceAuthID)
	if deviceCode == "" || deviceAuthID == "" {
		return nil, fmt.Errorf("device flow did not return required fields")
	}

	// Store device auth ID in session
	sess.DeviceAuthID = deviceAuthID

	pollInterval := codex.ParseDevicePollInterval(deviceResp.Interval)

	// Start background polling
	go m.pollDeviceAuth(context.Background(), sess.State, deviceAuthID, deviceCode, pollInterval)

	return &OAuthFlowInfo{
		AuthURL:         codex.DeviceVerificationURL,
		State:           sess.State,
		CallbackMode:    "device",
		ExpiresIn:       900, // 15 minutes
		Instructions:    "Visit the URL below and enter the device code to authorize.",
		DeviceCode:      deviceCode,
		VerificationURL: codex.DeviceVerificationURL,
	}, nil
}

// pollDeviceAuth polls OpenAI for device auth completion and exchanges code for tokens
func (m *Manager) pollDeviceAuth(ctx context.Context, state, deviceAuthID, userCode string, interval time.Duration) {
	codexAuth := codex.NewAuth()

	tokenResp, err := codexAuth.PollDeviceToken(ctx, deviceAuthID, userCode, interval)
	if err != nil {
		log.Warnf("device poll failed for state %s: %v", state, err)
		m.sessionManager.SetCallbackResult(state, "", err.Error())
		return
	}

	authCode := strings.TrimSpace(tokenResp.AuthorizationCode)
	codeVerifier := strings.TrimSpace(tokenResp.CodeVerifier)
	codeChallenge := strings.TrimSpace(tokenResp.CodeChallenge)
	if authCode == "" || codeVerifier == "" {
		m.sessionManager.SetCallbackResult(state, "", "device flow response missing required fields")
		return
	}

	// Store PKCE codes from device flow response
	sess, err := m.sessionManager.ValidateSession(state)
	if err != nil {
		m.sessionManager.SetCallbackResult(state, "", "session expired")
		return
	}
	sess.PKCECodes = &pkce.PKCECodes{
		CodeVerifier:  codeVerifier,
		CodeChallenge: codeChallenge,
	}

	// Exchange code for tokens
	tokenResult, err := codexAuth.ExchangeCodeWithRedirect(ctx, authCode, codeVerifier, codex.DeviceTokenExchangeRedirectURI)
	if err != nil {
		log.Warnf("device code exchange failed for state %s: %v", state, err)
		m.sessionManager.SetCallbackResult(state, "", fmt.Sprintf("token exchange failed: %v", err))
		return
	}

	// Build provider and save
	email := codex.ExtractEmailFromIDToken(tokenResult.IDToken)
	tokenData := codexAuth.BuildTokenData(tokenResult, email)
	tokenJSON, err := tokenData.ToJSON()
	if err != nil {
		m.sessionManager.SetCallbackResult(state, "", fmt.Sprintf("serialize token: %v", err))
		return
	}

	provider := &model.OAuthProvider{
		Name:           fmt.Sprintf("Codex-%s", email),
		ProviderType:   model.OAuthProviderTypeCodex,
		APIKey:         tokenResult.AccessToken,
		APIKeyExpireAt: tokenData.ExpiresAt,
		Status:         1,
		LastRefreshAt:  time.Now().Unix(),
	}
	authJson := &model.AuthJson{
		Content:         tokenJSON,
		Enabled:         true,
		StatusCode:      200,
		LastUseTimeStamp: time.Now().Unix(),
	}
	provider.AuthJsons = []model.AuthJson{*authJson}

	// Save to database
	createReq := &op.OAuthProviderCreateRequest{Provider: provider}
	if err := op.OAuthProviderCreate(createReq, ctx); err != nil {
		log.Warnf("device flow: failed to save provider: %v", err)
		m.sessionManager.SetCallbackResult(state, "", fmt.Sprintf("save provider: %v", err))
		return
	}

	// Store provider ID in session result for frontend to retrieve
	m.sessionManager.SetCallbackResult(state, fmt.Sprintf("provider:%d", provider.ID), "")
}

// HandleOAuthCallback handles the OAuth callback URL (manual mode)
// callbackURL is the full URL that user pasted (contains code and state)
func (m *Manager) HandleOAuthCallback(ctx context.Context, callbackURL, providerName string) (*model.OAuthProvider, error) {
	// Parse callback URL
	parsedURL, err := url.Parse(callbackURL)
	if err != nil {
		return nil, fmt.Errorf("parse callback URL: %w", err)
	}

	// Extract state and code
	state := parsedURL.Query().Get("state")
	code := parsedURL.Query().Get("code")
	errorMsg := parsedURL.Query().Get("error")

	if errorMsg != "" {
		return nil, fmt.Errorf("OAuth error: %s - %s", errorMsg, parsedURL.Query().Get("error_description"))
	}

	if state == "" || code == "" {
		return nil, fmt.Errorf("missing state or code in callback URL")
	}

	// Validate session
	sess, err := m.sessionManager.ValidateSession(state)
	if err != nil {
		return nil, fmt.Errorf("validate session: %w", err)
	}

	// Exchange code for token based on provider type
	var provider *model.OAuthProvider
	switch sess.ProviderType {
	case model.OAuthProviderTypeCodex:
		provider, err = m.exchangeCodexCode(ctx, sess, code)
	default:
		return nil, fmt.Errorf("unsupported provider type: %s", sess.ProviderType)
	}

	if err != nil {
		return nil, err
	}

	// Set provider name if provided
	if providerName != "" {
		provider.Name = providerName
	}

	// Complete session
	m.sessionManager.CompleteSession(state)

	return provider, nil
}

// HandleAutoCallback handles automatic callback from local server
// This is called when the local HTTP server receives a callback
func (m *Manager) HandleAutoCallback(ctx context.Context, state, code string) (*model.OAuthProvider, error) {
	// Validate session
	sess, err := m.sessionManager.ValidateSession(state)
	if err != nil {
		return nil, fmt.Errorf("validate session: %w", err)
	}

	// Exchange code for token
	var provider *model.OAuthProvider
	switch sess.ProviderType {
	case model.OAuthProviderTypeCodex:
		provider, err = m.exchangeCodexCode(ctx, sess, code)
	default:
		return nil, fmt.Errorf("unsupported provider type: %s", sess.ProviderType)
	}

	if err != nil {
		return nil, err
	}

	// Complete session
	m.sessionManager.CompleteSession(state)

	return provider, nil
}

// GetSessionStatus returns the status of an OAuth session
func (m *Manager) GetSessionStatus(state string) (*session.OAuthSession, error) {
	return m.sessionManager.ValidateSession(state)
}

// GetCallbackResult returns the callback result for auto mode
func (m *Manager) GetCallbackResult(state string) (bool, string, string) {
	return m.sessionManager.GetCallbackResult(state)
}

// exchangeCodexCode exchanges authorization code for Codex tokens
func (m *Manager) exchangeCodexCode(ctx context.Context, sess *session.OAuthSession, code string) (*model.OAuthProvider, error) {
	codexAuth := codex.NewAuth()

	redirectURI := "http://localhost:1455/auth/callback"
	if sess.CallbackMode == session.CallbackModeAuto && sess.CallbackPort > 0 {
		redirectURI = fmt.Sprintf("http://localhost:%d/auth/callback", sess.CallbackPort)
	}

	tokenResp, err := codexAuth.ExchangeCode(ctx, code, sess.PKCECodes.CodeVerifier, redirectURI)
	if err != nil {
		return nil, fmt.Errorf("exchange code: %w", err)
	}

	// Extract email from ID token
	email := codex.ExtractEmailFromIDToken(tokenResp.IDToken)

	// Build token data
	tokenData := codexAuth.BuildTokenData(tokenResp, email)
	tokenJSON, err := tokenData.ToJSON()
	if err != nil {
		return nil, fmt.Errorf("serialize token data: %w", err)
	}

	// Create OAuth provider
	provider := &model.OAuthProvider{
		Name:           fmt.Sprintf("Codex-%s", email),
		ProviderType:   model.OAuthProviderTypeCodex,
		APIKey:         tokenResp.AccessToken,
		APIKeyExpireAt: tokenData.ExpiresAt,
		Status:         1,
		LastRefreshAt:  time.Now().Unix(),
	}

	// Create AuthJson
	authJson := &model.AuthJson{
		Content:         tokenJSON,
		Enabled:         true,
		StatusCode:      200,
		LastUseTimeStamp: time.Now().Unix(),
	}

	provider.AuthJsons = []model.AuthJson{*authJson}

	return provider, nil
}

// refreshCodexToken handles Codex-specific token refresh
func (m *Manager) refreshCodexToken(ctx context.Context, provider *model.OAuthProvider) error {
	// Get active AuthJson
	authJson := provider.GetActiveAuthJson()
	if authJson == nil {
		m.recordFailure(provider)
		return fmt.Errorf("no active auth_json found")
	}

	// Extract RefreshToken from AuthJson
	refreshToken := authJson.GetCodexRefreshToken()
	if refreshToken == "" {
		m.recordFailure(provider)
		m.recordAuthJsonFailure(authJson)
		return fmt.Errorf("RefreshToken not found in auth_json")
	}

	// Refresh token
	codexAuth := codex.NewAuth()
	tokenResp, err := codexAuth.RefreshToken(ctx, refreshToken)
	if err != nil {
		m.recordFailure(provider)
		m.recordAuthJsonFailure(authJson)
		return fmt.Errorf("refresh token: %w", err)
	}

	// Validate response
	if tokenResp.AccessToken == "" {
		m.recordFailure(provider)
		m.recordAuthJsonFailure(authJson)
		return fmt.Errorf("refresh response missing access token")
	}

	// Update token data in AuthJson
	email := authJson.GetCodexEmail()
	tokenData := codexAuth.BuildTokenData(tokenResp, email)
	tokenJSON, err := tokenData.ToJSON()
	if err != nil {
		return fmt.Errorf("serialize token data: %w", err)
	}

	// Update provider with new data
	provider.APIKey = tokenResp.AccessToken
	provider.APIKeyExpireAt = tokenData.ExpiresAt
	provider.LastRefreshAt = time.Now().Unix()
	provider.RefreshFailCount = 0
	provider.Status = 1

	// Update AuthJson
	authJson.Content = tokenJSON
	authJson.StatusCode = 200
	authJson.LastUseTimeStamp = time.Now().Unix()

	// Save both provider and authJson
	tx := db.GetDB().Begin()
	if err := tx.Save(provider).Error; err != nil {
		tx.Rollback()
		return err
	}
	if err := tx.Save(authJson).Error; err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit().Error
}
