package anthropic

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/bestruirui/octopus/internal/transformer2/canonical"
)

func TestAnthropicSimpleText(t *testing.T) {
	adapter := NewClientAdapter()
	ctx := context.Background()

	// Test simple text request
	reqBody := []byte(`{
		"model": "claude-sonnet-4-20250514",
		"max_tokens": 1024,
		"messages": [
			{"role": "user", "content": "Hello Claude"}
		]
	}`)

	req, err := adapter.ParseRequest(ctx, reqBody, http.Header{})
	if err != nil {
		t.Fatalf("ParseRequest failed: %v", err)
	}

	// Verify basic fields
	if req.Model != "claude-sonnet-4-20250514" {
		t.Errorf("Expected model 'claude-sonnet-4-20250514', got '%s'", req.Model)
	}
	if req.Kind != canonical.KindChat {
		t.Errorf("Expected KindChat, got %d", req.Kind)
	}
	if req.SourceFormat != canonical.FormatAnthropic {
		t.Errorf("Expected FormatAnthropic, got '%s'", req.SourceFormat)
	}
	if req.MaxTokens == nil || *req.MaxTokens != 1024 {
		t.Errorf("Expected max_tokens 1024, got %v", req.MaxTokens)
	}

	// Verify messages
	if len(req.Messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(req.Messages))
	}
	if req.Messages[0].Role != canonical.RoleUser {
		t.Errorf("Expected role 'user', got '%s'", req.Messages[0].Role)
	}
	if len(req.Messages[0].Content) != 1 {
		t.Fatalf("Expected 1 content block, got %d", len(req.Messages[0].Content))
	}
	if req.Messages[0].Content[0].Text != "Hello Claude" {
		t.Errorf("Expected content 'Hello Claude', got '%s'", req.Messages[0].Content[0].Text)
	}
}

func TestAnthropicSystemPrompt(t *testing.T) {
	adapter := NewClientAdapter()
	ctx := context.Background()

	t.Run("string_form", func(t *testing.T) {
		reqBody := []byte(`{
			"model": "claude-sonnet-4-20250514",
			"max_tokens": 1024,
			"system": "You are a pirate.",
			"messages": [
				{"role": "user", "content": "Ahoy!"}
			]
		}`)

		req, err := adapter.ParseRequest(ctx, reqBody, http.Header{})
		if err != nil {
			t.Fatalf("ParseRequest failed: %v", err)
		}

		// Should have 2 messages: system + user
		if len(req.Messages) != 2 {
			t.Fatalf("Expected 2 messages, got %d", len(req.Messages))
		}

		// First message should be system
		if req.Messages[0].Role != canonical.RoleSystem {
			t.Errorf("Expected first message role 'system', got '%s'", req.Messages[0].Role)
		}
		if len(req.Messages[0].Content) != 1 || req.Messages[0].Content[0].Text != "You are a pirate." {
			t.Errorf("Expected system content 'You are a pirate.', got %v", req.Messages[0].Content)
		}

		// Second message should be user
		if req.Messages[1].Role != canonical.RoleUser {
			t.Errorf("Expected second message role 'user', got '%s'", req.Messages[1].Role)
		}
	})

	t.Run("array_form", func(t *testing.T) {
		reqBody := []byte(`{
			"model": "claude-sonnet-4-20250514",
			"max_tokens": 1024,
			"system": [
				{"type": "text", "text": "You are a helpful assistant.", "cache_control": {"type": "ephemeral"}},
				{"type": "text", "text": "Always respond in JSON."}
			],
			"messages": [
				{"role": "user", "content": "Hello"}
			]
		}`)

		req, err := adapter.ParseRequest(ctx, reqBody, http.Header{})
		if err != nil {
			t.Fatalf("ParseRequest failed: %v", err)
		}

		// Should have 2 messages: system + user
		if len(req.Messages) != 2 {
			t.Fatalf("Expected 2 messages, got %d", len(req.Messages))
		}

		// First message should be system with array content
		if req.Messages[0].Role != canonical.RoleSystem {
			t.Errorf("Expected first message role 'system', got '%s'", req.Messages[0].Role)
		}
		if len(req.Messages[0].Content) != 2 {
			t.Errorf("Expected 2 system blocks, got %d", len(req.Messages[0].Content))
		}

		// Verify array format hint
		if !req.Hints.AnthropicSystemArrayFormat {
			t.Error("Expected AnthropicSystemArrayFormat hint to be true")
		}
	})
}

