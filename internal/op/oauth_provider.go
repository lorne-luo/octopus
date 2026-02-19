package op

import (
	"context"
	"fmt"

	"github.com/bestruirui/octopus/internal/db"
	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/transformer/outbound"
)

func OAuthProviderList(ctx context.Context) ([]model.OAuthProvider, error) {
	var providers []model.OAuthProvider
	if err := db.GetDB().WithContext(ctx).Find(&providers).Error; err != nil {
		return nil, err
	}
	return providers, nil
}

func OAuthProviderCreate(provider *model.OAuthProvider, ctx context.Context) error {
	// Start a transaction to ensure both OAuth Provider and Channel are created together
	tx := db.GetDB().WithContext(ctx).Begin()
	if tx.Error != nil {
		return tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Create the OAuth Provider
	if err := tx.Create(provider).Error; err != nil {
		tx.Rollback()
		return err
	}

	// Create a corresponding Channel with UseOAuth=true
	channel := &model.Channel{
		Name:            fmt.Sprintf("OAuth-%s", provider.Name),
		Type:            outbound.OutboundTypeOpenAIChat, // Default to OpenAI Chat type
		Enabled:         provider.Status == 1,            // Enable if provider is active
		BaseUrls:        []model.BaseUrl{{URL: provider.GetBaseURL(), Delay: 0}},
		Keys:            []model.ChannelKey{}, // OAuth channels don't use keys directly
		Model:           provider.Model,
		CustomModel:     provider.CustomModel,
		Proxy:           false,
		AutoSync:        false,
		AutoGroup:       model.AutoGroupTypeNone,
		CustomHeader:    []model.CustomHeader{},
		UseOAuth:        true,
		OAuthProviderID: provider.ID,
	}

	if err := tx.Create(channel).Error; err != nil {
		tx.Rollback()
		return err
	}

	// Commit the transaction
	if err := tx.Commit().Error; err != nil {
		return err
	}

	// Update cache
	channelCache.Set(channel.ID, *channel)

	return nil
}

func OAuthProviderUpdate(req *model.OAuthProviderUpdateRequest, ctx context.Context) (*model.OAuthProvider, error) {
	// Start a transaction
	tx := db.GetDB().WithContext(ctx).Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	var provider model.OAuthProvider
	if err := tx.First(&provider, req.ID).Error; err != nil {
		tx.Rollback()
		return nil, err
	}

	updates := map[string]interface{}{}
	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.ProviderType != nil {
		updates["provider_type"] = *req.ProviderType
	}
	if req.Cookie != nil {
		updates["cookie"] = *req.Cookie
	}
	if req.Status != nil {
		updates["status"] = *req.Status
	}
	if req.Model != nil {
		updates["model"] = *req.Model
	}
	if req.CustomModel != nil {
		updates["custom_model"] = *req.CustomModel
	}
	if req.BaseURL != nil {
		updates["base_url"] = *req.BaseURL
	}

	if len(updates) > 0 {
		if err := tx.Model(&provider).Updates(updates).Error; err != nil {
			tx.Rollback()
			return nil, err
		}
	}

	// Update the corresponding Channel
	var channel model.Channel
	if err := tx.Where("use_oauth = ? AND oauth_provider_id = ?", true, provider.ID).First(&channel).Error; err == nil {
		// Channel exists, update it
		channelUpdates := map[string]interface{}{}

		if req.Name != nil {
			channelUpdates["name"] = fmt.Sprintf("OAuth-%s", *req.Name)
		}
		if req.Status != nil {
			channelUpdates["enabled"] = (*req.Status == 1)
		}
		if req.Model != nil {
			channelUpdates["model"] = *req.Model
		}
		if req.CustomModel != nil {
			channelUpdates["custom_model"] = *req.CustomModel
		}
		if req.BaseURL != nil {
			channelUpdates["base_urls"] = []model.BaseUrl{{URL: *req.BaseURL, Delay: 0}}
		}

		if len(channelUpdates) > 0 {
			if err := tx.Model(&channel).Updates(channelUpdates).Error; err != nil {
				tx.Rollback()
				return nil, err
			}
			// Update cache
			channelCache.Del(channel.ID)
		}
	}

	// Commit the transaction
	if err := tx.Commit().Error; err != nil {
		return nil, err
	}

	return &provider, nil
}

func OAuthProviderDelete(id int, ctx context.Context) error {
	// Start a transaction
	tx := db.GetDB().WithContext(ctx).Begin()
	if tx.Error != nil {
		return tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Delete the corresponding Channel first (if exists)
	var channel model.Channel
	if err := tx.Where("use_oauth = ? AND oauth_provider_id = ?", true, id).First(&channel).Error; err == nil {
		// Channel exists, delete it
		if err := tx.Delete(&channel).Error; err != nil {
			tx.Rollback()
			return err
		}
		// Clear cache
		channelCache.Del(channel.ID)
	}

	// Delete the OAuth Provider
	if err := tx.Delete(&model.OAuthProvider{}, id).Error; err != nil {
		tx.Rollback()
		return err
	}

	// Commit the transaction
	if err := tx.Commit().Error; err != nil {
		return err
	}

	return nil
}

func OAuthProviderGet(id int, ctx context.Context) (*model.OAuthProvider, error) {
	var provider model.OAuthProvider
	if err := db.GetDB().WithContext(ctx).First(&provider, id).Error; err != nil {
		return nil, err
	}
	return &provider, nil
}
