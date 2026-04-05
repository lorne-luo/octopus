package helper

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/transformer2/adapter"
	"github.com/bestruirui/octopus/internal/transformer2/canonical"
	"github.com/bestruirui/octopus/internal/utils/log"
)

// SpeedTestResult contains the outcome of a speed test.
type SpeedTestResult struct {
	Success        bool
	ResponseTimeMs int
	Error          string
	KeyID          int // Key ID used for this attempt (0 for OAuth keys)
}

// SpeedTestConfig configures a speed test run.
type SpeedTestConfig struct {
	Channel       *model.Channel
	ModelName     string
	ExcludeKeyIDs []int // Key IDs to exclude (already tried)
}

const speedTestAttemptTimeout = 10 * time.Second

func speedTestAttemptContext(parent context.Context) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	return context.WithTimeout(parent, speedTestAttemptTimeout)
}

// RunGroupSpeedTest executes a tool-call speed test for a channel/model pair.
// It measures the time from request start until the provider returns a tool-call response.
// Returns the result including response time in milliseconds.
func RunGroupSpeedTest(ctx context.Context, cfg SpeedTestConfig) SpeedTestResult {
	_ = ctx // silence unused warning in case ctx isn't used directly
	if cfg.Channel == nil {
		return SpeedTestResult{Success: false, Error: "channel is nil"}
	}

	// 1. Get provider adapter
	providerAdapter := adapter.GetProvider(adapter.ProviderType(cfg.Channel.Type))
	if providerAdapter == nil {
		return SpeedTestResult{Success: false, Error: fmt.Sprintf("unsupported channel type: %d", cfg.Channel.Type)}
	}

	// 2. Get API key (with exclusion support for retries)
	apiKey, keyID, err := getSpeedTestChannelKey(cfg.Channel, cfg.ExcludeKeyIDs)
	if err != nil {
		return SpeedTestResult{Success: false, Error: err.Error()}
	}

	// 3. Build canonical tool-call request
	canonicalReq, err := speedTestToolChatTemplate(cfg.ModelName)
	if err != nil {
		return SpeedTestResult{Success: false, Error: "failed to build test request: " + err.Error()}
	}

	// 4. Resolve base URL
	baseUrl := cfg.Channel.GetBaseUrl()
	if baseUrl == "" {
		return SpeedTestResult{Success: false, Error: "channel has no base URL configured"}
	}

	// 5. Build outbound request with per-attempt timeout context
	attemptCtx, cancel := speedTestAttemptContext(ctx)
	defer cancel()

	outboundRequest, err := providerAdapter.BuildRequest(attemptCtx, canonicalReq, baseUrl, apiKey)
	if err != nil {
		return SpeedTestResult{Success: false, Error: "failed to build outbound request: " + err.Error()}
	}

	// 6. Add custom headers
	if len(cfg.Channel.CustomHeader) > 0 {
		for _, header := range cfg.Channel.CustomHeader {
			outboundRequest.Header.Set(header.HeaderKey, header.HeaderValue)
		}
	}

	// 7. Get HTTP client
	httpClient, err := ChannelHttpClient(cfg.Channel)
	if err != nil {
		return SpeedTestResult{Success: false, Error: "failed to create http client: " + err.Error()}
	}

	// 8. Execute request and measure time
	startTime := time.Now()
	response, err := httpClient.Do(outboundRequest)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(attemptCtx.Err(), context.DeadlineExceeded) {
			return SpeedTestResult{Success: false, KeyID: keyID, Error: "request timed out after 10s"}
		}
		return SpeedTestResult{Success: false, KeyID: keyID, Error: "request failed: " + err.Error()}
	}
	defer response.Body.Close()

	// 9. Check response status
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4*1024))
		return SpeedTestResult{
			Success: false,
			KeyID:   keyID,
			Error:   fmt.Sprintf("upstream error %d: %s", response.StatusCode, string(body)),
		}
	}

	// 10. Read response body and measure tool-call response time
	body, err := io.ReadAll(io.LimitReader(response.Body, 64*1024))
	if err != nil {
		return SpeedTestResult{Success: false, Error: "failed to read response: " + err.Error()}
	}

	responseTimeMs := int(time.Since(startTime).Milliseconds())

	// 11. Parse response to verify tool call exists
	if !hasToolCallInResponse(body) {
		return SpeedTestResult{
			Success: false,
			KeyID:   keyID,
			Error:   "response did not contain a tool call",
		}
	}

	log.Debugf("[speed-test] channel=%d model=%s key_id=%d response_time=%dms",
		cfg.Channel.ID, cfg.ModelName, keyID, responseTimeMs)

	return SpeedTestResult{
		Success:       true,
		ResponseTimeMs: responseTimeMs,
		KeyID:         keyID,
	}
}

