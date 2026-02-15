package iflow

import (
	"fmt"
	"strings"
)

// NormalizeCookie normalizes raw cookie strings for iFlow authentication flows.
// It validates that the cookie contains BXAuth field and formats it properly.
func NormalizeCookie(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("cookie cannot be empty")
	}

	combined := strings.Join(strings.Fields(trimmed), " ")
	if !strings.HasSuffix(combined, ";") {
		combined += ";"
	}
	if !strings.Contains(combined, "BXAuth=") {
		return "", fmt.Errorf("cookie missing BXAuth field, please ensure cookie starts with BXAuth=")
	}
	return combined, nil
}

type IFlowTokenStorage struct {
	APIKey      string `json:"api_key"`
	Email       string `json:"email"`
	Expire      string `json:"expire"` // Format: "2006-01-02 15:04"
	Cookie      string `json:"cookie"`
	LastRefresh string `json:"last_refresh"`
}

type IFlowAPIKeyResponse struct {
	Success bool   `json:"success"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Data    struct {
		APIKey     string `json:"apiKey"`
		ExpireTime string `json:"expireTime"`
		HasExpired bool   `json:"hasExpired"`
		Name       string `json:"name"`
		APIKeyMask string `json:"apiKeyMask"`
	} `json:"data"`
}
