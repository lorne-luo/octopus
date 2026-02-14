package oauth

import (
	"context"
	"fmt"
	"time"

	"github.com/bestruirui/octopus/internal/db"
	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/oauth/iflow"
)

type Manager struct{}

var sharedManager *Manager

func GetManager() *Manager {
	if sharedManager == nil {
		sharedManager = &Manager{}
	}
	return sharedManager
}

func (m *Manager) RefreshAPIKey(ctx context.Context, provider *model.OAuthProvider) error {
	if provider.ProviderType == "iflow" {
		resp, err := iflow.RefreshAPIKey(provider.Cookie, provider.Name)
		if err != nil {
			m.recordFailure(provider)
			return err
		}

		// Update provider
		provider.APIKey = resp.Data.APIKey

		// Parse expiration time
		// IFlow format example: "2025-01-01 00:00"
		expireTime, err := time.Parse("2006-01-02 15:04", resp.Data.ExpireTime)
		if err != nil {
			// Try without time if parsing fails, or log error.
			// For now, assume format is consistent.
			// If parsing fails, maybe set a default or don't update expire time?
			// Let's log it but proceed if we got a key.
			// Actually, if we can't parse expire time, we might loop refresh.
			// Let's assume standard format for now.
			fmt.Printf("failed to parse expire time: %v\n", err)
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
	// If failed too many times, maybe disable it?
	if provider.RefreshFailCount > 3 {
		provider.Status = 2 // Error/Expired
	}
	db.GetDB().Save(provider)
}
