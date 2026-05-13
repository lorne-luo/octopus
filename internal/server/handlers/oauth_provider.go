package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/bestruirui/octopus/internal/helper"
	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/oauth"
	"github.com/bestruirui/octopus/internal/oauth/session"
	"github.com/bestruirui/octopus/internal/op"
	"github.com/bestruirui/octopus/internal/server/middleware"
	"github.com/bestruirui/octopus/internal/server/resp"
	"github.com/bestruirui/octopus/internal/server/router"
	"github.com/bestruirui/octopus/internal/transformer2/adapter"
	log "github.com/bestruirui/octopus/internal/utils/log"
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
		).
		AddRoute(
			router.NewRoute("/auth-url", http.MethodGet).
				Handle(getOAuthAuthURL),
		).
		AddRoute(
			router.NewRoute("/callback", http.MethodPost).
				Handle(handleOAuthCallback),
		).
		AddRoute(
			router.NewRoute("/callback-status", http.MethodGet).
				Handle(getOAuthCallbackStatus),
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
	if providerType == model.OAuthProviderTypeKiro {
		var data map[string]interface{}
		if err := json.Unmarshal([]byte(content), &data); err != nil {
			return "", "failed to parse auth_json content: " + err.Error()
		}
		refreshTokenRaw, ok := data["RefreshToken"]
		if !ok {
			return "", "auth_json must contain RefreshToken field for Kiro provider"
		}
		refreshToken, ok := refreshTokenRaw.(string)
		if !ok {
			return "", "RefreshToken must be a string"
		}
		if refreshToken == "" {
			return "", "RefreshToken cannot be empty"
		}
		// Region is optional, defaults to us-east-1
		// Validate Region format if provided
		if regionRaw, ok := data["Region"]; ok {
			region, ok := regionRaw.(string)
			if !ok {
				return "", "Region must be a string"
			}
			// Basic validation for AWS region format
			if region != "" && !isValidAWSRegion(region) {
				return "", "Region must be a valid AWS region (e.g., us-east-1, us-west-2)"
			}
		}
		// Content is valid as-is
		return content, ""
	}

	return content, ""
}