func TestAnthrophicToolCalling(t *testing.T) {
	adapter := NewClientAdapter()
	ctx := context.Background()

	t.Run("tool_definition", func(t *testing.T) {
		reqBody := []byte(`{
			"model": "claude-sonnet-4-20250514",
			"max_tokens": 1024,
			"tools": [
				{
					"name": "get_weather",
					"description": "Get weather for a location",
					"input_schema": {"type": "object", "properties": {"location": {"type": "string"}}}
				}
			],
			"tool_choice": {"type": "auto"},
			"messages": [
				{"role": "user", "content": "What's the weather in Tokyo?"}
			]
		}`)

		req, err := adapter.ParseRequest(ctx, reqBody, http.Header{})
		if err != nil {
			t.Fatalf("ParseRequest failed: %v", err)
		}

		// Verify tools
		if len(req.Tools) != 1 {
			t.Fatalf("Expected 1 tool, got %d", len(req.Tools))
		}

		tool := req.Tools[0]
		if tool.Name != "get_weather" {
			t.Errorf("Expected tool name 'get_weather', got '%s'", tool.Name)
		}
		if tool.Description != "Get weather for a location" {
			t.Errorf("Expected tool description, got '%s'", tool.Description)
		}
		if tool.Type != "function" {
			t.Errorf("Expected tool type 'function', got '%s'", tool.Type)
		}

		// Verify tool_choice
		if req.ToolChoice == nil {
			t.Fatal("Expected tool_choice, got nil")
		}
		if req.ToolChoice.Mode != "auto" {
			t.Errorf("Expected tool_choice mode 'auto', got '%s'", req.ToolChoice.Mode)
		}
	})

	t.Run("tool_result_round_trip", func(t *testing.T) {
		reqBody := []byte(`{
			"model": "claude-sonnet-4-20250514",
			"max_tokens": 1024,
			"messages": [
				{"role": "user", "content": "What's the weather?"},
				{"role": "assistant", "content": [{"type": "tool_use", "id": "toolu_123", "name": "get_weather", "input": {"location": "Tokyo"}}]},
				{"role": "user", "content": [{"type": "tool_result", "tool_use_id": "toolu_123", "content": "Sunny, 25C"}]}
			]
		}`)

		req, err := adapter.ParseRequest(ctx, reqBody, http.Header{})
		if err != nil {
			t.Fatalf("ParseRequest failed: %v", err)
		}

		// Verify 3 messages
		if len(req.Messages) != 3 {
			t.Fatalf("Expected 3 messages, got %d", len(req.Messages))
		}

		// First message: user
		if req.Messages[0].Role != canonical.RoleUser {
			t.Errorf("Expected first message role 'user', got '%s'", req.Messages[0].Role)
		}

		// Second message: assistant with tool_calls
		if req.Messages[1].Role != canonical.RoleAssistant {
			t.Errorf("Expected second message role 'assistant', got '%s'", req.Messages[1].Role)
		}
		if len(req.Messages[1].ToolCalls) != 1 {
			t.Errorf("Expected 1 tool_call, got %d", len(req.Messages[1].ToolCalls))
		}
		if req.Messages[1].ToolCalls[0].ID != "toolu_123" {
			t.Errorf("Expected tool_call id 'toolu_123', got '%s'", req.Messages[1].ToolCalls[0].ID)
		}
		if req.Messages[1].ToolCalls[0].Name != "get_weather" {
			t.Errorf("Expected tool_call name 'get_weather', got '%s'", req.Messages[1].ToolCalls[0].Name)
		}

		// Third message: tool result
		if req.Messages[2].Role != canonical.RoleTool {
			t.Errorf("Expected third message role 'tool', got '%s'", req.Messages[2].Role)
		}
		if req.Messages[2].ToolCallID == nil || *req.Messages[2].ToolCallID != "toolu_123" {
			t.Errorf("Expected tool_call_id 'toolu_123', got %v", req.Messages[2].ToolCallID)
		}
	})
}

