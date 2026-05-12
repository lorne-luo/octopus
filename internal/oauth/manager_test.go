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
	// AutoMigrate is called in InitDB, but we need to ensure AuthJson table exists
	db.GetDB().AutoMigrate(&model.AuthJson{})
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

	// Create auth_json content with BXAuth
	authJSONContent, _ := json.Marshal(map[string]string{"BXAuth": "test-cookie"})

	provider := &model.OAuthProvider{
		Name:         "test-provider",
		ProviderType: model.OAuthProviderTypeIFlow,
		AuthJsons: []model.AuthJson{
			{
				Content: string(authJSONContent),
				Enabled: true,
			},
		},
		Status: 1,
	}
	db.GetDB().Create(provider)
	// Create AuthJson records
	for i := range provider.AuthJsons {
		provider.AuthJsons[i].OAuthProviderID = provider.ID
		db.GetDB().Create(&provider.AuthJsons[i])
	}

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
	db.GetDB().Preload("AuthJsons").First(&updatedProvider, provider.ID)
	if updatedProvider.APIKey != "refreshed-key" {
		t.Errorf("expected db api key 'refreshed-key', got %s", updatedProvider.APIKey)
	}
	if updatedProvider.KeyName != "test-key-name" {
		t.Errorf("expected db key name 'test-key-name', got %s", updatedProvider.KeyName)
	}
}

func TestManager_RefreshAPIKey_NoAuthJsons(t *testing.T) {
	setupTestDB()

	manager := GetManager()

	provider := &model.OAuthProvider{
		Name:         "test-provider",
		ProviderType: model.OAuthProviderTypeIFlow,
		AuthJsons:    []model.AuthJson{},
		Status:       1,
	}
	db.GetDB().Create(provider)

	ctx := context.Background()
	err := manager.RefreshAPIKey(ctx, provider)
	if err == nil {
		t.Fatal("expected error for no auth_jsons")
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

func TestGetActiveAuthJson(t *testing.T) {
	// Case 1: No auth_jsons
	p1 := &model.OAuthProvider{AuthJsons: []model.AuthJson{}}
	if aj := p1.GetActiveAuthJson(); aj != nil {
		t.Error("expected nil for no auth_jsons")
	}

	// Case 2: All disabled
	p2 := &model.OAuthProvider{
		AuthJsons: []model.AuthJson{
			{Content: `{"BXAuth":"test"}`, Enabled: false},
		},
	}
	if aj := p2.GetActiveAuthJson(); aj != nil {
		t.Error("expected nil for all disabled")
	}

	// Case 3: StatusCode 200 preferred
	p3 := &model.OAuthProvider{
		AuthJsons: []model.AuthJson{
			{Content: `{"BXAuth":"test1"}`, Enabled: true, StatusCode: 200, LastUseTimeStamp: 1000},
			{Content: `{"BXAuth":"test2"}`, Enabled: true, StatusCode: 0, LastUseTimeStamp: 2000},
		},
	}
	aj3 := p3.GetActiveAuthJson()
	if aj3 == nil || aj3.GetBXAuth() != "test1" {
		t.Error("expected test1 for StatusCode 200 preferred")
	}

	// Case 4: Most recent LastUseTimeStamp among StatusCode 200
	p4 := &model.OAuthProvider{
		AuthJsons: []model.AuthJson{
			{Content: `{"BXAuth":"test1"}`, Enabled: true, StatusCode: 200, LastUseTimeStamp: 1000},
			{Content: `{"BXAuth":"test2"}`, Enabled: true, StatusCode: 200, LastUseTimeStamp: 2000},
		},
	}
	aj4 := p4.GetActiveAuthJson()
	if aj4 == nil || aj4.GetBXAuth() != "test2" {
		t.Error("expected test2 for most recent LastUseTimeStamp")
	}

	// Case 5: Empty content skipped
	p5 := &model.OAuthProvider{
		AuthJsons: []model.AuthJson{
			{Content: "", Enabled: true},
			{Content: `{"BXAuth":"test"}`, Enabled: true, StatusCode: 0},
		},
	}
	aj5 := p5.GetActiveAuthJson()
	if aj5 == nil || aj5.GetBXAuth() != "test" {
		t.Error("expected test for non-empty content")
	}
}
