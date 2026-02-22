package op

import (
	"context"
	"fmt"

	"github.com/bestruirui/octopus/internal/db"
	log "github.com/bestruirui/octopus/internal/utils/log"
	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/transformer/outbound"
)

func OAuthProviderList(ctx context.Context) ([]model.OAuthProvider, error) {
	var providers []model.OAuthProvider
	if err := db.GetDB().WithContext(ctx).Preload("AuthJsons").Find(&providers).Error; err != nil {
		return nil, err
	}

	// Populate channel data for each provider
	for i := range providers {
		channel, err := ChannelGetByOAuthProviderID(providers[i].ID, ctx)
		if err != nil {
			log.Warnf("OAuthProviderList: Channel not found for provider %d: %v", providers[i].ID, err)
			continue
		}
		if channel != nil {
			providers[i].Channel = &model.OAuthProviderChannel{
				ID:          channel.ID,
				Model:       channel.Model,
				CustomModel: channel.CustomModel,
				MatchRegex:  channel.MatchRegex,
				Enabled:     channel.Enabled,
			}
			log.Infof("OAuthProviderList: Provider %d has channel %d with Model=%s, CustomModel=%s",
				providers[i].ID, channel.ID, channel.Model, channel.CustomModel)
		}
	}

	return providers, nil
}

// OAuthProviderCreateRequest contains all data needed to create an OAuth provider with channel
type OAuthProviderCreateRequest struct {
	Provider    *model.OAuthProvider
	Model       string
	CustomModel string
	MatchRegex  *string
	AuthJsons   []model.AuthJsonAddRequest // Initial AuthJsons to create
}

