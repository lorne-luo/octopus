package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/bestruirui/octopus/internal/helper"
	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/oauth"
	"github.com/bestruirui/octopus/internal/oauth/iflow"
	"github.com/bestruirui/octopus/internal/op"
	"github.com/bestruirui/octopus/internal/server/middleware"
	"github.com/bestruirui/octopus/internal/server/resp"
	"github.com/bestruirui/octopus/internal/server/router"
	"github.com/bestruirui/octopus/internal/transformer/outbound"
	"github.com/gin-gonic/gin"
)

func init() {
	router.NewGroupRouter("/api/v1/oauth-provider").
		Use(middleware.Auth()).
		Use(middleware.RequireJSON()).
		AddRoute(
			router.NewRoute("/list", http.MethodGet).
				Handle(listOAuthProvider),
		).
		AddRoute(
			router.NewRoute("/create", http.MethodPost).
				Handle(createOAuthProvider),
		).
		AddRoute(
			router.NewRoute("/update", http.MethodPost).
				Handle(updateOAuthProvider),
		).
		AddRoute(
			router.NewRoute("/delete/:id", http.MethodDelete).
				Handle(deleteOAuthProvider),
		).
		AddRoute(
			router.NewRoute("/refresh", http.MethodPost).
				Handle(refreshOAuthProvider),
		).
		AddRoute(
			router.NewRoute("/fetch-model", http.MethodPost).
				Handle(fetchOAuthProviderModels),
		)
}

func listOAuthProvider(c *gin.Context) {
	providers, err := op.OAuthProviderList(c.Request.Context())
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	resp.Success(c, providers)
}

type CreateOAuthProviderRequest struct {
	Name         string                `json:"name" binding:"required"`
	ProviderType model.OAuthProviderType `json:"provider_type" binding:"required"`
	AuthJSON     string                `json:"auth_json" binding:"required"`
	APIKey       string                `json:"api_key"`
	Status       int                   `json:"status"`
	Model        string                `json:"model"`
	CustomModel  string                `json:"custom_model"`
	MatchRegex   *string               `json:"match_regex"`
}

func createOAuthProvider(c *gin.Context) {
	var req CreateOAuthProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.ErrInvalidJSON)
		return
	}

	// Validate AuthJSON format
	if req.AuthJSON == "" {
		resp.Error(c, http.StatusBadRequest, "auth_json is required")
		return
	}

	// Validate JSON format
	if !json.Valid([]byte(req.AuthJSON)) {
		resp.Error(c, http.StatusBadRequest, "auth_json must be valid JSON")
		return
	}

	// Provider-specific validation
	authJSON := req.AuthJSON
	if req.ProviderType == model.OAuthProviderTypeIFlow {
		// Extract and validate BXAuth for iFlow
		var data map[string]interface{}
		if err := json.Unmarshal([]byte(req.AuthJSON), &data); err != nil {
			resp.Error(c, http.StatusBadRequest, "failed to parse auth_json: "+err.Error())
			return
		}
		bxAuthRaw, ok := data["BXAuth"]
		if !ok {
			resp.Error(c, http.StatusBadRequest, "auth_json must contain BXAuth field for iFlow provider")
			return
		}
		bxAuth, ok := bxAuthRaw.(string)
		if !ok {
			resp.Error(c, http.StatusBadRequest, "BXAuth must be a string")
			return
		}
		// Normalize the BXAuth value
		normalizedBXAuth, err := iflow.NormalizeCookie(bxAuth)
		if err != nil {
			resp.Error(c, http.StatusBadRequest, err.Error())
			return
		}
		// Rebuild auth_json with normalized value
		normalizedAuthJSON, _ := json.Marshal(map[string]string{"BXAuth": normalizedBXAuth})
		authJSON = string(normalizedAuthJSON)
	}

	provider := &model.OAuthProvider{
		Name:         req.Name,
		ProviderType: req.ProviderType,
		AuthJSON:     authJSON,
		APIKey:       req.APIKey,
		Status:       req.Status,
	}

	createReq := &op.OAuthProviderCreateRequest{
		Provider:    provider,
		Model:       req.Model,
		CustomModel: req.CustomModel,
		MatchRegex:  req.MatchRegex,
	}

	if err := op.OAuthProviderCreate(createReq, c.Request.Context()); err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	// Trigger immediate refresh to get API key if auth_json is provided
	if provider.AuthJSON != "" && provider.APIKey == "" {
		manager := oauth.GetManager()
		go func() {
			_ = manager.RefreshAPIKey(context.Background(), provider)
		}()
	}
	resp.Success(c, provider)
}

