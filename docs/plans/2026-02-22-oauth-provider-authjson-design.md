# OAuthProvider AuthJSON Refactor Design

**Date:** 2026-02-22
**Status:** Approved

## Overview

Refactor OAuthProvider model to use a flexible `auth_json` field instead of a fixed `cookie` field. This enables supporting multiple OAuth provider types with different authentication requirements.

## Goals

1. Replace `Cookie` field with `AuthJSON` field for storing provider-specific credentials
2. Create `OAuthProviderType` enum for type-safe provider identification
3. Prepare the architecture for future Kiro OAuth provider support

## Changes

### 1. Model Changes (`internal/model/oauth_provider.go`)

**New enum type:**

```go
type OAuthProviderType int

const (
    OAuthProviderTypeIFlow OAuthProviderType = iota + 1
    OAuthProviderTypeKiro  // Future: Kiro OAuth
)

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
```

**Updated OAuthProvider model:**

```go
type OAuthProvider struct {
    ID               int               `gorm:"primaryKey" json:"id"`
    Name             string            `gorm:"size:255;not null" json:"name"`
    ProviderType     OAuthProviderType `gorm:"not null" json:"provider_type"`
    AuthJSON         string            `gorm:"type:text" json:"-"`              // Replaces Cookie
    KeyName          string            `gorm:"size:255" json:"-"`               // iFlow specific
    APIKey           string            `gorm:"size:255" json:"api_key"`
    APIKeyExpireAt   int64             `json:"api_key_expire_at"`
    Status           int               `gorm:"default:1" json:"status"`
    LastRefreshAt    int64             `json:"last_refresh_at"`
    RefreshFailCount int               `json:"refresh_fail_count"`
    CreatedAt        int64             `json:"created_at"`
    UpdatedAt        int64             `json:"updated_at"`
    BaseURL          string            `gorm:"size:255" json:"base_url"`
    Channel          *OAuthProviderChannel `gorm:"-" json:"channel,omitempty"`
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
```

### 2. Request Models

```go
type OAuthProviderCreateRequest struct {
    Name         *string           `json:"name,omitempty"`
    ProviderType *OAuthProviderType `json:"provider_type,omitempty"`
    AuthJSON     *string           `json:"auth_json,omitempty"`
    BaseURL      *string           `json:"base_url,omitempty"`
    Model        *string           `json:"model,omitempty"`
    CustomModel  *string           `json:"custom_model,omitempty"`
    MatchRegex   *string           `json:"match_regex,omitempty"`
}

type OAuthProviderUpdateRequest struct {
    ID           int               `json:"id" binding:"required"`
    Name         *string           `json:"name,omitempty"`
    ProviderType *OAuthProviderType `json:"provider_type,omitempty"`
    AuthJSON     *string           `json:"auth_json,omitempty"`
    Status       *int              `json:"status,omitempty"`
    BaseURL      *string           `json:"base_url,omitempty"`
    Model        *string           `json:"model,omitempty"`
    CustomModel  *string           `json:"custom_model,omitempty"`
    MatchRegex   *string           `json:"match_regex,omitempty"`
}
```

### 3. Database Migration

**One-shot migration approach:**

1. Add new `auth_json` column
2. Migrate existing `cookie` data to `{"BXAuth": "cookie_value"}`
3. Update `provider_type` from string to int
4. Drop old `cookie` column

**AuthJSON format by provider:**

| Provider Type | AuthJSON Format |
|---------------|-----------------|
| iFlow | `{"BXAuth": "cookie_value"}` |
| kiro (future) | `{"refreshToken": "...", "region": "us-east-1", ...}` |

### 4. Files to Update

| File | Changes |
|------|---------|
| `internal/model/oauth_provider.go` | Add enum, update model, add helper methods |
| `internal/db/migrate/XXX.go` | Migration script |
| `internal/oauth/manager.go` | Use `GetBXAuth()` instead of `Cookie` |
| `internal/oauth/iflow/auth.go` | Rename `cookie` param to `bxAuth` |
| `internal/op/oauth_provider.go` | Update CRUD for `auth_json` |
| `internal/server/handlers/oauth_provider.go` | Handle `auth_json` input |
| Frontend components | Replace cookie input with Auth JSON textarea |

### 5. Handler Validation

```go
// Validate AuthJSON format
if req.AuthJSON != nil && *req.AuthJSON != "" {
    if !json.Valid([]byte(*req.AuthJSON)) {
        resp.Error(c, http.StatusBadRequest, "auth_json must be valid JSON")
        return
    }

    // Provider-specific validation
    if providerType == OAuthProviderTypeIFlow {
        var data map[string]interface{}
        json.Unmarshal([]byte(*req.AuthJSON), &data)
        if _, ok := data["BXAuth"]; !ok {
            resp.Error(c, http.StatusBadRequest, "auth_json must contain BXAuth field for iFlow provider")
            return
        }
    }
}
```

### 6. Frontend Changes

- Replace `cookie` text input with `auth_json` textarea
- Provider Type dropdown: `iflow`, `kiro` (future)
- Placeholder examples:
  - iFlow: `{"BXAuth": "your_cookie_value"}`
  - kiro: `{"refreshToken": "...", "region": "us-east-1"}`

## Testing

1. Unit tests for `GetBXAuth()` helper
2. Unit tests for OAuthProviderType enum
3. Integration tests for create/update with `auth_json`
4. Migration tests for cookie -> auth_json conversion
