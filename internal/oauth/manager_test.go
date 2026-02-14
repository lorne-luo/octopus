package oauth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bestruirui/octopus/internal/db"
	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/oauth/iflow"
)

func changeIFlowEndpoint(url string) {
	iflow.IFlowAPIKeyEndpoint = url
}

func setupTestDB() {
	// Initialize in-memory SQLite DB
	db.InitDB("sqlite", ":memory:", false)
	// AutoMigrate is called in InitDB
}

func TestManager_RefreshAPIKey(t *testing.T) {
	setupTestDB()

	// Mock IFlow server - handles both GET and POST
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			resp := iflow.IFlowAPIKeyResponse{
				Success: true,
				Data: struct {
					APIKey     string `json:"apiKey"`
					ExpireTime string `json:"expireTime"`
					HasExpired bool   `json:"hasExpired"`
					Name       string `json:"name"`
					APIKeyMask string `json:"apiKeyMask"`
				}{
					APIKey:     "existing-key",
					ExpireTime: "2025-01-01 12:00",
					HasExpired: false,
					Name:       "test-key-name",
					APIKeyMask: "sk-xxx",
				},
			}
			json.NewEncoder(w).Encode(resp)
			return
		}

		// POST request
		resp := iflow.IFlowAPIKeyResponse{
			Success: true,
			Data: struct {
				APIKey     string `json:"apiKey"`
				ExpireTime string `json:"expireTime"`
				HasExpired bool   `json:"hasExpired"`
				Name       string `json:"name"`
				APIKeyMask string `json:"apiKeyMask"`
			}{
				APIKey:     "refreshed-key",
				ExpireTime: "2025-02-01 12:00",
				HasExpired: false,
				Name:       "test-key-name",
				APIKeyMask: "sk-yyy",
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	changeIFlowEndpoint(server.URL)

	manager := GetManager()

	provider := &model.OAuthProvider{
		Name:         "test-provider",
		ProviderType: "iflow",
		Cookie:       "test-cookie",
		Status:       1,
	}
	db.GetDB().Create(provider)

	ctx := context.Background()
	err := manager.RefreshAPIKey(ctx, provider)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if provider.APIKey != "refreshed-key" {
		t.Errorf("expected api key 'refreshed-key', got %s", provider.APIKey)
	}

	if provider.KeyName != "test-key-name" {
		t.Errorf("expected key name 'test-key-name', got %s", provider.KeyName)
	}

	// Verify DB update
	var updatedProvider model.OAuthProvider
	db.GetDB().First(&updatedProvider, provider.ID)
	if updatedProvider.APIKey != "refreshed-key" {
		t.Errorf("expected db api key 'refreshed-key', got %s", updatedProvider.APIKey)
	}
	if updatedProvider.KeyName != "test-key-name" {
		t.Errorf("expected db key name 'test-key-name', got %s", updatedProvider.KeyName)
	}
}

func TestManager_ShouldRefresh(t *testing.T) {
	manager := GetManager()

	// Case 1: No API Key
	p1 := &model.OAuthProvider{APIKey: ""}
	if !manager.ShouldRefresh(p1) {
		t.Error("expected should refresh when no api key")
	}

	// Case 2: Expired
	p2 := &model.OAuthProvider{APIKey: "key", APIKeyExpireAt: time.Now().Unix() - 100}
	if !manager.ShouldRefresh(p2) {
		t.Error("expected should refresh when expired")
	}

	// Case 3: About to expire (within 1 hour)
	p3 := &model.OAuthProvider{APIKey: "key", APIKeyExpireAt: time.Now().Unix() + 1800} // 30 mins
	if !manager.ShouldRefresh(p3) {
		t.Error("expected should refresh when about to expire")
	}

	// Case 4: Valid
	p4 := &model.OAuthProvider{APIKey: "key", APIKeyExpireAt: time.Now().Unix() + 7200} // 2 hours
	if manager.ShouldRefresh(p4) {
		t.Error("expected shouldn't refresh when valid")
	}
}
