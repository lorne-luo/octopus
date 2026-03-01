package model

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"time"
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

// AuthJson represents a single auth credential for an OAuth provider
type AuthJson struct {
	ID               int    `gorm:"primaryKey" json:"id"`
	OAuthProviderID  int    `gorm:"not null;index" json:"oauth_provider_id"`
	Content          string `gorm:"type:text;not null" json:"content"`          // JSON auth credentials
	Enabled          bool   `gorm:"default:true" json:"enabled"`
	StatusCode       int    `gorm:"default:0" json:"status_code"`              // 200=success, 429=rate limited, etc.
	LastUseTimeStamp int64  `json:"last_use_time_stamp"`
	TotalToken       int64  `json:"total_token"`
	Remark           string `gorm:"size:255" json:"remark"`
}

// TableName specifies the table name for AuthJson
func (AuthJson) TableName() string {
	return "auth_jsons"
}

// GetBXAuth extracts BXAuth from Content for iFlow provider
func (aj *AuthJson) GetBXAuth() string {
	if aj.Content == "" {
		return ""
	}
	var data struct {
		BXAuth string `json:"BXAuth"`
	}
	if err := json.Unmarshal([]byte(aj.Content), &data); err != nil {
		return ""
	}
	return data.BXAuth
}

// GetRefreshToken extracts RefreshToken from Content for Kiro provider
func (aj *AuthJson) GetRefreshToken() string {
	if aj.Content == "" {
		return ""
	}
	var data struct {
		RefreshToken string `json:"RefreshToken"`
	}
	if err := json.Unmarshal([]byte(aj.Content), &data); err != nil {
		return ""
	}
	return data.RefreshToken
}

// GetRegion extracts Region from Content for Kiro provider
// Returns "us-east-1" as default if not specified
func (aj *AuthJson) GetRegion() string {
	if aj.Content == "" {
		return "us-east-1"
	}
	var data struct {
		Region string `json:"Region"`
	}
	if err := json.Unmarshal([]byte(aj.Content), &data); err != nil {
		return "us-east-1"
	}
	if data.Region == "" {
		return "us-east-1"
	}
	return data.Region
}

type OAuthProvider struct {
	ID               int                    `gorm:"primaryKey" json:"id"`
	Name             string                 `gorm:"size:255;not null" json:"name"`
	ProviderType     OAuthProviderType      `gorm:"not null" json:"provider_type"`
	AuthJsons        []AuthJson             `gorm:"foreignKey:OAuthProviderID" json:"auth_jsons,omitempty"`
	KeyName          string                 `gorm:"size:255" json:"-"`                          // iFlow key name for refresh
	APIKey           string                 `gorm:"size:255" json:"api_key"`
	APIKeyExpireAt   int64                  `json:"api_key_expire_at"`
	Status           int                    `gorm:"default:1" json:"status"` // 1: Active, 2: Expired, 0: Disabled
	LastRefreshAt    int64                  `json:"last_refresh_at"`
	RefreshFailCount int                    `json:"refresh_fail_count"`
	CreatedAt        int64                  `json:"created_at"`
	UpdatedAt        int64                  `json:"updated_at"`
	BaseURL          string                 `gorm:"size:255" json:"base_url"`
	Channel          *OAuthProviderChannel  `gorm:"-" json:"channel,omitempty"` // Not a DB field, populated on demand
}

// TableName specifies the table name for OAuthProvider
func (OAuthProvider) TableName() string {
	return "o_auth_providers"
}

// OAuthProviderChannel contains channel data for OAuth provider response
type OAuthProviderChannel struct {
	ID          int     `json:"id"`
	Model       string  `json:"model"`
	CustomModel string  `json:"custom_model"`
	MatchRegex  *string `json:"match_regex,omitempty"`
	Enabled     bool    `json:"enabled"`
}

// AuthJsonAddRequest represents a request to add a new AuthJson
type AuthJsonAddRequest struct {
	Enabled bool   `json:"enabled"`
	Content string `json:"content" binding:"required"`
	Remark  string `json:"remark"`
}

// AuthJsonUpdateRequest represents a request to update an AuthJson
type AuthJsonUpdateRequest struct {
	ID      int     `json:"id" binding:"required"`
	Enabled *bool   `json:"enabled,omitempty"`
	Content *string `json:"content,omitempty"`
	Remark  *string `json:"remark,omitempty"`
}

type OAuthProviderUpdateRequest struct {
	ID              int                     `json:"id" binding:"required"`
	Name            *string                 `json:"name,omitempty"`
	ProviderType    *OAuthProviderType      `json:"provider_type,omitempty"`
	Status          *int                    `json:"status,omitempty"`
	BaseURL         *string                 `json:"base_url,omitempty"`
	Model           *string                 `json:"model,omitempty"`
	CustomModel     *string                 `json:"custom_model,omitempty"`
	MatchRegex      *string                 `json:"match_regex,omitempty"`

	AuthJsonsToAdd    []AuthJsonAddRequest    `json:"auth_jsons_to_add,omitempty"`
	AuthJsonsToUpdate []AuthJsonUpdateRequest `json:"auth_jsons_to_update,omitempty"`
	AuthJsonsToDelete []int                   `json:"auth_jsons_to_delete,omitempty"`
}

// GetActiveAuthJson selects the best AuthJson for API access
// Strategy: Prefer StatusCode=200 with most recent LastUseTimeStamp
// If no StatusCode=200, randomly select from enabled AuthJsons
func (p *OAuthProvider) GetActiveAuthJson() *AuthJson {
	if len(p.AuthJsons) == 0 {
		return nil
	}

	var candidates []*AuthJson
	var successCandidates []*AuthJson

	for i := range p.AuthJsons {
		aj := &p.AuthJsons[i]
		if !aj.Enabled || aj.Content == "" {
			continue
		}
		// Skip rate-limited (429) for 5 minutes
		if aj.StatusCode == 429 && aj.LastUseTimeStamp > 0 {
			if time.Now().Unix()-aj.LastUseTimeStamp < 300 {
				continue
			}
		}
		candidates = append(candidates, aj)
		if aj.StatusCode == 200 {
			successCandidates = append(successCandidates, aj)
		}
	}

	if len(successCandidates) > 0 {
		// Select the one with most recent LastUseTimeStamp
		best := successCandidates[0]
		for _, aj := range successCandidates[1:] {
			if aj.LastUseTimeStamp > best.LastUseTimeStamp {
				best = aj
			}
		}
		return best
	}

	if len(candidates) == 0 {
		return nil
	}

	// Random selection from remaining candidates
	return candidates[rand.Intn(len(candidates))]
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
		// Default Kiro base URL, region-specific URLs can be set via BaseURL field
		return "https://q.us-east-1.amazonaws.com"
	default:
		return ""
	}
}
