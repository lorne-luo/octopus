package model

import (
	"testing"
	"time"

	"github.com/bestruirui/octopus/internal/transformer/outbound"
)

func TestChannel_GetChannelKey(t *testing.T) {
	tests := []struct {
		name    string
		channel *Channel
		wantKey string
		wantOk  bool
	}{
		{
			name:    "nil channel",
			channel: nil,
			wantKey: "",
			wantOk:  false,
		},
		{
			name: "empty keys",
			channel: &Channel{
				Keys: []ChannelKey{},
			},
			wantKey: "",
			wantOk:  false,
		},
		{
			name: "single enabled key",
			channel: &Channel{
				Keys: []ChannelKey{
					{Enabled: true, ChannelKey: "sk-test1", TotalToken: 0},
				},
			},
			wantKey: "sk-test1",
			wantOk:  true,
		},
		{
			name: "multiple keys select lowest token",
			channel: &Channel{
				Keys: []ChannelKey{
					{Enabled: true, ChannelKey: "sk-test1", TotalToken: 100},
					{Enabled: true, ChannelKey: "sk-test2", TotalToken: 50},
					{Enabled: true, ChannelKey: "sk-test3", TotalToken: 200},
				},
			},
			wantKey: "sk-test2",
			wantOk:  true,
		},
		{
			name: "all keys disabled",
			channel: &Channel{
				Keys: []ChannelKey{
					{Enabled: false, ChannelKey: "sk-test1"},
					{Enabled: false, ChannelKey: "sk-test2"},
				},
			},
			wantKey: "",
			wantOk:  false,
		},
		{
			name: "rate limited key skipped",
			channel: &Channel{
				Keys: []ChannelKey{
					{Enabled: true, ChannelKey: "sk-test1", StatusCode: 429, LastUseTimeStamp: time.Now().Unix()},
					{Enabled: true, ChannelKey: "sk-test2", TotalToken: 0},
				},
			},
			wantKey: "sk-test2",
			wantOk:  true,
		},
		{
			name: "rate limited key available after cooldown",
			channel: &Channel{
				Keys: []ChannelKey{
					{Enabled: true, ChannelKey: "sk-test1", StatusCode: 429, LastUseTimeStamp: time.Now().Unix() - int64(6*time.Minute/1e9)}, // 6 minutes ago
					{Enabled: true, ChannelKey: "sk-test2", TotalToken: 100},
				},
			},
			wantKey: "sk-test1", // sk-test1 has 0 TotalToken, sk-test2 has 100
			wantOk:  true,
		},
		{
			name: "empty channel key skipped",
			channel: &Channel{
				Keys: []ChannelKey{
					{Enabled: true, ChannelKey: ""},
					{Enabled: true, ChannelKey: "sk-test2", TotalToken: 0},
				},
			},
			wantKey: "sk-test2",
			wantOk:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.channel.GetChannelKey()
			if (got.ChannelKey != "") != tt.wantOk {
				t.Errorf("GetChannelKey() ok = %v, want %v", got.ChannelKey != "", tt.wantOk)
			}
			if got.ChannelKey != tt.wantKey {
				t.Errorf("GetChannelKey() key = %q, want %q", got.ChannelKey, tt.wantKey)
			}
		})
	}
}

func TestChannel_GetBaseUrl(t *testing.T) {
	tests := []struct {
		name    string
		channel *Channel
		want    string
	}{
		{
			name:    "nil channel",
			channel: nil,
			want:    "",
		},
		{
			name: "empty base urls",
			channel: &Channel{
				BaseUrls: []BaseUrl{},
			},
			want: "",
		},
		{
			name: "single base url",
			channel: &Channel{
				BaseUrls: []BaseUrl{{URL: "https://api.example.com"}},
			},
			want: "https://api.example.com",
		},
		{
			name: "select lowest delay",
			channel: &Channel{
				BaseUrls: []BaseUrl{
					{URL: "https://api1.example.com", Delay: 100},
					{URL: "https://api2.example.com", Delay: 50},
					{URL: "https://api3.example.com", Delay: 200},
				},
			},
			want: "https://api2.example.com",
		},
		{
			name: "skip empty urls",
			channel: &Channel{
				BaseUrls: []BaseUrl{
					{URL: "", Delay: 10},
					{URL: "https://api.example.com", Delay: 100},
				},
			},
			want: "https://api.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.channel.GetBaseUrl(); got != tt.want {
				t.Errorf("GetBaseUrl() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestOAuthProvider_GetBaseURL(t *testing.T) {
	tests := []struct {
		name     string
		provider *OAuthProvider
		want     string
	}{
		{
			name: "custom base url",
			provider: &OAuthProvider{
				ProviderType: "iflow",
				BaseURL:      "https://custom.api.com/v1",
			},
			want: "https://custom.api.com/v1",
		},
		{
			name: "iflow default",
			provider: &OAuthProvider{
				ProviderType: "iflow",
			},
			want: "https://apis.iflow.cn/v1",
		},
		{
			name: "unknown provider type",
			provider: &OAuthProvider{
				ProviderType: "unknown",
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.provider.GetBaseURL(); got != tt.want {
				t.Errorf("GetBaseURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestChannel_UseOAuth(t *testing.T) {
	channel := &Channel{
		Name:     "test-oauth-channel",
		Type:     outbound.OutboundTypeOpenAIChat,
		UseOAuth: true,
		OAuthProviderID: 1,
		OAuthProvider: &OAuthProvider{
			ID:       1,
			Name:     "test-provider",
			ProviderType: "iflow",
			APIKey:   "test-api-key",
			Status:   1,
		},
	}

	if !channel.UseOAuth {
		t.Error("expected UseOAuth to be true")
	}
	if channel.OAuthProviderID != 1 {
		t.Errorf("expected OAuthProviderID to be 1, got %d", channel.OAuthProviderID)
	}
	if channel.OAuthProvider == nil {
		t.Error("expected OAuthProvider to be set")
	}
}
