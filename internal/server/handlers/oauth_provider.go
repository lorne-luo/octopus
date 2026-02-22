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

// validateAndNormalizeAuthJsonContent validates and normalizes auth_json content for a given provider type.
// Returns the normalized content or an error message.
func validateAndNormalizeAuthJsonContent(content string, providerType model.OAuthProviderType) (string, string) {
	// Validate JSON format
	if !json.Valid([]byte(content)) {
		return "", "auth_json content must be valid JSON"
	}

	// Provider-specific validation and normalization
	if providerType == model.OAuthProviderTypeIFlow {
		var data map[string]interface{}
		if err := json.Unmarshal([]byte(content), &data); err != nil {
			return "", "failed to parse auth_json content: " + err.Error()
		}
		bxAuthRaw, ok := data["BXAuth"]
		if !ok {
			return "", "auth_json must contain BXAuth field for iFlow provider"
		}
		bxAuth, ok := bxAuthRaw.(string)
		if !ok {
			return "", "BXAuth must be a string"
		}
		normalizedBXAuth, err := iflow.NormalizeCookie(bxAuth)
		if err != nil {
			return "", err.Error()
		}
		// Rebuild auth_json with normalized value
		normalizedContent, _ := json.Marshal(map[string]string{"BXAuth": normalizedBXAuth})
		return string(normalizedContent), ""
	}

	return content, ""
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
	Name         string                    `json:"name" binding:"required"`
	ProviderType model.OAuthProviderType   `json:"provider_type" binding:"required"`
	APIKey       string                    `json:"api_key"`
	Status       int                       `json:"status"`
	Model        string                    `json:"model"`
	CustomModel  string                    `json:"custom_model"`
	MatchRegex   *string                   `json:"match_regex"`
	AuthJsons    []model.AuthJsonAddRequest `json:"auth_jsons"`
}

func createOAuthProvider(c *gin.Context) {
	var req CreateOAuthProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.ErrInvalidJSON)
		return
	}

	// Validate and normalize AuthJsons
	authJsons := make([]model.AuthJsonAddRequest, 0, len(req.AuthJsons))
	for _, aj := range req.AuthJsons {
		content, errMsg := validateAndNormalizeAuthJsonContent(aj.Content, req.ProviderType)
		if errMsg != "" {
			resp.Error(c, http.StatusBadRequest, errMsg)
			return
		}

		authJsons = append(authJsons, model.AuthJsonAddRequest{
			Enabled: aj.Enabled,
			Content: content,
			Remark:  aj.Remark,
		})
	}

	provider := &model.OAuthProvider{
		Name:         req.Name,
		ProviderType: req.ProviderType,
		APIKey:       req.APIKey,
		Status:       req.Status,
	}

	createReq := &op.OAuthProviderCreateRequest{
		Provider:    provider,
		Model:       req.Model,
		CustomModel: req.CustomModel,
		MatchRegex:  req.MatchRegex,
		AuthJsons:   authJsons,
	}

	if err := op.OAuthProviderCreate(createReq, c.Request.Context()); err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return
	}

	// Trigger immediate refresh to get API key if auth_jsons are provided
	if len(provider.AuthJsons) > 0 && provider.APIKey == "" {
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

	// Get existing provider to check type
	existingProvider, err := op.OAuthProviderGet(req.ID, c.Request.Context())
	if err != nil {
		resp.Error(c, http.StatusNotFound, "provider not found")
		return
	}

	// Validate and normalize AuthJsons to add
	for i, aj := range req.AuthJsonsToAdd {
		content, errMsg := validateAndNormalizeAuthJsonContent(aj.Content, existingProvider.ProviderType)
		if errMsg != "" {
			resp.Error(c, http.StatusBadRequest, errMsg)
			return
		}
		req.AuthJsonsToAdd[i].Content = content
	}

	// Validate and normalize AuthJsons to update
	for _, aj := range req.AuthJsonsToUpdate {
		if aj.Content != nil && *aj.Content != "" {
			content, errMsg := validateAndNormalizeAuthJsonContent(*aj.Content, existingProvider.ProviderType)
			if errMsg != "" {
				resp.Error(c, http.StatusBadRequest, errMsg)
				return
			}
			*aj.Content = content
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
			resp.Error(c, http.StatusBadRequest, "failed to refresh api key: "+err.Error())
			return
		}
	}

	// Build Channel-like request for FetchModels
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
