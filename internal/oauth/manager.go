package oauth

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/bestruirui/octopus/internal/db"
	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/oauth/iflow"
	"github.com/bestruirui/octopus/internal/oauth/kiro"
	"github.com/bestruirui/octopus/internal/utils/log"
)

type Manager struct{}

var (
	sharedManager *Manager
	once          sync.Once
)

func GetManager() *Manager {
	once.Do(func() {
		sharedManager = &Manager{}
	})
	return sharedManager
}

// RefreshAPIKey refreshes the API key for an OAuth provider
// For IFlow: it first fetches key info via GET, then refreshes via POST
// For Kiro: it uses refresh token to get a new access token
func (m *Manager) RefreshAPIKey(ctx context.Context, provider *model.OAuthProvider) error {
	switch provider.ProviderType {
	case model.OAuthProviderTypeIFlow:
		return m.refreshIFlowAPIKey(ctx, provider)
	case model.OAuthProviderTypeKiro:
		return m.refreshKiroToken(ctx, provider)
	default:
		return fmt.Errorf("unsupported provider type: %s", provider.ProviderType)
	}
}

// refreshIFlowAPIKey handles iFlow-specific token refresh
func (m *Manager) refreshIFlowAPIKey(ctx context.Context, provider *model.OAuthProvider) error {
	// Get active AuthJson
	authJson := provider.GetActiveAuthJson()
	if authJson == nil {
		m.recordFailure(provider)
		return fmt.Errorf("no active auth_json found")
	}

	// Extract BXAuth from AuthJson
	bxAuth := authJson.GetBXAuth()
	if bxAuth == "" {
		m.recordFailure(provider)
		return fmt.Errorf("BXAuth not found in auth_json")
	}

	// Step 1: Get API key info to obtain the correct key name
	keyInfo, err := iflow.FetchAPIKeyInfo(ctx, bxAuth)
	if err != nil {
		m.recordFailure(provider)
		m.recordAuthJsonFailure(authJson)
		return fmt.Errorf("fetch api key info: %w", err)
	}

	// Use stored key name or the one from GET response
	keyName := provider.KeyName
	if keyName == "" {
		keyName = keyInfo.Data.Name
	}

	// Validate key name is not empty
	if keyName == "" {
		m.recordFailure(provider)
		m.recordAuthJsonFailure(authJson)
		return fmt.Errorf("no valid key name found (stored key name is empty and GET response name is empty)")
	}

	// Step 2: Refresh API key using POST with the correct key name
	resp, err := iflow.RefreshAPIKey(ctx, bxAuth, keyName)
	if err != nil {
		m.recordFailure(provider)
		m.recordAuthJsonFailure(authJson)
		return fmt.Errorf("refresh api key with name '%s': %w", keyName, err)
	}

	// Validate response has API key
	if resp.Data.APIKey == "" {
		m.recordFailure(provider)
		m.recordAuthJsonFailure(authJson)
		return fmt.Errorf("refresh response missing api key")
	}

	// Update provider with new data
	provider.APIKey = resp.Data.APIKey
	provider.KeyName = resp.Data.Name

	// Parse expiration time
	// IFlow format example: "2025-01-01 00:00"
	expireTime, err := time.Parse("2006-01-02 15:04", resp.Data.ExpireTime)
	if err != nil {
		// Log error but proceed - we got a valid key
		log.Warnf("failed to parse expire time: %v", err)
	} else {
		provider.APIKeyExpireAt = expireTime.Unix()
	}

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
