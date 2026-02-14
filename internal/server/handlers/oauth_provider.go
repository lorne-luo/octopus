package handlers

import (
	"context"
	"net/http"
	"strconv"

	"github.com/bestruirui/octopus/internal/helper"
	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/oauth"
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
	Name         string `json:"name" binding:"required"`
	ProviderType string `json:"provider_type" binding:"required"`
	Cookie       string `json:"cookie" binding:"required"`
	APIKey       string `json:"api_key"`
	Status       int    `json:"status"`
	Model        string `json:"model"`
	CustomModel  string `json:"custom_model"`
}

func createOAuthProvider(c *gin.Context) {
	var req CreateOAuthProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.ErrInvalidJSON)
		return
	}

	provider := &model.OAuthProvider{
		Name:         req.Name,
		ProviderType: req.ProviderType,
		Cookie:       req.Cookie,
		APIKey:       req.APIKey,
		Status:       req.Status,
		Model:        req.Model,
		CustomModel:  req.CustomModel,
	}

	if err := op.OAuthProviderCreate(provider, c.Request.Context()); err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	// Trigger immediate refresh to get API key if cookie is provided
	if provider.Cookie != "" && provider.APIKey == "" {
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

	if provider.APIKey == "" {
		resp.Error(c, http.StatusBadRequest, "provider has no api key, please refresh first")
		return
	}

	// Build Channel-like request for FetchModels
	// IFlow uses OpenAI-compatible API
	channel := model.Channel{
		Type:         outbound.OutboundTypeOpenAIChat,
		BaseUrls:     []model.BaseUrl{{URL: "https://apis.iflow.cn/v1"}},
		CustomHeader: []model.CustomHeader{},
	}
	channel.Keys = []model.ChannelKey{{
		Enabled:     true,
		ChannelKey:  provider.APIKey,
	}}

	models, err := helper.FetchModels(c.Request.Context(), channel)
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return
	}

	resp.Success(c, models)
}
