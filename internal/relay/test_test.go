package relay

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/transformer2/adapter/anthropic"
	"github.com/bestruirui/octopus/internal/transformer2/adapter/openai"
	"github.com/gin-gonic/gin"
)

func TestGetTestChannelKey_OAuthChannel(t *testing.T) {
	t.Run("OAuth channel returns OAuth provider API key", func(t *testing.T) {
		channel := &model.Channel{
			ID:       1,
			Name:     "Test OAuth Channel",
			UseOAuth: true,
			OAuthProvider: &model.OAuthProvider{
				ID:     1,
				Name:   "Test OAuth Provider",
				APIKey: "oauth-api-key-123",
				Status: 1,
			},
			Keys: []model.ChannelKey{}, // Empty keys for OAuth channel
		}

		key, err := getTestChannelKey(channel, 0)
		if err != nil {
			t.Errorf("expected no error for OAuth channel with provider API key, got: %v", err)
		}
		if key != "oauth-api-key-123" {
			t.Errorf("expected OAuth provider API key 'oauth-api-key-123', got %q", key)
		}
	})

	t.Run("OAuth channel with no provider returns error", func(t *testing.T) {
		channel := &model.Channel{
			ID:            2,
			Name:          "Test OAuth Channel No Provider",
			UseOAuth:      true,
			OAuthProvider: nil,
			Keys:          []model.ChannelKey{},
		}

		_, err := getTestChannelKey(channel, 0)
		if err == nil {
			t.Error("expected error for OAuth channel without provider")
		}
	})

	t.Run("OAuth channel with empty provider API key returns error", func(t *testing.T) {
		channel := &model.Channel{
			ID:       3,
			Name:     "Test OAuth Channel Empty API Key",
			UseOAuth: true,
			OAuthProvider: &model.OAuthProvider{
				ID:     2,
				Name:   "Test OAuth Provider",
				APIKey: "", // Empty API key
				Status: 1,
			},
			Keys: []model.ChannelKey{},
		}

		_, err := getTestChannelKey(channel, 0)
		if err == nil {
			t.Error("expected error for OAuth channel with empty provider API key")
		}
	})
}

func TestGetTestChannelKey_RegularChannel(t *testing.T) {
	t.Run("regular channel with keys returns key by index", func(t *testing.T) {
		channel := &model.Channel{
			ID:       4,
			Name:     "Test Regular Channel",
			UseOAuth: false,
			Keys: []model.ChannelKey{
				{ID: 1, ChannelID: 4, Enabled: true, ChannelKey: "key-1"},
				{ID: 2, ChannelID: 4, Enabled: true, ChannelKey: "key-2"},
			},
		}

		key, err := getTestChannelKey(channel, 0)
		if err != nil {
			t.Errorf("expected no error, got: %v", err)
		}
		if key != "key-1" {
			t.Errorf("expected key-1, got %q", key)
		}

		// Test second key
		key2, err := getTestChannelKey(channel, 1)
		if err != nil {
			t.Errorf("expected no error, got: %v", err)
		}
		if key2 != "key-2" {
			t.Errorf("expected key-2, got %q", key2)
		}
	})

	t.Run("regular channel with no keys returns error", func(t *testing.T) {
		channel := &model.Channel{
			ID:       5,
			Name:     "Test Regular Channel No Keys",
			UseOAuth: false,
			Keys:     []model.ChannelKey{},
		}

		_, err := getTestChannelKey(channel, 0)
		if err == nil {
			t.Error("expected error for regular channel with no keys")
		}
	})

	t.Run("regular channel with index out of bounds falls back to first key", func(t *testing.T) {
		channel := &model.Channel{
			ID:       6,
			Name:     "Test Regular Channel",
			UseOAuth: false,
			Keys: []model.ChannelKey{
				{ID: 1, ChannelID: 6, Enabled: true, ChannelKey: "key-1"},
			},
		}

		key, err := getTestChannelKey(channel, 10) // Index out of bounds
		if err != nil {
			t.Errorf("expected no error, got: %v", err)
		}
		if key != "key-1" {
			t.Errorf("expected key-1, got %q", key)
		}
	})

	t.Run("regular channel with negative index falls back to first key", func(t *testing.T) {
		channel := &model.Channel{
			ID:       7,
			Name:     "Test Regular Channel",
			UseOAuth: false,
			Keys: []model.ChannelKey{
				{ID: 1, ChannelID: 7, Enabled: true, ChannelKey: "key-1"},
			},
		}

		key, err := getTestChannelKey(channel, -1) // Negative index
		if err != nil {
			t.Errorf("expected no error, got: %v", err)
		}
		if key != "key-1" {
			t.Errorf("expected key-1, got %q", key)
		}
	})
}

func TestGetTestChannelKey_EdgeCases(t *testing.T) {
	t.Run("nil channel returns error", func(t *testing.T) {
		_, err := getTestChannelKey(nil, 0)
		if err == nil {
			t.Error("expected error for nil channel")
		}
	})

	t.Run("OAuth channel ignores key index and uses OAuth provider", func(t *testing.T) {
		channel := &model.Channel{
			ID:       8,
			Name:     "Test OAuth Channel",
			UseOAuth: true,
			OAuthProvider: &model.OAuthProvider{
				ID:     3,
				Name:   "Test OAuth Provider",
				APIKey: "oauth-key",
				Status: 1,
			},
			Keys: []model.ChannelKey{
				{ID: 1, ChannelID: 8, Enabled: true, ChannelKey: "regular-key"}, // Should be ignored
			},
		}

		// Even with key index 0 and available keys, OAuth channel should use OAuth provider key
		key, err := getTestChannelKey(channel, 0)
		if err != nil {
			t.Errorf("expected no error, got: %v", err)
		}
		if key != "oauth-key" {
			t.Errorf("expected OAuth key 'oauth-key', got %q", key)
		}
	})
}

func TestHandleTestStreamResponseConvertsAnthropicSSEToOpenAIChunks(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/channel/test", nil)

	upstreamBody := strings.Join([]string{
		`event: message_start`,
		`data: {"type":"message_start","message":{"id":"msg_123","type":"message","role":"assistant","model":"claude-3-5-sonnet-20241022","content":[],"usage":{"input_tokens":8,"output_tokens":0}}}`,
		``,
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`,
		``,
		`event: message_delta`,
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":1}}`,
		``,
		`event: message_stop`,
		`data: {"type":"message_stop"}`,
		``,
	}, "\n")

	response := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
	}
	response.Header.Set("Content-Type", "text/event-stream")

	handleTestStreamResponse(c, response, anthropic.NewProviderAdapter(), openai.NewChatClientAdapter())

	body := recorder.Body.String()
	if !strings.Contains(body, `"choices"`) {
		t.Fatalf("expected OpenAI-compatible choices in SSE body, got %s", body)
	}
	if !strings.Contains(body, `"content":"Hello"`) {
		t.Fatalf("expected text delta content in SSE body, got %s", body)
	}
	if strings.Contains(body, `"choices":null`) {
		t.Fatalf("expected metadata-only Anthropic events to be skipped, got %s", body)
	}
	if !strings.Contains(body, "data: [DONE]") {
		t.Fatalf("expected OpenAI-compatible stream terminator, got %s", body)
	}
}
