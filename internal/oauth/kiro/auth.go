package kiro

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/bestruirui/octopus/internal/utils/log"
	"github.com/google/uuid"
)

// RefreshToken calls the Kiro Desktop Auth endpoint to refresh the access token
// endpoint: https://prod.{region}.auth.desktop.kiro.dev/refreshToken
func RefreshToken(ctx context.Context, refreshToken, region string) (*TokenRefreshResponse, error) {
	if strings.TrimSpace(refreshToken) == "" {
		return nil, fmt.Errorf("kiro: refresh token is empty")
	}

	if region == "" {
		region = "us-east-1"
	}

	refreshURL := GetRefreshURL(region)

	payload := map[string]string{"refreshToken": refreshToken}
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("kiro: marshal request failed: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, refreshURL, strings.NewReader(string(jsonData)))
	if err != nil {
		return nil, fmt.Errorf("kiro: create request failed: %w", err)
	}

	// Set headers matching Kiro Desktop client
	fingerprint := generateFingerprint()
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", fmt.Sprintf("KiroIDE-0.7.45-%s", fingerprint))

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("kiro: request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("kiro: read response failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("kiro: refresh failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result TokenRefreshResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("kiro: unmarshal response failed: %w", err)
	}

	if result.AccessToken == "" {
		return nil, fmt.Errorf("kiro: response missing access token")
	}

	log.Infof("kiro: token refreshed successfully, expires_in=%d", result.ExpiresIn)

	return &result, nil
}

// generateFingerprint generates a random fingerprint for the Kiro client
func generateFingerprint() string {
	return uuid.New().String()[:8]
}

// ShouldRefresh checks if the token should be refreshed
// Returns true if the token is within 10 minutes of expiration or is empty
func ShouldRefresh(apiKey string, expireAt int64) bool {
	if apiKey == "" {
		return true
	}
	// Refresh if within 10 minutes (600 seconds) of expiration
	if time.Now().Unix() > expireAt-600 {
		return true
	}
	return false
}

// IsKiroError checks if the response body indicates a Kiro-specific error
func IsKiroError(body []byte) bool {
	bodyStr := strings.ToLower(string(body))
	return strings.Contains(bodyStr, "unauthorizedexception") ||
		strings.Contains(bodyStr, "accessdenied") ||
		strings.Contains(bodyStr, "throttlingexception")
}