func OAuthProviderCreate(req *OAuthProviderCreateRequest, ctx context.Context) error {
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
	if err := tx.Create(req.Provider).Error; err != nil {
		tx.Rollback()
		return err
	}

	// Create AuthJsons if provided
	for _, addReq := range req.AuthJsons {
		authJson := &model.AuthJson{
			OAuthProviderID: req.Provider.ID,
			Content:         addReq.Content,
			Enabled:         addReq.Enabled,
			Remark:          addReq.Remark,
		}
		if err := tx.Create(authJson).Error; err != nil {
			log.Warnf("OAuthProviderCreate: Failed to create auth_json: %v", err)
		}
	}

	// Create a corresponding Channel with UseOAuth=true
	channel := &model.Channel{
		Name:            fmt.Sprintf("OAuth-%s", req.Provider.Name),
		Type:            outbound.OutboundTypeOpenAIChat, // Default to OpenAI Chat type
		Enabled:         req.Provider.Status == 1,        // Enable if provider is active
		BaseUrls:        []model.BaseUrl{{URL: req.Provider.GetBaseURL(), Delay: 0}},
		Keys:            []model.ChannelKey{}, // OAuth channels don't use keys directly
		Model:           req.Model,
		CustomModel:     req.CustomModel,
		MatchRegex:      req.MatchRegex,
		Proxy:           false,
		AutoSync:        false,
		AutoGroup:       model.AutoGroupTypeNone,
		CustomHeader:    []model.CustomHeader{},
		UseOAuth:        true,
		OAuthProviderID: req.Provider.ID,
	}

	log.Infof("OAuthProviderCreate: Creating channel for provider %d with Model=%s, CustomModel=%s",
		req.Provider.ID, req.Model, req.CustomModel)

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

	// Populate channel data in provider for response
	req.Provider.Channel = &model.OAuthProviderChannel{
		ID:          channel.ID,
		Model:       channel.Model,
		CustomModel: channel.CustomModel,
		MatchRegex:  channel.MatchRegex,
		Enabled:     channel.Enabled,
	}

	// Reload AuthJsons for response
	db.GetDB().WithContext(ctx).Preload("AuthJsons").First(req.Provider, req.Provider.ID)

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
	if req.Status != nil {
		updates["status"] = *req.Status
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

	// Handle AuthJsons to add
	for _, addReq := range req.AuthJsonsToAdd {
		authJson := &model.AuthJson{
			OAuthProviderID: provider.ID,
			Content:         addReq.Content,
			Enabled:         addReq.Enabled,
			Remark:          addReq.Remark,
		}
		if err := tx.Create(authJson).Error; err != nil {
			log.Warnf("OAuthProviderUpdate: Failed to create auth_json: %v", err)
		}
	}

	// Handle AuthJsons to update
	for _, updateReq := range req.AuthJsonsToUpdate {
		authJsonUpdates := map[string]interface{}{}
		if updateReq.Enabled != nil {
			authJsonUpdates["enabled"] = *updateReq.Enabled
		}
		if updateReq.Content != nil {
			authJsonUpdates["content"] = *updateReq.Content
		}
		if updateReq.Remark != nil {
			authJsonUpdates["remark"] = *updateReq.Remark
		}
		if len(authJsonUpdates) > 0 {
			if err := tx.Model(&model.AuthJson{}).Where("id = ?", updateReq.ID).Updates(authJsonUpdates).Error; err != nil {
				log.Warnf("OAuthProviderUpdate: Failed to update auth_json %d: %v", updateReq.ID, err)
			}
		}
	}

	// Handle AuthJsons to delete
	if len(req.AuthJsonsToDelete) > 0 {
		if err := tx.Where("id IN ?", req.AuthJsonsToDelete).Delete(&model.AuthJson{}).Error; err != nil {
			log.Warnf("OAuthProviderUpdate: Failed to delete auth_jsons: %v", err)
		}
	}

	// Update the corresponding Channel
	var channel model.Channel
	var channelIDToRefresh int // Track channel ID for cache refresh
	err := tx.Where("use_o_auth = ? AND o_auth_provider_id = ?", true, provider.ID).First(&channel).Error
	if err != nil {
		// Log the error but don't fail - this is for debugging
		// Channel might not exist for providers created before this feature
		log.Warnf("OAuthProviderUpdate: Channel not found for provider %d: %v", provider.ID, err)
	}
	if err == nil {
		// Channel exists, update it
		channelUpdates := map[string]interface{}{}
		// Prepare log values
		modelVal := "(nil)"
		if req.Model != nil {
			modelVal = *req.Model
		}
		customModelVal := "(nil)"
		if req.CustomModel != nil {
			customModelVal = *req.CustomModel
		}
		log.Infof("OAuthProviderUpdate: Found channel %d for provider %d, Model=%s, CustomModel=%s, MatchRegex=%v",
			channel.ID, provider.ID, modelVal, customModelVal, req.MatchRegex)

		if req.Name != nil {
			channelUpdates["name"] = fmt.Sprintf("OAuth-%s", *req.Name)
		}
		if req.Status != nil {
			channelUpdates["enabled"] = (*req.Status == 1)
		}
		if req.BaseURL != nil {
			channelUpdates["base_urls"] = []model.BaseUrl{{URL: *req.BaseURL, Delay: 0}}
		}
		if req.Model != nil {
			channelUpdates["model"] = *req.Model
		}
		if req.CustomModel != nil {
			channelUpdates["custom_model"] = *req.CustomModel
		}
		if req.MatchRegex != nil {
			channelUpdates["match_regex"] = *req.MatchRegex
		}

		if len(channelUpdates) > 0 {
			if err := tx.Model(&channel).Updates(channelUpdates).Error; err != nil {
				tx.Rollback()
				return nil, err
			}
			channelIDToRefresh = channel.ID
		}
	}

	// Commit the transaction
	if err := tx.Commit().Error; err != nil {
		return nil, err
	}

	// Refresh channel cache if it was updated
	if channelIDToRefresh != 0 {
		if err := channelRefreshCacheByID(channelIDToRefresh, ctx); err != nil {
			log.Warnf("OAuthProviderUpdate: Failed to refresh channel cache: %v", err)
		}
	}

	// Reload provider with AuthJsons
	db.GetDB().WithContext(ctx).Preload("AuthJsons").First(&provider, provider.ID)

	// Populate channel data for response
	ch, err := ChannelGetByOAuthProviderID(provider.ID, ctx)
	if err == nil && ch != nil {
		provider.Channel = &model.OAuthProviderChannel{
			ID:          ch.ID,
			Model:       ch.Model,
			CustomModel: ch.CustomModel,
			MatchRegex:  ch.MatchRegex,
			Enabled:     ch.Enabled,
		}
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

	// Delete AuthJsons first
	if err := tx.Where("oauth_provider_id = ?", id).Delete(&model.AuthJson{}).Error; err != nil {
		log.Warnf("OAuthProviderDelete: Failed to delete auth_jsons for provider %d: %v", id, err)
	}

	// Delete the corresponding Channel first (if exists)
	var channel model.Channel
	err := tx.Where("use_o_auth = ? AND o_auth_provider_id = ?", true, id).First(&channel).Error
	if err != nil {
		log.Warnf("OAuthProviderDelete: Channel not found for provider %d: %v", id, err)
	} else {
		// Channel exists, delete it
		log.Infof("OAuthProviderDelete: Deleting channel %d for provider %d", channel.ID, id)
		if err := tx.Delete(&channel).Error; err != nil {
			tx.Rollback()
			return err
		}
		// Clear cache
		channelCache.Del(channel.ID)
	}

	// Delete the OAuth Provider
	log.Infof("OAuthProviderDelete: Deleting provider %d", id)
	if err := tx.Delete(&model.OAuthProvider{}, id).Error; err != nil {
		tx.Rollback()
		return err
	}

	// Commit the transaction
	if err := tx.Commit().Error; err != nil {
		return err
	}

	log.Infof("OAuthProviderDelete: Successfully deleted provider %d", id)
	return nil
}

func OAuthProviderGet(id int, ctx context.Context) (*model.OAuthProvider, error) {
	var provider model.OAuthProvider
	if err := db.GetDB().WithContext(ctx).Preload("AuthJsons").First(&provider, id).Error; err != nil {
		return nil, err
	}

	// Populate channel data
	channel, err := ChannelGetByOAuthProviderID(provider.ID, ctx)
	if err == nil && channel != nil {
		provider.Channel = &model.OAuthProviderChannel{
			ID:          channel.ID,
			Model:       channel.Model,
			CustomModel: channel.CustomModel,
			MatchRegex:  channel.MatchRegex,
			Enabled:     channel.Enabled,
		}
	}

	return &provider, nil
}