func TestAnthropicThinkingConfig(t *testing.T) {
	adapter := NewClientAdapter()
	ctx := context.Background()

	t.Run("adaptive_mode", func(t *testing.T) {
		reqBody := []byte(`{
			"model": "claude-sonnet-4-20250514",
			"max_tokens": 1024,
			"thinking": {"type": "adaptive"},
			"messages": [
				{"role": "user", "content": "Hello"}
			]
		}`)

		req, err := adapter.ParseRequest(ctx, reqBody, http.Header{})
		if err != nil {
			t.Fatalf("ParseRequest failed: %v", err)
		}

		if req.Reasoning == nil {
			t.Fatal("Expected Reasoning config, got nil")
		}
		// Adaptive mode: Enabled = nil (not set)
		if req.Reasoning.Enabled != nil {
			t.Errorf("Expected Enabled=nil for adaptive, got %v", req.Reasoning.Enabled)
		}
	})

	t.Run("enabled_mode", func(t *testing.T) {
		reqBody := []byte(`{
			"model": "claude-sonnet-4-20250514",
			"max_tokens": 1024,
			"thinking": {"type": "enabled", "budget_tokens": 10000},
			"messages": [
				{"role": "user", "content": "Hello"}
			]
		}`)

		req, err := adapter.ParseRequest(ctx, reqBody, http.Header{})
		if err != nil {
			t.Fatalf("ParseRequest failed: %v", err)
		}

		if req.Reasoning == nil {
			t.Fatal("Expected Reasoning config, got nil")
		}
		if req.Reasoning.Enabled == nil || !*req.Reasoning.Enabled {
			t.Errorf("Expected Enabled=true, got %v", req.Reasoning.Enabled)
		}
		if req.Reasoning.BudgetTokens == nil || *req.Reasoning.BudgetTokens != 10000 {
			t.Errorf("Expected BudgetTokens=10000, got %v", req.Reasoning.BudgetTokens)
		}
	})

	t.Run("disabled_mode", func(t *testing.T) {
		reqBody := []byte(`{
			"model": "claude-sonnet-4-20250514",
			"max_tokens": 1024,
			"thinking": {"type": "disabled"},
			"messages": [
				{"role": "user", "content": "Hello"}
			]
		}`)

		req, err := adapter.ParseRequest(ctx, reqBody, http.Header{})
		if err != nil {
			t.Fatalf("ParseRequest failed: %v", err)
		}

		if req.Reasoning == nil {
			t.Fatal("Expected Reasoning config, got nil")
		}
		if req.Reasoning.Enabled == nil || *req.Reasoning.Enabled {
			t.Errorf("Expected Enabled=false, got %v", req.Reasoning.Enabled)
		}
	})
}