func updateOAuthProvider(c *gin.Context) {
	var req model.OAuthProviderUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.ErrInvalidJSON)
		return
	}

	// Validate auth_json format if provided
	if req.AuthJSON != nil && *req.AuthJSON != "" {
		if !json.Valid([]byte(*req.AuthJSON)) {
			resp.Error(c, http.StatusBadRequest, "auth_json must be valid JSON")
			return
		}

		// Get existing provider to check type
		existingProvider, err := op.OAuthProviderGet(req.ID, c.Request.Context())
		if err != nil {
			resp.Error(c, http.StatusNotFound, "provider not found")
			return
		}

		// Provider-specific validation
		if existingProvider.ProviderType == model.OAuthProviderTypeIFlow {
			var data map[string]interface{}
			if err := json.Unmarshal([]byte(*req.AuthJSON), &data); err != nil {
				resp.Error(c, http.StatusBadRequest, "failed to parse auth_json: "+err.Error())
				return
			}
			bxAuthRaw, ok := data["BXAuth"]
			if !ok {
				resp.Error(c, http.StatusBadRequest, "auth_json must contain BXAuth field for iFlow provider")
				return
			}
			bxAuth, ok := bxAuthRaw.(string)
			if !ok {
				resp.Error(c, http.StatusBadRequest, "BXAuth must be a string")
				return
			}
			// Normalize the BXAuth value
			normalizedBXAuth, err := iflow.NormalizeCookie(bxAuth)
			if err != nil {
				resp.Error(c, http.StatusBadRequest, err.Error())
				return
			}
			// Rebuild auth_json with normalized value
			normalizedAuthJSON, _ := json.Marshal(map[string]string{"BXAuth": normalizedBXAuth})
			*req.AuthJSON = string(normalizedAuthJSON)
		}
	}

	provider, err := op.OAuthProviderUpdate(&req, c.Request.Context())
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	resp.Success(c, provider)
}

func deleteOAuthProvider(c *gin.Context) {
	id := c.Param("id")
	idNum, err := strconv.Atoi(id)
	if err != nil {
		resp.Error(c, http.StatusBadRequest, resp.ErrInvalidParam)
		return
	}
	if err := op.OAuthProviderDelete(idNum, c.Request.Context()); err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	resp.Success(c, nil)
}

func refreshOAuthProvider(c *gin.Context) {
	var req struct {
		ID int `json:"id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.ErrInvalidJSON)
		return
	}

	provider, err := op.OAuthProviderGet(req.ID, c.Request.Context())
	if err != nil {
		resp.Error(c, http.StatusNotFound, "provider not found")
		return
	}

	manager := oauth.GetManager()
	if err := manager.RefreshAPIKey(c.Request.Context(), provider); err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return
	}

	resp.Success(c, provider)
}

func fetchOAuthProviderModels(c *gin.Context) {
	var req struct {
		ID int `json:"id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.ErrInvalidJSON)
		return
	}

	provider, err := op.OAuthProviderGet(req.ID, c.Request.Context())
	if err != nil {
		resp.Error(c, http.StatusNotFound, "provider not found")
		return
	}

	// If no API key, try to refresh first
	if provider.APIKey == "" {
		manager := oauth.GetManager()
		if err := manager.RefreshAPIKey(c.Request.Context(), provider); err != nil {

			resp.Error(c, http.StatusBadRequest, "failed to refresh api key2: "+err.Error())
			return
		}
	}

	// Build Channel-like request for FetchModels
	// IFlow uses OpenAI-compatible API
	channel := model.Channel{
		Type:         outbound.OutboundTypeOpenAIChat,
		BaseUrls:     []model.BaseUrl{{URL: provider.GetBaseURL()}},
		CustomHeader: []model.CustomHeader{},
	}
	channel.Keys = []model.ChannelKey{{
		Enabled:    true,
		ChannelKey: provider.APIKey,
	}}

	models, err := helper.FetchModels(c.Request.Context(), channel)
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return
	}

	resp.Success(c, models)
}
