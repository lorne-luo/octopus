package op

import (
	"context"

	"github.com/bestruirui/octopus/internal/db"
	"github.com/bestruirui/octopus/internal/model"
)

func OAuthProviderList(ctx context.Context) ([]model.OAuthProvider, error) {
	var providers []model.OAuthProvider
	if err := db.GetDB().WithContext(ctx).Find(&providers).Error; err != nil {
		return nil, err
	}
	return providers, nil
}

func OAuthProviderCreate(provider *model.OAuthProvider, ctx context.Context) error {
	if err := db.GetDB().WithContext(ctx).Create(provider).Error; err != nil {
		return err
	}
	return nil
}

func OAuthProviderUpdate(req *model.OAuthProviderUpdateRequest, ctx context.Context) (*model.OAuthProvider, error) {
	var provider model.OAuthProvider
	if err := db.GetDB().WithContext(ctx).First(&provider, req.ID).Error; err != nil {
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

	if len(updates) > 0 {
		if err := db.GetDB().WithContext(ctx).Model(&provider).Updates(updates).Error; err != nil {
			return nil, err
		}
	}

	return &provider, nil
}

func OAuthProviderDelete(id int, ctx context.Context) error {
	if err := db.GetDB().WithContext(ctx).Delete(&model.OAuthProvider{}, id).Error; err != nil {
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
