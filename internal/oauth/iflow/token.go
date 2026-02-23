package iflow

import (
	"fmt"
	"strings"
)

// NormalizeCookie normalizes raw cookie strings for iFlow authentication flows.
// It validates that the cookie contains BXAuth field and returns only the BXAuth value.
// The returned value is just the raw value without "BXAuth=" prefix.
func NormalizeCookie(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("cookie cannot be empty")
	}

	// Extract only BXAuth value
	bxAuth := ExtractBXAuth(trimmed)
	if bxAuth == "" {
		return "", fmt.Errorf("cookie missing BXAuth field, please ensure cookie starts with BXAuth=")
	}

	return bxAuth, nil
}

// ExtractBXAuth extracts the BXAuth value from a cookie string.
func ExtractBXAuth(cookie string) string {
	parts := strings.Split(cookie, ";")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "BXAuth=") {
			return strings.TrimPrefix(part, "BXAuth=")
		}
	}
	return cookie
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
