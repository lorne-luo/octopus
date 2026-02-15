package model

type OAuthProvider struct {
	ID               int    `gorm:"primaryKey" json:"id"`
	Name             string `gorm:"size:255;not null" json:"name"`
	ProviderType     string `gorm:"size:50;not null" json:"provider_type"` // e.g., "iflow"
	Cookie           string `gorm:"type:text;not null" json:"-"`           // Never expose in JSON
	KeyName          string `gorm:"size:255" json:"-"`                     // IFlow key name for refresh
	APIKey           string `gorm:"size:255" json:"api_key"`
	APIKeyExpireAt   int64  `json:"api_key_expire_at"`
	Status           int    `gorm:"default:1" json:"status"` // 1: Active, 2: Expired, 0: Disabled
	LastRefreshAt    int64  `json:"last_refresh_at"`
	RefreshFailCount int    `json:"refresh_fail_count"`
	CreatedAt        int64  `json:"created_at"`
	UpdatedAt        int64  `json:"updated_at"`
	Model            string `gorm:"type:text" json:"model"`           // Auto-fetched models, comma-separated
	CustomModel      string `gorm:"type:text" json:"custom_model"`    // Manually added models, comma-separated
	BaseURL          string `gorm:"size:255" json:"base_url"`
}

type OAuthProviderUpdateRequest struct {
	ID           int     `json:"id" binding:"required"`
	Name         *string `json:"name,omitempty"`
	ProviderType *string `json:"provider_type,omitempty"`
	Cookie       *string `json:"cookie,omitempty"`
	Status       *int    `json:"status,omitempty"`
	Model        *string `json:"model,omitempty"`
	CustomModel  *string `json:"custom_model,omitempty"`
	BaseURL      *string `json:"base_url,omitempty"`
}