func TestAnthropicFinishReasonMapping(t *testing.T) {
	// Test Anthropic -> Canonical mappings
	anthropicToCanonical := []struct {
		anthropic string
		canonical string
	}{
		{"end_turn", "stop"},
		{"max_tokens", "length"},
		{"tool_use", "tool_calls"},
		{"stop_sequence", "stop"},
		{"pause_turn", "stop"},
		{"refusal", "content_filter"},
	}

	for _, tt := range anthropicToCanonical {
		t.Run(tt.anthropic+"_to_canonical", func(t *testing.T) {
			result := mapAnthropicStopReasonToCanonical(tt.anthropic)
			if result == nil {
				t.Fatalf("Expected non-nil result for '%s'", tt.anthropic)
			}
			if *result != tt.canonical {
				t.Errorf("Expected '%s' -> '%s', got '%s'", tt.anthropic, tt.canonical, *result)
			}
		})
	}

	// Test Canonical -> Anthropic mappings (note: these are the inverse mappings)
	canonicalToAnthropic := []struct {
		canonical string
		anthropic string
	}{
		{"stop", "end_turn"},
		{"length", "max_tokens"},
		{"tool_calls", "tool_use"},
		{"content_filter", "refusal"},
	}

	for _, tt := range canonicalToAnthropic {
		t.Run(tt.canonical+"_to_anthropic", func(t *testing.T) {
			result := mapCanonicalFinishReasonToAnthropic(tt.canonical)
			if result != tt.anthropic {
				t.Errorf("Expected '%s' -> '%s', got '%s'", tt.canonical, tt.anthropic, result)
			}
		})
	}
}

func TestAnthropicStreamAggregation(t *testing.T) {
	adapter := NewClientAdapter()
	ctx := context.Background()

	// Create chunks simulating Anthropic streaming
	chunks := []*canonical.Chunk{
		// message_start - contains input_tokens
		{
			ID:    "msg_123",
			Model: "claude-sonnet-4-20250514",
			Usage: &canonical.Usage{PromptTokens: 100},
		},
		// content_block_start (text)
		{
			ID: "msg_123",
		},
		// content_block_delta (text)
		{
			ID: "msg_123",
			Deltas: []canonical.ChoiceDelta{
				{
					Index: 0,
					Delta: canonical.Message{
						Content: []canonical.ContentBlock{{Type: canonical.ContentText, Text: "Hello"}},
					},
				},
			},
		},
		// content_block_delta (more text)
		{
			ID: "msg_123",
			Deltas: []canonical.ChoiceDelta{
				{
					Index: 0,
					Delta: canonical.Message{
						Content: []canonical.ContentBlock{{Type: canonical.ContentText, Text: " there!"}},
					},
				},
			},
		},
		// message_delta - contains output_tokens and stop_reason
		{
			ID: "msg_123",
			Deltas: []canonical.ChoiceDelta{
				{
					Index:        0,
					FinishReason: strPtr("stop"),
				},
			},
			Usage: &canonical.Usage{CompletionTokens: 10},
		},
	}

	for _, chunk := range chunks {
		adapter.aggregator.addChunk(chunk)
	}

	// Aggregate
	resp, err := adapter.AggregateStream(ctx)
	if err != nil {
		t.Fatalf("AggregateStream failed: %v", err)
	}

	// Verify aggregated response
	if resp.ID != "msg_123" {
		t.Errorf("Expected ID 'msg_123', got '%s'", resp.ID)
	}
	if resp.Model != "claude-sonnet-4-20250514" {
		t.Errorf("Expected model 'claude-sonnet-4-20250514', got '%s'", resp.Model)
	}

	// Verify content aggregation
	if len(resp.Choices) != 1 {
		t.Fatalf("Expected 1 choice, got %d", len(resp.Choices))
	}
	if len(resp.Choices[0].Message.Content) != 1 {
		t.Fatalf("Expected 1 content block, got %d", len(resp.Choices[0].Message.Content))
	}
	aggregatedText := resp.Choices[0].Message.Content[0].Text
	if aggregatedText != "Hello there!" {
		t.Errorf("Expected content 'Hello there!', got '%s'", aggregatedText)
	}

	// Verify finish reason
	if resp.Choices[0].FinishReason == nil || *resp.Choices[0].FinishReason != "stop" {
		t.Errorf("Expected finish_reason 'stop', got %v", resp.Choices[0].FinishReason)
	}

	// Verify usage aggregation
	if resp.Usage == nil {
		t.Fatal("Expected usage, got nil")
	}
	// Input tokens from message_start
	if resp.Usage.PromptTokens != 100 {
		t.Errorf("Expected prompt_tokens 100, got %d", resp.Usage.PromptTokens)
	}
	// Output tokens from message_delta
	if resp.Usage.CompletionTokens != 10 {
		t.Errorf("Expected completion_tokens 10, got %d", resp.Usage.CompletionTokens)
	}
}

