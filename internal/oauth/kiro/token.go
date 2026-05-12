package kiro

import (
	"encoding/json"
	"time"
)

// TokenData represents token information stored in AuthJson
type TokenData struct {
	AccessToken  string   `json:"access_token"`
	RefreshToken string   `json:"refresh_token"`
	ProfileArn   string   `json:"profile_arn"`
	Region       string   `json:"region"`
	ExpiresAt    string   `json:"expires_at"`
	Scopes       []string `json:"scopes"`
}

// TokenRefreshResponse represents the response from Kiro Desktop Auth refresh endpoint
type TokenRefreshResponse struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken,omitempty"`
	ExpiresIn    int    `json:"expiresIn"`
	TokenType    string `json:"tokenType,omitempty"`
	ProfileArn   string `json:"profileArn,omitempty"`
}

// AuthJsonContent represents the expected content format in AuthJson.Content
// Example: {"RefreshToken": "xxx...", "Region": "us-east-1"}
type AuthJsonContent struct {
	RefreshToken string `json:"RefreshToken"`
	Region       string `json:"Region"`
}

// ParseAuthJsonContent parses AuthJson content string into AuthJsonContent struct
func ParseAuthJsonContent(content string) (*AuthJsonContent, error) {
	if content == "" {
		return nil, nil
	}

	var data AuthJsonContent
	if err := json.Unmarshal([]byte(content), &data); err != nil {
		return nil, err
	}
	return &data, nil
}

// GetDefaultBaseURL returns the default base URL for Kiro API
func GetDefaultBaseURL(region string) string {
	if region == "" {
		region = "us-east-1"
	}
	return "https://q." + region + ".amazonaws.com"
}

// GetRefreshURL returns the refresh URL for Kiro Desktop Auth
func GetRefreshURL(region string) string {
	if region == "" {
		region = "us-east-1"
	}
	return "https://prod." + region + ".auth.desktop.kiro.dev/refreshToken"
}

// CalculateExpirationTime calculates expiration time from expiresIn seconds
// with a 60-second buffer before actual expiration
func CalculateExpirationTime(expiresIn int) time.Time {
	if expiresIn <= 60 {
		return time.Now().Add(time.Duration(expiresIn) * time.Second)
	}
	return time.Now().Add(time.Duration(expiresIn-60) * time.Second)
}
