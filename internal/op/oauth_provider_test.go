package op

import (
	"context"
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

	// Create a test OAuth provider
	provider := &model.OAuthProvider{
		Name:         "Test Provider",
		ProviderType: "iflow",
		Cookie:       "test-cookie",
		Status:       1,
		Model:        "gpt-4,gpt-3.5-turbo",
		CustomModel:  "custom-model",
	}

	err := OAuthProviderCreate(provider, ctx)
	require.NoError(t, err)
	require.NotZero(t, provider.ID)

	// Verify that a channel was created
	var channel model.Channel
	err = db.GetDB().Where("use_oauth = ? AND oauth_provider_id = ?", true, provider.ID).
		First(&channel).Error
	require.NoError(t, err)

	// Verify channel properties
	assert.Equal(t, "OAuth-Test Provider", channel.Name)
	assert.True(t, channel.UseOAuth)
	assert.Equal(t, provider.ID, channel.OAuthProviderID)
	assert.True(t, channel.Enabled)
	assert.Equal(t, provider.Model, channel.Model)
	assert.Equal(t, provider.CustomModel, channel.CustomModel)

	// Cleanup
	_ = OAuthProviderDelete(provider.ID, ctx)
}

func TestOAuthProviderUpdateWithChannel(t *testing.T) {
	if db.GetDB() == nil {
		t.Skip("Database not available")
	}

	ctx := context.Background()

	// Create a test OAuth provider
	provider := &model.OAuthProvider{
		Name:         "Test Provider",
		ProviderType: "iflow",
		Cookie:       "test-cookie",
		Status:       1,
		Model:        "gpt-4",
	}

	err := OAuthProviderCreate(provider, ctx)
	require.NoError(t, err)

	// Update the provider
	newName := "Updated Provider"
	newStatus := 0
	newModel := "gpt-4,gpt-3.5-turbo"

	updateReq := &model.OAuthProviderUpdateRequest{
		ID:     provider.ID,
		Name:   &newName,
		Status: &newStatus,
		Model:  &newModel,
	}

	updatedProvider, err := OAuthProviderUpdate(updateReq, ctx)
	require.NoError(t, err)
	assert.Equal(t, newName, updatedProvider.Name)

	// Verify that the channel was also updated
	var channel model.Channel
	err = db.GetDB().Where("use_oauth = ? AND oauth_provider_id = ?", true, provider.ID).
		First(&channel).Error
	require.NoError(t, err)

	assert.Equal(t, "OAuth-Updated Provider", channel.Name)
	assert.False(t, channel.Enabled) // Status 0 means disabled
	assert.Equal(t, newModel, channel.Model)

	// Cleanup
	_ = OAuthProviderDelete(provider.ID, ctx)
}

func TestOAuthProviderDeleteWithChannel(t *testing.T) {
	if db.GetDB() == nil {
		t.Skip("Database not available")
	}

	ctx := context.Background()

	// Create a test OAuth provider
	provider := &model.OAuthProvider{
		Name:         "Test Provider",
		ProviderType: "iflow",
		Cookie:       "test-cookie",
		Status:       1,
	}

	err := OAuthProviderCreate(provider, ctx)
	require.NoError(t, err)

	providerID := provider.ID

	// Verify channel exists
	var channel model.Channel
	err = db.GetDB().Where("use_oauth = ? AND oauth_provider_id = ?", true, providerID).
		First(&channel).Error
	require.NoError(t, err)

	// Delete the provider
	err = OAuthProviderDelete(providerID, ctx)
	require.NoError(t, err)

	// Verify that the channel was also deleted
	err = db.GetDB().Where("use_oauth = ? AND oauth_provider_id = ?", true, providerID).
		First(&channel).Error
	assert.Error(t, err) // Should not find the channel
}