func TestFormatResponse(t *testing.T) {
	adapter := NewClientAdapter()
	ctx := context.Background()

	t.Run("simple_text_response", func(t *testing.T) {
		finishReason := "stop"
		resp := &canonical.Response{
			ID:     "msg_123",
			Model:  "claude-sonnet-4-20250514",
			Object: "chat.completion",
			Choices: []canonical.Choice{
				{
					Index: 0,
					Message: canonical.Message{
						Role: canonical.RoleAssistant,
						Content: []canonical.ContentBlock{
							{Type: canonical.ContentText, Text: "Hello! How can I help?"},
						},
					},
					FinishReason: &finishReason,
				},
			},
			Usage: &canonical.Usage{
				PromptTokens:     10,
				CompletionTokens: 8,
				TotalTokens:      18,
			},
		}

		output, err := adapter.FormatResponse(ctx, resp)
		if err != nil {
			t.Fatalf("FormatResponse failed: %v", err)
		}

		var aresp MessageResponse
		if err := json.Unmarshal(output, &aresp); err != nil {
			t.Fatalf("Failed to unmarshal response: %v", err)
		}

		// Verify response structure
		if aresp.ID != "msg_123" {
			t.Errorf("Expected ID 'msg_123', got '%s'", aresp.ID)
		}
		if aresp.Type != "message" {
			t.Errorf("Expected type 'message', got '%s'", aresp.Type)
		}
		if aresp.Role != "assistant" {
			t.Errorf("Expected role 'assistant', got '%s'", aresp.Role)
		}
		if aresp.StopReason != "end_turn" {
			t.Errorf("Expected stop_reason 'end_turn', got '%s'", aresp.StopReason)
		}
		if len(aresp.Content) != 1 {
			t.Errorf("Expected 1 content block, got %d", len(aresp.Content))
		}
		if aresp.Content[0].Type != ContentTypeText {
			t.Errorf("Expected content type 'text', got '%s'", aresp.Content[0].Type)
		}
		if aresp.Content[0].Text != "Hello! How can I help?" {
			t.Errorf("Expected content text 'Hello! How can I help?', got '%s'", aresp.Content[0].Text)
		}
		if aresp.Usage.InputTokens != 10 {
			t.Errorf("Expected input_tokens 10, got %d", aresp.Usage.InputTokens)
		}
		if aresp.Usage.OutputTokens != 8 {
			t.Errorf("Expected output_tokens 8, got %d", aresp.Usage.OutputTokens)
		}
	})

	t.Run("tool_calls_response", func(t *testing.T) {
		finishReason := "tool_calls"
		resp := &canonical.Response{
			ID:     "msg_tool",
			Model:  "claude-sonnet-4-20250514",
			Object: "chat.completion",
			Choices: []canonical.Choice{
				{
					Index: 0,
					Message: canonical.Message{
						Role: canonical.RoleAssistant,
						ToolCalls: []canonical.ToolCall{
							{
								ID:        "toolu_123",
								Type:      "function",
								Name:      "get_weather",
								Arguments: `{"location": "Tokyo"}`,
							},
						},
					},
					FinishReason: &finishReason,
				},
			},
		}

		output, err := adapter.FormatResponse(ctx, resp)
		if err != nil {
			t.Fatalf("FormatResponse failed: %v", err)
		}

		var aresp MessageResponse
		if err := json.Unmarshal(output, &aresp); err != nil {
			t.Fatalf("Failed to unmarshal response: %v", err)
		}

		// Verify tool_use content block
		if len(aresp.Content) != 1 {
			t.Fatalf("Expected 1 content block, got %d", len(aresp.Content))
		}
		if aresp.Content[0].Type != ContentTypeToolUse {
			t.Errorf("Expected content type 'tool_use', got '%s'", aresp.Content[0].Type)
		}
		if aresp.Content[0].ID != "toolu_123" {
			t.Errorf("Expected tool id 'toolu_123', got '%s'", aresp.Content[0].ID)
		}
		if aresp.Content[0].Name != "get_weather" {
			t.Errorf("Expected tool name 'get_weather', got '%s'", aresp.Content[0].Name)
		}
		if aresp.StopReason != "tool_use" {
			t.Errorf("Expected stop_reason 'tool_use', got '%s'", aresp.StopReason)
		}
	})
}

