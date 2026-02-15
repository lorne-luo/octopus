package oauth

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/bestruirui/octopus/internal/db"
	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/oauth/iflow"
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
func (m *Manager) RefreshAPIKey(ctx context.Context, provider *model.OAuthProvider) error {
	if provider.ProviderType == "iflow" {
		// Step 1: Get API key info to obtain the correct key name
		keyInfo, err := iflow.FetchAPIKeyInfo(ctx, provider.Cookie)
		if err != nil {
			m.recordFailure(provider)
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
			return fmt.Errorf("no valid key name found (stored key name is empty and GET response name is empty)")
		}

		// Step 2: Refresh API key using POST with the correct key name
		resp, err := iflow.RefreshAPIKey(ctx, provider.Cookie, keyName)
		if err != nil {
			m.recordFailure(provider)
			return fmt.Errorf("refresh api key with name '%s': %w", keyName, err)
		}

		// Validate response has API key
		if resp.Data.APIKey == "" {
			m.recordFailure(provider)
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
			fmt.Printf("warning: failed to parse expire time: %v\n", err)
		} else {
			provider.APIKeyExpireAt = expireTime.Unix()
		}

		provider.LastRefreshAt = time.Now().Unix()
		provider.RefreshFailCount = 0
		provider.Status = 1

		return db.GetDB().Save(provider).Error
	}
	return fmt.Errorf("unknown provider type: %s", provider.ProviderType)
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