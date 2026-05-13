package oauth

import (
	"context"
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/bestruirui/octopus/internal/db"
	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/oauth/callback"
	"github.com/bestruirui/octopus/internal/oauth/codex"
	"github.com/bestruirui/octopus/internal/oauth/kiro"
	"github.com/bestruirui/octopus/internal/oauth/pkce"
	"github.com/bestruirui/octopus/internal/oauth/session"
	"github.com/bestruirui/octopus/internal/utils/log"
	"golang.org/x/sync/singleflight"
)

type refreshResult struct {
	apiKey           string
	apiKeyExpireAt   int64
	status           int
	lastRefreshAt    int64
	refreshFailCount int
}

type Manager struct {
	sessionManager  *session.Manager
	callbackServers map[int]*callback.Server // Active callback servers by port
	serversMu       sync.Mutex
	refreshGroup    singleflight.Group
}

var (
	sharedManager *Manager
	once          sync.Once
)

func GetManager() *Manager {
	once.Do(func() {
		sharedManager = &Manager{
			sessionManager:  session.NewManager(),
			callbackServers: make(map[int]*callback.Server),
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
}

// RefreshAPIKey refreshes the API key for an OAuth provider
// For Kiro: it uses refresh token to get a new access token
// For Codex: it uses refresh token to get a new access token
func (m *Manager) RefreshAPIKey(ctx context.Context, provider *model.OAuthProvider) error {
	key := fmt.Sprintf("%d", provider.ID)
	ch := m.refreshGroup.DoChan(key, func() (any, error) {
		refreshCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		if err := m.refreshAPIKey(refreshCtx, provider); err != nil {
			return nil, err
		}
		return refreshResult{
			apiKey:           provider.APIKey,
			apiKeyExpireAt:   provider.APIKeyExpireAt,
			status:           provider.Status,
			lastRefreshAt:    provider.LastRefreshAt,
			refreshFailCount: provider.RefreshFailCount,
		}, nil
	})

	select {
	case result := <-ch:
		if result.Err != nil {
			return result.Err
		}
		refresh := result.Val.(refreshResult)
		provider.APIKey = refresh.apiKey
		provider.APIKeyExpireAt = refresh.apiKeyExpireAt
		provider.Status = refresh.status
		provider.LastRefreshAt = refresh.lastRefreshAt
		provider.RefreshFailCount = refresh.refreshFailCount
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m *Manager) refreshAPIKey(ctx context.Context, provider *model.OAuthProvider) error {
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

// codexCallbackPort is the fixed port for Codex OAuth callbacks.
// OpenAI only accepts redirect_uri with this port.
const codexCallbackPort = 1455

// InitiateOAuthFlow initiates a new OAuth flow for the given provider type
// Returns OAuthFlowInfo containing auth URL and session info
func (m *Manager) InitiateOAuthFlow(ctx context.Context, providerType model.OAuthProviderType, mode session.CallbackMode) (*OAuthFlowInfo, error) {
	// Generate PKCE codes
	pkceCodes, err := pkce.GeneratePKCECodes()
	if err != nil {
		return nil, fmt.Errorf("generate PKCE codes: %w", err)
	}

	var port int
	var callbackURL string
	var cbServer *callback.Server

	// Determine callback mode
	if mode == session.CallbackModeAuto {
		// Use fixed port 1455 for Codex (OpenAI requires this specific redirect_uri)
		port = codexCallbackPort
		callbackURL = fmt.Sprintf("http://localhost:%d/auth/callback", port)

		// Create and start callback server
		cbServer = callback.NewServer(port)
		_, err = cbServer.Start(ctx)
		if err != nil {
			// Fall back to manual mode
			mode = session.CallbackModeManual
			log.Warnf("failed to start callback server, falling back to manual mode: %v", err)
		} else {
			// Store the server for later cleanup
			m.serversMu.Lock()
			m.callbackServers[port] = cbServer
			m.serversMu.Unlock()
		}
	}

	// Create session
	sess, err := m.sessionManager.CreateSession(providerType, mode, pkceCodes, port)
	if err != nil {
		// Clean up server if session creation fails
		if cbServer != nil {
			_ = cbServer.Stop()
			m.serversMu.Lock()
			delete(m.callbackServers, port)
			m.serversMu.Unlock()
		}
		return nil, fmt.Errorf("create session: %w", err)
	}

	// For auto mode, start goroutine to wait for callback
	if mode == session.CallbackModeAuto && cbServer != nil {
		go m.waitForAutoCallback(ctx, sess.State, cbServer)
	}

	// Generate auth URL based on provider type
	switch providerType {
	case model.OAuthProviderTypeCodex:
		codexAuth := codex.NewAuth()
		if callbackURL == "" {
			callbackURL = "http://localhost:1455/auth/callback"
		}
		authURL := codexAuth.GetAuthURL(sess.State, pkceCodes.CodeChallenge, callbackURL)

		// Build response
		info := &OAuthFlowInfo{
			AuthURL:   authURL,
			State:     sess.State,
			ExpiresIn: 600, // 10 minutes
		}

		if mode == session.CallbackModeAuto {
			info.CallbackMode = "auto"
			info.CallbackPort = port
			info.Instructions = "Click the link below to authorize. The page will automatically detect when authorization is complete."
		} else {
			info.CallbackMode = "manual"
			info.Instructions = "Click the link below to authorize. After authorization, copy the URL from your browser's address bar and paste it in the callback URL field."
		}

		return info, nil
	default:
		// Clean up server for unsupported provider
		if cbServer != nil {
			_ = cbServer.Stop()
			m.serversMu.Lock()
			delete(m.callbackServers, port)
			m.serversMu.Unlock()
		}
		return nil, fmt.Errorf("unsupported OAuth provider type: %s", providerType)
	}
}

// waitForAutoCallback waits for the callback in auto mode
func (m *Manager) waitForAutoCallback(ctx context.Context, state string, cbServer *callback.Server) {
	defer func() {
		// Stop server when done
		_ = cbServer.Stop()
		m.serversMu.Lock()
		delete(m.callbackServers, cbServer.GetPort())
		m.serversMu.Unlock()
	}()

	// Wait for callback with 10 minute timeout (matching session TTL)
	result, err := cbServer.WaitForCallback(ctx, 10*time.Minute)
	if err != nil {
		log.Warnf("callback wait failed for state %s: %v", state, err)
		// Mark session with error
		m.sessionManager.SetCallbackResult(state, "", err.Error())
		return
	}

	// Store callback result in session
	if result.Error != "" {
		m.sessionManager.SetCallbackResult(state, "", result.Error)
		return
	}

	m.sessionManager.SetCallbackResult(state, result.Code, "")
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

	redirectURI := fmt.Sprintf("http://localhost:%d/auth/callback", codexCallbackPort)

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
		Content:          tokenJSON,
		Enabled:          true,
		StatusCode:       200,
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