func TestFormatError(t *testing.T) {
	adapter := NewClientAdapter()
	ctx := context.Background()

	canonErr := &canonical.Error{
		Code:       "invalid_api_key",
		Message:    "Invalid API key provided",
		Type:       "authentication_error",
		StatusCode: 401,
	}

	output, err := adapter.FormatError(ctx, canonErr)
	if err != nil {
		t.Fatalf("FormatError failed: %v", err)
	}

	var aerr ErrorResponse
	if json.Unmarshal(output, &aerr) != nil {
		t.Fatalf("Failed to unmarshal error: %v", err)
	}

	if aerr.Type != "error" {
		t.Errorf("Expected type 'error', got '%s'", aerr.Type)
	}
	if aerr.Error.Type != "authentication_error" {
		t.Errorf("Expected error type 'authentication_error', got '%s'", aerr.Error.Type)
	}
	if aerr.Error.Message != "Invalid API key provided" {
		t.Errorf("Expected error message 'Invalid API key provided', got '%s'", aerr.Error.Message)
	}
}

func TestParseRequestWithHeaders(t *testing.T) {
	adapter := NewClientAdapter()
	ctx := context.Background()

	reqBody := []byte(`{
		"model": "claude-sonnet-4-20250514",
		"max_tokens": 1024,
		"messages": [{"role": "user", "content": "Hello"}]
	}`)

	header := http.Header{}
	header.Set("anthropic-beta", "prompt-caching-2024-07-31")
	header.Set("anthropic-version", "2023-06-01")

	req, err := adapter.ParseRequest(ctx, reqBody, header)
	if err != nil {
		t.Fatalf("ParseRequest failed: %v", err)
	}

	// Verify headers were extracted
	if req.Headers == nil {
		t.Fatal("Expected Headers to be set")
	}
	if req.Headers.Get("Anthropic-Beta") != "prompt-caching-2024-07-31" {
		t.Errorf("Expected Anthropic-Beta header, got '%s'", req.Headers.Get("Anthropic-Beta"))
	}
	if req.Headers.Get("Anthropic-Version") != "2023-06-01" {
		t.Errorf("Expected Anthropic-Version header, got '%s'", req.Headers.Get("Anthropic-Version"))
	}
}

// Helper function
func strPtr(s string) *string {
	return &s
}

