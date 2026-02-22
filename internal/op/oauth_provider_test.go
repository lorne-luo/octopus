package op

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/bestruirui/octopus/internal/db"
	"github.com/bestruirui/octopus/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOAuthProviderCreateWithChannel(t *testing.T) {
	// This test requires a test database setup
	// Skip if DB is not available
	if db.GetDB() == nil {
		t.Skip("Database not available")
	}

	ctx := context.Background()

	// Create auth_json content with BXAuth
	authJSONContent, _ := json.Marshal(map[string]string{"BXAuth": "test-cookie"})

	// Create a test OAuth provider
	provider := &model.OAuthProvider{
		Name:         "Test Provider",
		ProviderType: model.OAuthProviderTypeIFlow,
		Status:       1,
	}

	createReq := &OAuthProviderCreateRequest{
		Provider:    provider,
		Model:       "gpt-4,gpt-3.5-turbo",
		CustomModel: "custom-model",
		AuthJsons: []model.AuthJsonAddRequest{
			{Enabled: true, Content: string(authJSONContent), Remark: "test credential"},
		},
	}

	err := OAuthProviderCreate(createReq, ctx)
	require.NoError(t, err)
	require.NotZero(t, provider.ID)

	// Verify that a channel was created
	var channel model.Channel
	err = db.GetDB().Where("use_o_auth = ? AND o_auth_provider_id = ?", true, provider.ID).
		First(&channel).Error
	require.NoError(t, err)

	// Verify channel properties
	assert.Equal(t, "OAuth-Test Provider", channel.Name)
	assert.True(t, channel.UseOAuth)
	assert.Equal(t, provider.ID, channel.OAuthProviderID)
	assert.True(t, channel.Enabled)
	assert.Equal(t, "gpt-4,gpt-3.5-turbo", channel.Model)
	assert.Equal(t, "custom-model", channel.CustomModel)

	// Verify AuthJsons were created
	var authJsons []model.AuthJson
	err = db.GetDB().Where("oauth_provider_id = ?", provider.ID).Find(&authJsons).Error
	require.NoError(t, err)
	assert.Len(t, authJsons, 1)
	assert.Equal(t, string(authJSONContent), authJsons[0].Content)
	assert.True(t, authJsons[0].Enabled)

	// Cleanup
	_ = OAuthProviderDelete(provider.ID, ctx)
}

func TestOAuthProviderUpdateWithChannel(t *testing.T) {
	if db.GetDB() == nil {
		t.Skip("Database not available")
	}

	ctx := context.Background()

	// Create auth_json content with BXAuth
	authJSONContent, _ := json.Marshal(map[string]string{"BXAuth": "test-cookie"})

	// Create a test OAuth provider
	provider := &model.OAuthProvider{
		Name:         "Test Provider",
		ProviderType: model.OAuthProviderTypeIFlow,
		Status:       1,
	}

	createReq := &OAuthProviderCreateRequest{
		Provider:    provider,
		Model:       "gpt-4",
		CustomModel: "",
		AuthJsons: []model.AuthJsonAddRequest{
			{Enabled: true, Content: string(authJSONContent)},
		},
	}

	err := OAuthProviderCreate(createReq, ctx)
	require.NoError(t, err)

	// Update the provider
	newName := "Updated Provider"
	newStatus := 0
	newModel := "gpt-4,gpt-3.5-turbo"

	// Add a new auth_json
	newAuthJSONContent, _ := json.Marshal(map[string]string{"BXAuth": "test-cookie-2"})

	updateReq := &model.OAuthProviderUpdateRequest{
		ID:     provider.ID,
		Name:   &newName,
		Status: &newStatus,
		Model:  &newModel,
		AuthJsonsToAdd: []model.AuthJsonAddRequest{
			{Enabled: true, Content: string(newAuthJSONContent), Remark: "second credential"},
		},
	}

	updatedProvider, err := OAuthProviderUpdate(updateReq, ctx)
	require.NoError(t, err)
	assert.Equal(t, newName, updatedProvider.Name)

	// Verify that the channel was also updated
	var channel model.Channel
	err = db.GetDB().Where("use_o_auth = ? AND o_auth_provider_id = ?", true, provider.ID).
		First(&channel).Error
	require.NoError(t, err)

	assert.Equal(t, "OAuth-Updated Provider", channel.Name)
	assert.False(t, channel.Enabled) // Status 0 means disabled
	assert.Equal(t, newModel, channel.Model)

	// Verify AuthJsons were added
	var authJsons []model.AuthJson
	err = db.GetDB().Where("oauth_provider_id = ?", provider.ID).Find(&authJsons).Error
	require.NoError(t, err)
	assert.Len(t, authJsons, 2)

	// Cleanup
	_ = OAuthProviderDelete(provider.ID, ctx)
}

func TestOAuthProviderDeleteWithChannel(t *testing.T) {
	if db.GetDB() == nil {
		t.Skip("Database not available")
	}

	ctx := context.Background()

	// Create auth_json content with BXAuth
	authJSONContent, _ := json.Marshal(map[string]string{"BXAuth": "test-cookie"})

	// Create a test OAuth provider
	provider := &model.OAuthProvider{
		Name:         "Test Provider",
		ProviderType: model.OAuthProviderTypeIFlow,
		Status:       1,
	}

	createReq := &OAuthProviderCreateRequest{
		Provider: provider,
		AuthJsons: []model.AuthJsonAddRequest{
			{Enabled: true, Content: string(authJSONContent)},
		},
	}

	err := OAuthProviderCreate(createReq, ctx)
	require.NoError(t, err)

	providerID := provider.ID

	// Verify channel exists
	var channel model.Channel
	err = db.GetDB().Where("use_o_auth = ? AND o_auth_provider_id = ?", true, providerID).
		First(&channel).Error
	require.NoError(t, err)

	// Verify auth_jsons exist
	var authJsons []model.AuthJson
	err = db.GetDB().Where("oauth_provider_id = ?", providerID).Find(&authJsons).Error
	require.NoError(t, err)
	assert.Len(t, authJsons, 1)

	// Delete the provider
	err = OAuthProviderDelete(providerID, ctx)
	require.NoError(t, err)

	// Verify that the channel was also deleted
	err = db.GetDB().Where("use_o_auth = ? AND o_auth_provider_id = ?", true, providerID).
		First(&channel).Error
	assert.Error(t, err) // Should not find the channel

	// Verify that auth_jsons were also deleted
	err = db.GetDB().Where("oauth_provider_id = ?", providerID).Find(&authJsons).Error
	require.NoError(t, err)
	assert.Len(t, authJsons, 0)
}
