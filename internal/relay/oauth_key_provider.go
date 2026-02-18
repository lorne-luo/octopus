package relay

import (
	"context"
	"fmt"

	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/oauth"
)

func GetChannelKey(ctx context.Context, channel *model.Channel) (string, error) {
	if !channel.UseOAuth {
		return "", fmt.Errorf("channel does not use oauth")
	}

	if channel.OAuthProvider == nil {
		return "", fmt.Errorf("oauth provider not found for channel %s", channel.Name)
	}

	provider := channel.OAuthProvider
	manager := oauth.GetManager()

	if manager.ShouldRefresh(provider) {
		if err := manager.RefreshAPIKey(ctx, provider); err != nil {
			return "", fmt.Errorf("failed to refresh api key: %w", err)
		}
	}

	return provider.APIKey, nil
}
