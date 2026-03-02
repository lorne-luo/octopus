package kiro

import (
	"testing"
)

func TestGetDefaultBaseURL(t *testing.T) {
	tests := []struct {
		name     string
		region   string
		expected string
	}{
		{
			name:     "default region",
			region:   "",
			expected: "https://q.us-east-1.amazonaws.com",
		},
		{
			name:     "us-east-1",
			region:   "us-east-1",
			expected: "https://q.us-east-1.amazonaws.com",
		},
		{
			name:     "us-west-2",
			region:   "us-west-2",
			expected: "https://q.us-west-2.amazonaws.com",
		},
		{
			name:     "eu-west-1",
			region:   "eu-west-1",
			expected: "https://q.eu-west-1.amazonaws.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetDefaultBaseURL(tt.region)
			if result != tt.expected {
				t.Errorf("GetDefaultBaseURL(%q) = %q, want %q", tt.region, result, tt.expected)
			}
		})
	}
}

func TestGetRefreshURL(t *testing.T) {
	tests := []struct {
		name     string
		region   string
		expected string
	}{
		{
			name:     "default region",
			region:   "",
			expected: "https://prod.us-east-1.auth.desktop.kiro.dev/refreshToken",
		},
		{
			name:     "us-east-1",
			region:   "us-east-1",
			expected: "https://prod.us-east-1.auth.desktop.kiro.dev/refreshToken",
		},
		{
			name:     "us-west-2",
			region:   "us-west-2",
			expected: "https://prod.us-west-2.auth.desktop.kiro.dev/refreshToken",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetRefreshURL(tt.region)
			if result != tt.expected {
				t.Errorf("GetRefreshURL(%q) = %q, want %q", tt.region, result, tt.expected)
			}
		})
	}
}

func TestParseAuthJsonContent(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		expected    *AuthJsonContent
		expectError bool
	}{
		{
			name:     "valid content",
			content:  `{"RefreshToken":"token123","Region":"us-west-2"}`,
			expected: &AuthJsonContent{RefreshToken: "token123", Region: "us-west-2"},
		},
		{
			name:     "only refresh token",
			content:  `{"RefreshToken":"token123"}`,
			expected: &AuthJsonContent{RefreshToken: "token123"},
		},
		{
			name:     "empty content",
			content:  "",
			expected: nil,
		},
		{
			name:        "invalid json",
			content:     `{invalid}`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseAuthJsonContent(tt.content)
			if tt.expectError {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if tt.expected == nil {
				if result != nil {
					t.Errorf("expected nil, got %+v", result)
				}
				return
			}
			if result.RefreshToken != tt.expected.RefreshToken {
				t.Errorf("RefreshToken = %q, want %q", result.RefreshToken, tt.expected.RefreshToken)
			}
			if result.Region != tt.expected.Region {
				t.Errorf("Region = %q, want %q", result.Region, tt.expected.Region)
			}
		})
	}
}

func TestShouldRefresh(t *testing.T) {
	tests := []struct {
		name     string
		apiKey   string
		expireAt int64
		expected bool
	}{
		{
			name:     "empty api key",
			apiKey:   "",
			expireAt: 0,
			expected: true,
		},
		{
			name:     "valid api key",
			apiKey:   "valid-key",
			expireAt: 9999999999, // far in future
			expected: false,
		},
		{
			name:     "expiring soon",
			apiKey:   "valid-key",
			expireAt: 500, // within 10 minutes
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ShouldRefresh(tt.apiKey, tt.expireAt)
			if result != tt.expected {
				t.Errorf("ShouldRefresh(%q, %d) = %v, want %v", tt.apiKey, tt.expireAt, result, tt.expected)
			}
		})
	}
}

func TestIsKiroError(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		expected bool
	}{
		{
			name:     "unauthorized exception",
			body:     `{"error": "UnauthorizedException"}`,
			expected: true,
		},
		{
			name:     "access denied",
			body:     `{"error": "AccessDenied"}`,
			expected: true,
		},
		{
			name:     "throttling exception",
			body:     `{"error": "ThrottlingException"}`,
			expected: true,
		},
		{
			name:     "normal response",
			body:     `{"content": "hello"}`,
			expected: false,
		},
		{
			name:     "empty response",
			body:     "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsKiroError([]byte(tt.body))
			if result != tt.expected {
				t.Errorf("IsKiroError(%q) = %v, want %v", tt.body, result, tt.expected)
			}
		})
	}
}