func TestAnthropicCachingUsageSemantics(t *testing.T) {
	adapter := NewClientAdapter()
	ctx := context.Background()

	// Test that cache tokens are correctly calculated
	// When caching is enabled, input_tokens only represents tokens AFTER the last cache breakpoint
	// Correct formula: PromptTokens = cache_read_input_tokens + cache_creation_input_tokens + input_tokens
	resp := &canonical.Response{
		ID:     "msg_cached",
		Model:  "claude-sonnet-4-20250514",
		Object: "chat.completion",
		Choices: []canonical.Choice{
			{
				Index: 0,
				Message: canonical.Message{
					Role: canonical.RoleAssistant,
					Content: []canonical.ContentBlock{
						{Type: canonical.ContentText, Text: "Response"},
					},
				},
				FinishReason: strPtr("stop"),
			},
		},
		Usage: &canonical.Usage{
			PromptTokens:              500,  // This should be the sum
			CompletionTokens:          50,
			CacheCreationInputTokens:  200,
			CacheReadInputTokens:      100,
			InputTokensAfterBreakpoint: 200, // Original input_tokens from API
		},
	}

	output, err := adapter.FormatResponse(ctx, resp)
	if err != nil {
		t.Fatalf("FormatResponse failed: %v", err)
	}

	var aresp MessageResponse
	if err := json.Unmarshal(output, &aresp); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	// Verify usage
	if aresp.Usage.InputTokens != 500 {
		t.Errorf("Expected input_tokens 500 (total), got %d", aresp.Usage.InputTokens)
	}
	if aresp.Usage.CacheCreationInputTokens != 200 {
		t.Errorf("Expected cache_creation_input_tokens 200, got %d", aresp.Usage.CacheCreationInputTokens)
	}
	if aresp.Usage.CacheReadInputTokens != 100 {
		t.Errorf("Expected cache_read_input_tokens 100, got %d", aresp.Usage.CacheReadInputTokens)
	}
}

func TestAnthropicImageContent(t *testing.T) {
	adapter := NewClientAdapter()
	ctx := context.Background()

	t.Run("image_url", func(t *testing.T) {
		reqBody := []byte(`{
			"model": "claude-sonnet-4-20250514",
			"max_tokens": 1024,
			"messages": [
				{
					"role": "user",
					"content": [
						{"type": "image", "source": {"type": "url", "url": "https://example.com/image.png"}}
					]
				}
			]
		}`)

		req, err := adapter.ParseRequest(ctx, reqBody, http.Header{})
		if err != nil {
			t.Fatalf("ParseRequest failed: %v", err)
		}

		if len(req.Messages) != 1 {
			t.Fatalf("Expected 1 message, got %d", len(req.Messages))
		}
		if len(req.Messages[0].Content) != 1 {
			t.Fatalf("Expected 1 content block, got %d", len(req.Messages[0].Content))
		}
		if req.Messages[0].Content[0].Type != canonical.ContentImage {
			t.Errorf("Expected content type 'image', got '%s'", req.Messages[0].Content[0].Type)
		}
		if req.Messages[0].Content[0].Media == nil {
			t.Fatal("Expected media content, got nil")
		}
		if req.Messages[0].Content[0].Media.URL != "https://example.com/image.png" {
			t.Errorf("Expected URL 'https://example.com/image.png', got '%s'", req.Messages[0].Content[0].Media.URL)
		}
	})

	t.Run("image_base64", func(t *testing.T) {
		reqBody := []byte(`{
			"model": "claude-sonnet-4-20250514",
			"max_tokens": 1024,
			"messages": [
				{
					"role": "user",
					"content": [
						{"type": "image", "source": {"type": "base64", "media_type": "image/png", "data": "iVBORw0KGgo="}}
					]
				}
			]
		}`)

		req, err := adapter.ParseRequest(ctx, reqBody, http.Header{})
		if err != nil {
			t.Fatalf("ParseRequest failed: %v", err)
		}

		if len(req.Messages) != 1 {
			t.Fatalf("Expected 1 message, got %d", len(req.Messages))
		}
		if req.Messages[0].Content[0].Type != canonical.ContentImage {
			t.Errorf("Expected content type 'image', got '%s'", req.Messages[0].Content[0].Type)
		}
		if req.Messages[0].Content[0].Media == nil {
			t.Fatal("Expected media content, got nil")
		}
		if req.Messages[0].Content[0].Media.MimeType != "image/png" {
			t.Errorf("Expected MIME type 'image/png', got '%s'", req.Messages[0].Content[0].Media.MimeType)
		}
		if req.Messages[0].Content[0].Media.Base64 != "iVBORw0KGgo=" {
			t.Errorf("Expected base64 data, got '%s'", req.Messages[0].Content[0].Media.Base64)
		}
	})
}