package model

import (
	"encoding/json"
	"fmt"
)

// OAuthProviderType represents the type of OAuth provider
type OAuthProviderType int

const (
	OAuthProviderTypeIFlow OAuthProviderType = iota + 1
	OAuthProviderTypeKiro  // Future: Kiro OAuth
)

// String returns the string representation of the provider type
func (t OAuthProviderType) String() string {
	switch t {
	case OAuthProviderTypeIFlow:
		return "iflow"
	case OAuthProviderTypeKiro:
		return "kiro"
	default:
		return "unknown"
	}
}

// ParseOAuthProviderType parses a string to OAuthProviderType
func ParseOAuthProviderType(s string) (OAuthProviderType, error) {
	switch s {
	case "iflow":
		return OAuthProviderTypeIFlow, nil
	case "kiro":
		return OAuthProviderTypeKiro, nil
	default:
		return 0, fmt.Errorf("unknown oauth provider type: %s", s)
	}
}

// MarshalJSON implements json.Marshaler for OAuthProviderType
func (t OAuthProviderType) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.String())
}

// UnmarshalJSON implements json.Unmarshaler for OAuthProviderType
func (t *OAuthProviderType) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	parsed, err := ParseOAuthProviderType(s)
	if err != nil {
		return err
	}
	*t = parsed
	return nil
}

// GormDataType implements gorm's data type interface
func (OAuthProviderType) GormDataType() string {
	return "integer"
}

type OAuthProvider struct {
	ID               int               `gorm:"primaryKey" json:"id"`
	Name             string            `gorm:"size:255;not null" json:"name"`
	ProviderType     OAuthProviderType `gorm:"not null" json:"provider_type"`
	AuthJSON         string            `gorm:"type:text" json:"-"`                    // JSON-formatted auth credentials
	KeyName          string            `gorm:"size:255" json:"-"`                     // iFlow key name for refresh
	APIKey           string            `gorm:"size:255" json:"api_key"`
	APIKeyExpireAt   int64             `json:"api_key_expire_at"`
	Status           int               `gorm:"default:1" json:"status"` // 1: Active, 2: Expired, 0: Disabled
	LastRefreshAt    int64             `json:"last_refresh_at"`
	RefreshFailCount int               `json:"refresh_fail_count"`
	CreatedAt        int64             `json:"created_at"`
	UpdatedAt        int64             `json:"updated_at"`
	BaseURL          string            `gorm:"size:255" json:"base_url"`
	Channel          *OAuthProviderChannel `gorm:"-" json:"channel,omitempty"` // Not a DB field, populated on demand
}

// OAuthProviderChannel contains channel data for OAuth provider response
type OAuthProviderChannel struct {
	ID          int     `json:"id"`
	Model       string  `json:"model"`
	CustomModel string  `json:"custom_model"`
	MatchRegex  *string `json:"match_regex,omitempty"`
	Enabled     bool    `json:"enabled"`
}

type OAuthProviderUpdateRequest struct {
	ID           int                `json:"id" binding:"required"`
	Name         *string            `json:"name,omitempty"`
	ProviderType *OAuthProviderType `json:"provider_type,omitempty"`
	AuthJSON     *string            `json:"auth_json,omitempty"`
	Status       *int               `json:"status,omitempty"`
	BaseURL      *string            `json:"base_url,omitempty"`
	Model        *string            `json:"model,omitempty"`
	CustomModel  *string            `json:"custom_model,omitempty"`
	MatchRegex   *string            `json:"match_regex,omitempty"`
}

// GetBXAuth extracts BXAuth from AuthJSON for iFlow provider
func (p *OAuthProvider) GetBXAuth() string {
	if p.AuthJSON == "" {
		return ""
	}
	var data struct {
		BXAuth string `json:"BXAuth"`
	}
	if err := json.Unmarshal([]byte(p.AuthJSON), &data); err != nil {
		return ""
	}
	return data.BXAuth
}

// SetBXAuth sets BXAuth in AuthJSON for iFlow provider
func (p *OAuthProvider) SetBXAuth(bxAuth string) error {
	data := map[string]string{"BXAuth": bxAuth}
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}
	p.AuthJSON = string(jsonData)
	return nil
}

// GetBaseURL returns the base URL for the OAuth Provider
// Returns configured BaseURL or default based on ProviderType
func (p *OAuthProvider) GetBaseURL() string {
	if p.BaseURL != "" {
		return p.BaseURL
	}
	switch p.ProviderType {
	case OAuthProviderTypeIFlow:
		return "https://apis.iflow.cn/v1"
	case OAuthProviderTypeKiro:
		// Kiro uses dynamic region-based URLs, return empty for now
		return ""
	default:
		return ""
	}
}