// isValidAWSRegion validates AWS region format
func isValidAWSRegion(region string) bool {
	// Common AWS regions
	validRegions := map[string]bool{
		"us-east-1":      true,
		"us-east-2":      true,
		"us-west-1":      true,
		"us-west-2":      true,
		"eu-west-1":      true,
		"eu-west-2":      true,
		"eu-west-3":      true,
		"eu-central-1":   true,
		"eu-central-2":   true,
		"ap-northeast-1": true,
		"ap-northeast-2": true,
		"ap-northeast-3": true,
		"ap-southeast-1": true,
		"ap-southeast-2": true,
		"ap-south-1":     true,
		"sa-east-1":      true,
		"ca-central-1":   true,
	}
	return validRegions[region]
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
	Name         string                     `json:"name" binding:"required"`
	ProviderType model.OAuthProviderType    `json:"provider_type" binding:"required"`
	APIKey       string                     `json:"api_key"`
	Status       int                        `json:"status"`
	Model        string                     `json:"model"`
	CustomModel  string                     `json:"custom_model"`
	MatchRegex   *string                    `json:"match_regex"`
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

	log.Infof("updateOAuthProvider: received request with ID=%d, AuthJsonsToAdd=%d, AuthJsonsToUpdate=%d, AuthJsonsToDelete=%d",
		req.ID, len(req.AuthJsonsToAdd), len(req.AuthJsonsToUpdate), len(req.AuthJsonsToDelete))

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
	for i, aj := range req.AuthJsonsToUpdate {
		if aj.Content != nil && *aj.Content != "" {
			content, errMsg := validateAndNormalizeAuthJsonContent(*aj.Content, existingProvider.ProviderType)
			if errMsg != "" {
				resp.Error(c, http.StatusBadRequest, errMsg)
				return
			}
			*req.AuthJsonsToUpdate[i].Content = content
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

	// Refresh API key if needed (empty, expired, or within 1 hour of expiration)
	manager := oauth.GetManager()
	if manager.ShouldRefresh(provider) {
		if err := manager.RefreshAPIKey(c.Request.Context(), provider); err != nil {
			resp.Error(c, http.StatusBadRequest, "failed to refresh api key: "+err.Error())
			return
		}
	}

	// Build Channel-like request for FetchModels
	channel := model.Channel{
		Type:         adapter.ProviderOpenAIChat,
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

// getOAuthAuthURL returns the OAuth authorization URL for the user to visit
// GET /api/v1/oauth-provider/auth-url?type=codex&mode=auto|manual
func getOAuthAuthURL(c *gin.Context) {
	providerTypeStr := c.Query("type")
	if providerTypeStr == "" {
		resp.Error(c, http.StatusBadRequest, "missing provider type")
		return
	}

	providerType, err := model.ParseOAuthProviderType(providerTypeStr)
	if err != nil {
		resp.Error(c, http.StatusBadRequest, "invalid provider type: "+providerTypeStr)
		return
	}

	// Determine callback mode
	modeStr := c.Query("mode")
	var mode session.CallbackMode
	switch modeStr {
	case "auto":
		mode = session.CallbackModeAuto
	case "manual":
		mode = session.CallbackModeManual
	default:
		// Auto-detect: try auto mode first
		mode = session.CallbackModeAuto
	}

	manager := oauth.GetManager()
	flowInfo, err := manager.InitiateOAuthFlow(c.Request.Context(), providerType, mode)
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return
	}

	resp.Success(c, flowInfo)
}

// HandleCallbackRequest represents the request body for handleOAuthCallback
type HandleCallbackRequest struct {
	CallbackURL string `json:"callback_url" binding:"required"`
	Name        string `json:"name"`
}

// handleOAuthCallback handles the OAuth callback URL (manual mode)
// POST /api/v1/oauth-provider/callback
func handleOAuthCallback(c *gin.Context) {
	var req HandleCallbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.ErrInvalidJSON)
		return
	}

	manager := oauth.GetManager()
	provider, err := manager.HandleOAuthCallback(c.Request.Context(), req.CallbackURL, req.Name)
	if err != nil {
		resp.Error(c, http.StatusBadRequest, err.Error())
		return
	}

	// Save the provider to database
	createReq := &op.OAuthProviderCreateRequest{
		Provider: provider,
	}
	if err := op.OAuthProviderCreate(createReq, c.Request.Context()); err != nil {
		resp.Error(c, http.StatusInternalServerError, "failed to save provider: "+err.Error())
		return
	}

	resp.Success(c, gin.H{
		"provider": provider,
		"message":  "OAuth provider created successfully",
	})
}

// getOAuthCallbackStatus returns the status of an OAuth session (for auto mode polling)
// GET /api/v1/oauth-provider/callback-status?state=xxx
func getOAuthCallbackStatus(c *gin.Context) {
	state := c.Query("state")
	if state == "" {
		resp.Error(c, http.StatusBadRequest, "missing state parameter")
		return
	}

	manager := oauth.GetManager()
	sess, err := manager.GetSessionStatus(state)
	if err != nil {
		resp.Success(c, gin.H{
			"status": "expired",
			"error":  err.Error(),
		})
		return
	}

	// Check if this is auto mode and callback has been received
	if sess.CallbackMode == session.CallbackModeAuto {
		completed, code, callbackErr := manager.GetCallbackResult(state)
		if completed {
			if callbackErr != "" {
				resp.Success(c, gin.H{
					"status": "error",
					"error":  callbackErr,
				})
				return
			}

			// Exchange code for provider
			provider, err := manager.HandleAutoCallback(c.Request.Context(), state, code)
			if err != nil {
				resp.Success(c, gin.H{
					"status": "error",
					"error":  err.Error(),
				})
				return
			}

			// Save the provider to database
			createReq := &op.OAuthProviderCreateRequest{
				Provider: provider,
			}
			if err := op.OAuthProviderCreate(createReq, c.Request.Context()); err != nil {
				resp.Success(c, gin.H{
					"status": "error",
					"error":  "failed to save provider: " + err.Error(),
				})
				return
			}

			resp.Success(c, gin.H{
				"status":   "completed",
				"provider": provider,
			})
			return
		}
	}

	// Session is still pending
	resp.Success(c, gin.H{
		"status":        "pending",
		"provider_type": sess.ProviderType.String(),
		"expires_at":    sess.ExpiresAt.Unix(),
	})
}