// speedTestToolChatTemplate returns the canonical tool-chat test request.
func speedTestToolChatTemplate(modelName string) (*canonical.Request, error) {
	temp := float64(0.7)
	topP := float64(1)

	return &canonical.Request{
		Kind:        canonical.KindChat,
		Model:       modelName,
		Temperature: &temp,
		TopP:        &topP,
		Stream:      false, // Non-streaming for simpler timing measurement
		Messages: []canonical.Message{
			{
				Role: "user",
				Content: []canonical.ContentBlock{
					{Type: canonical.ContentText, Text: "What is the weather in San Francisco?"},
				},
			},
		},
		Tools: []canonical.Tool{
			{
				Type:        "function",
				Name:        "get_weather",
				Description: "Get the weather",
				Parameters: json.RawMessage(`{
					"type": "object",
					"properties": {
						"location": {"description": "City name", "type": "string"}
					},
					"required": ["location"],
					"additionalProperties": false
				}`),
			},
		},
	}, nil
}

// getSpeedTestChannelKey retrieves an API key for speed testing.
// For OAuth channels, returns the OAuth provider's API key.
// For regular channels, returns the best available key excluding specified key IDs.
// Returns the key string and the key ID (0 for OAuth keys).
func getSpeedTestChannelKey(channel *model.Channel, excludeKeyIDs []int) (string, int, error) {
	if channel == nil {
		return "", 0, fmt.Errorf("channel is nil")
	}

	// For OAuth channels, use the OAuth provider's API key
	if channel.UseOAuth {
		if channel.OAuthProvider == nil {
			return "", 0, fmt.Errorf("OAuth provider not found for channel %s", channel.Name)
		}
		if channel.OAuthProvider.APIKey == "" {
			return "", 0, fmt.Errorf("OAuth provider has no API key for channel %s", channel.Name)
		}
		return channel.OAuthProvider.APIKey, 0, nil
	}

	// For regular channels, find the best available key
	if len(channel.Keys) == 0 {
		return "", 0, fmt.Errorf("channel has no keys")
	}

	// Build exclusion set
	excludeSet := make(map[int]bool, len(excludeKeyIDs))
	for _, id := range excludeKeyIDs {
		excludeSet[id] = true
	}

	nowSec := time.Now().Unix()
	best := model.ChannelKey{}
	bestToken := int64(0)
	bestSet := false

	for _, k := range channel.Keys {
		if !k.Enabled || k.ChannelKey == "" {
			continue
		}
		// Skip excluded keys
		if excludeSet[k.ID] {
			continue
		}
		// Skip 429-rate-limited keys (5-minute cooldown)
		if k.StatusCode == 429 && k.LastUseTimeStamp > 0 {
			if nowSec-k.LastUseTimeStamp < int64(5*time.Minute/time.Second) {
				continue
			}
		}
		// Select key with lowest token usage
		if !bestSet || k.TotalToken < bestToken {
			best = k
			bestToken = k.TotalToken
			bestSet = true
		}
	}

	if !bestSet {
		return "", 0, fmt.Errorf("no available keys (all excluded or disabled)")
	}

	return best.ChannelKey, best.ID, nil
}

// hasToolCallInResponse checks if the response body contains a tool call.
// Supports both OpenAI and Anthropic response formats.
func hasToolCallInResponse(body []byte) bool {
	// OpenAI format: {"choices":[{"message":{"tool_calls":[...]}}]}
	var openaiResp struct {
		Choices []struct {
			Message struct {
				ToolCalls []interface{} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &openaiResp); err == nil && len(openaiResp.Choices) > 0 {
		if len(openaiResp.Choices[0].Message.ToolCalls) > 0 {
			return true
		}
	}

	// Anthropic format: {"content":[{"type":"tool_use"}]}
	var anthropicResp struct {
		Content []struct {
			Type string `json:"type"`
		} `json:"content"`
	}
	if err := json.Unmarshal(body, &anthropicResp); err == nil {
		for _, c := range anthropicResp.Content {
			if c.Type == "tool_use" {
				return true
			}
		}
	}

	return false
}
