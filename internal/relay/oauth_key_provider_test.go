package relay

import (
	"context"
	"testing"

	"github.com/bestruirui/octopus/internal/model"
)

func TestGetChannelKey_NotOAuth(t *testing.T) {
	channel := &model.Channel{
		Name:    "test-channel",
		UseOAuth: false,
	}

	_, err := GetChannelKey(context.Background(), channel)
	if err == nil {
		t.Error("expected error for non-OAuth channel")
	}
	if err.Error() != "channel does not use oauth" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestGetChannelKey_NoProvider(t *testing.T) {
	channel := &model.Channel{
		Name:          "test-channel",
		UseOAuth:      true,
		OAuthProvider: nil,
	}

	_, err := GetChannelKey(context.Background(), channel)
	if err == nil {
		t.Error("expected error for channel without OAuth provider")
	}
}

func TestGetChannelKey_ProviderWithAPIKey(t *testing.T) {
	channel := &model.Channel{
		Name:     "test-channel",
		UseOAuth: true,
		OAuthProvider: &model.OAuthProvider{
			ID:       1,
			Name:     "test-provider",
			APIKey:   "existing-api-key",
			Status:   1,
			APIKeyExpireAt: 9999999999, // Far future
		},
	}

	key, err := GetChannelKey(context.Background(), channel)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if key != "existing-api-key" {
		t.Errorf("expected 'existing-api-key', got %q", key)
	}
}
