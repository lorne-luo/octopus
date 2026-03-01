package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bestruirui/octopus/internal/transformer2/canonical"
	"github.com/bestruirui/octopus/internal/transformer2/canonical/test"
)

func TestBuildRequest_BasicChat(t *testing.T) {
	fixtures, err := test.LoadFixtures("01_basic_chat.json")
	if err != nil {
		t.Fatalf("Failed to load fixtures: %v", err)
	}

	var tc *test.TestCase
	for i := range fixtures {
		if fixtures[i].Name == "openai_chat_simple_text" {
			tc = &fixtures[i]
			break
		}
	}
	if tc == nil {
		t.Fatal("openai_chat_simple_text fixture not found")
	}

	adapter := NewChatProviderAdapter()
	ctx := context.Background()

	// Parse client request to get canonical request
	clientAdapter := NewChatClientAdapter()
	canonicalReq, err := clientAdapter.ParseRequest(ctx, tc.ClientRequest, http.Header{})
	if err != nil {
		t.Fatalf("ParseRequest failed: %v", err)
	}

	// Build HTTP request
	httpReq, err := adapter.BuildRequest(ctx, canonicalReq, "https://api.openai.com", "test-key")
	if err != nil {
		t.Fatalf("BuildRequest failed: %v", err)
	}

	// Verify HTTP method and URL
	if httpReq.Method != "POST" {
		t.Errorf("Expected POST method, got %s", httpReq.Method)
	}
	if httpReq.URL.String() != "https://api.openai.com/v1/chat/completions" {
		t.Errorf("Expected URL 'https://api.openai.com/v1/chat/completions', got %s", httpReq.URL.String())
	}

	// Verify headers
	if httpReq.Header.Get("Content-Type") != "application/json" {
		t.Errorf("Expected Content-Type 'application/json', got %s", httpReq.Header.Get("Content-Type"))
	}
	if !strings.HasPrefix(httpReq.Header.Get("Authorization"), "Bearer ") {
		t.Errorf("Expected Authorization header with Bearer token, got %s", httpReq.Header.Get("Authorization"))
	}

	// Verify body
	body, err := io.ReadAll(httpReq.Body)
	if err != nil {
		t.Fatalf("Failed to read request body: %v", err)
	}

	var req ChatCompletionRequest
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("Failed to unmarshal request body: %v", err)
	}

	if req.Model != "gpt-4o" {
		t.Errorf("Expected model 'gpt-4o', got '%s'", req.Model)
	}
	if len(req.Messages) != 1 {
		t.Errorf("Expected 1 message, got %d", len(req.Messages))
	}
}

func TestBuildRequest_MultiTurn(t *testing.T) {
	fixtures, err := test.LoadFixtures("01_basic_chat.json")
	if err != nil {
		t.Fatalf("Failed to load fixtures: %v", err)
	}

	var tc *test.TestCase
	for i := range fixtures {
		if fixtures[i].Name == "openai_chat_multi_turn" {
			tc = &fixtures[i]
			break
		}
	}
	if tc == nil {
		t.Fatal("openai_chat_multi_turn fixture not found")
	}

	adapter := NewChatProviderAdapter()
	ctx := context.Background()

	clientAdapter := NewChatClientAdapter()
	canonicalReq, err := clientAdapter.ParseRequest(ctx, tc.ClientRequest, http.Header{})
	if err != nil {
		t.Fatalf("ParseRequest failed: %v", err)
	}

	httpReq, err := adapter.BuildRequest(ctx, canonicalReq, "https://api.openai.com", "test-key")
	if err != nil {
		t.Fatalf("BuildRequest failed: %v", err)
	}

	body, err := io.ReadAll(httpReq.Body)
	if err != nil {
		t.Fatalf("Failed to read request body: %v", err)
	}

	var req ChatCompletionRequest
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("Failed to unmarshal request body: %v", err)
	}

	// Verify messages count
	if len(req.Messages) != 4 {
		t.Errorf("Expected 4 messages, got %d", len(req.Messages))
	}

	// Verify temperature and max_tokens
	if req.Temperature == nil || *req.Temperature != 0.7 {
		t.Errorf("Expected temperature 0.7, got %v", req.Temperature)
	}
	if req.MaxTokens == nil || *req.MaxTokens != 100 {
		t.Errorf("Expected max_tokens 100, got %v", req.MaxTokens)
	}
}

func TestBuildRequest_AllParams(t *testing.T) {
	fixtures, err := test.LoadFixtures("01_basic_chat.json")
	if err != nil {
		t.Fatalf("Failed to load fixtures: %v", err)
	}

	var tc *test.TestCase
	for i := range fixtures {
		if fixtures[i].Name == "openai_chat_with_all_params" {
			tc = &fixtures[i]
			break
		}
	}
	if tc == nil {
		t.Fatal("openai_chat_with_all_params fixture not found")
	}

	adapter := NewChatProviderAdapter()
	ctx := context.Background()

	clientAdapter := NewChatClientAdapter()
	canonicalReq, err := clientAdapter.ParseRequest(ctx, tc.ClientRequest, http.Header{})
	if err != nil {
		t.Fatalf("ParseRequest failed: %v", err)
	}

	httpReq, err := adapter.BuildRequest(ctx, canonicalReq, "https://api.openai.com", "test-key")
	if err != nil {
		t.Fatalf("BuildRequest failed: %v", err)
	}

	body, err := io.ReadAll(httpReq.Body)
	if err != nil {
		t.Fatalf("Failed to read request body: %v", err)
	}

	var req ChatCompletionRequest
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("Failed to unmarshal request body: %v", err)
	}

	// Verify all parameters are preserved
	tests := []struct {
		name     string
		expected interface{}
		actual   interface{}
	}{
		{"temperature", 0.9, float64PtrVal(req.Temperature)},
		{"top_p", 0.95, float64PtrVal(req.TopP)},
		{"max_tokens", int64(256), int64PtrVal(req.MaxTokens)},
		{"frequency_penalty", 0.5, float64PtrVal(req.FrequencyPenalty)},
		{"presence_penalty", 0.3, float64PtrVal(req.PresencePenalty)},
		{"seed", int64(42), int64PtrVal(req.Seed)},
		{"logprobs", true, boolPtrVal(req.Logprobs)},
	}

	for _, tt := range tests {
		if tt.expected != tt.actual {
			t.Errorf("%s: expected %v, got %v", tt.name, tt.expected, tt.actual)
		}
	}

	// Verify stop sequences
	if req.Stop == nil {
		t.Error("Expected stop sequences, got nil")
	} else if len(req.Stop.Multiple) != 2 {
		t.Errorf("Expected 2 stop sequences, got %d", len(req.Stop.Multiple))
	}

	// Verify user
	if req.User == nil || *req.User != "user-123" {
		t.Errorf("Expected user 'user-123', got %v", req.User)
	}
}

func TestBuildRequest_WithTools(t *testing.T) {
	fixtures, err := test.LoadFixtures("03_tool_calling.json")
	if err != nil {
		t.Fatalf("Failed to load fixtures: %v", err)
	}

	var tc *test.TestCase
	for i := range fixtures {
		if fixtures[i].Name == "openai_tool_definition" {
			tc = &fixtures[i]
			break
		}
	}
	if tc == nil {
		t.Fatal("openai_tool_definition fixture not found")
	}

	adapter := NewChatProviderAdapter()
	ctx := context.Background()

	clientAdapter := NewChatClientAdapter()
	canonicalReq, err := clientAdapter.ParseRequest(ctx, tc.ClientRequest, http.Header{})
	if err != nil {
		t.Fatalf("ParseRequest failed: %v", err)
	}

	httpReq, err := adapter.BuildRequest(ctx, canonicalReq, "https://api.openai.com", "test-key")
	if err != nil {
		t.Fatalf("BuildRequest failed: %v", err)
	}

	body, err := io.ReadAll(httpReq.Body)
	if err != nil {
		t.Fatalf("Failed to read request body: %v", err)
	}

	var req ChatCompletionRequest
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("Failed to unmarshal request body: %v", err)
	}

	// Verify tools
	if len(req.Tools) != 1 {
		t.Fatalf("Expected 1 tool, got %d", len(req.Tools))
	}

	tool := req.Tools[0]
	if tool.Type != "function" {
		t.Errorf("Expected tool type 'function', got '%s'", tool.Type)
	}
	if tool.Function == nil {
		t.Fatal("Expected function definition, got nil")
	}
	if tool.Function.Name != "get_weather" {
		t.Errorf("Expected function name 'get_weather', got '%s'", tool.Function.Name)
	}
	if tool.Function.Description != "Get weather for a location" {
		t.Errorf("Expected function description, got '%s'", tool.Function.Description)
	}

	// Verify tool_choice
	if req.ToolChoice == nil {
		t.Error("Expected tool_choice, got nil")
	} else if req.ToolChoice.Mode != "auto" {
		t.Errorf("Expected tool_choice mode 'auto', got '%s'", req.ToolChoice.Mode)
	}
}

func TestBuildRequest_ToolResultRoundTrip(t *testing.T) {
	fixtures, err := test.LoadFixtures("03_tool_calling.json")
	if err != nil {
		t.Fatalf("Failed to load fixtures: %v", err)
	}

	var tc *test.TestCase
	for i := range fixtures {
		if fixtures[i].Name == "openai_tool_result_round_trip" {
			tc = &fixtures[i]
			break
		}
	}
	if tc == nil {
		t.Fatal("openai_tool_result_round_trip fixture not found")
	}

	adapter := NewChatProviderAdapter()
	ctx := context.Background()

	clientAdapter := NewChatClientAdapter()
	canonicalReq, err := clientAdapter.ParseRequest(ctx, tc.ClientRequest, http.Header{})
	if err != nil {
		t.Fatalf("ParseRequest failed: %v", err)
	}

	httpReq, err := adapter.BuildRequest(ctx, canonicalReq, "https://api.openai.com", "test-key")
	if err != nil {
		t.Fatalf("BuildRequest failed: %v", err)
	}

	body, err := io.ReadAll(httpReq.Body)
	if err != nil {
		t.Fatalf("Failed to read request body: %v", err)
	}

	var req ChatCompletionRequest
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("Failed to unmarshal request body: %v", err)
	}

	// Verify 3 messages
	if len(req.Messages) != 3 {
		t.Fatalf("Expected 3 messages, got %d", len(req.Messages))
	}

	// First message: user
	if req.Messages[0].Role != "user" {
		t.Errorf("Expected first message role 'user', got '%s'", req.Messages[0].Role)
	}

	// Second message: assistant with tool_calls
	if req.Messages[1].Role != "assistant" {
		t.Errorf("Expected second message role 'assistant', got '%s'", req.Messages[1].Role)
	}
	if len(req.Messages[1].ToolCalls) != 1 {
		t.Errorf("Expected 1 tool_call, got %d", len(req.Messages[1].ToolCalls))
	}

	// Third message: tool result
	if req.Messages[2].Role != "tool" {
		t.Errorf("Expected third message role 'tool', got '%s'", req.Messages[2].Role)
	}
	if req.Messages[2].ToolCallID == nil || *req.Messages[2].ToolCallID != "call_abc123" {
		t.Errorf("Expected tool_call_id 'call_abc123', got %v", req.Messages[2].ToolCallID)
	}
}

func TestBuildRequest_WithStream(t *testing.T) {
	req := &canonical.Request{
		Kind:  canonical.KindChat,
		Model: "gpt-4o",
		Messages: []canonical.Message{
			{Role: canonical.RoleUser, Content: []canonical.ContentBlock{{Type: canonical.ContentText, Text: "Hello"}}},
		},
		Stream: true,
		StreamOptions: &canonical.StreamOptions{
			IncludeUsage: true,
		},
	}

	adapter := NewChatProviderAdapter()
	ctx := context.Background()

	httpReq, err := adapter.BuildRequest(ctx, req, "https://api.openai.com", "test-key")
	if err != nil {
		t.Fatalf("BuildRequest failed: %v", err)
	}

	body, err := io.ReadAll(httpReq.Body)
	if err != nil {
		t.Fatalf("Failed to read request body: %v", err)
	}

	var chatReq ChatCompletionRequest
	if err := json.Unmarshal(body, &chatReq); err != nil {
		t.Fatalf("Failed to unmarshal request body: %v", err)
	}

	if chatReq.Stream == nil || !*chatReq.Stream {
		t.Errorf("Expected stream true, got %v", chatReq.Stream)
	}
	if chatReq.StreamOptions == nil || !chatReq.StreamOptions.IncludeUsage {
		t.Errorf("Expected stream_options.include_usage true, got %v", chatReq.StreamOptions)
	}
}

func TestBuildRequest_WithResponseFormat(t *testing.T) {
	schema := json.RawMessage(`{"type": "object", "properties": {"name": {"type": "string"}}}`)

	req := &canonical.Request{
		Kind:  canonical.KindChat,
		Model: "gpt-4o",
		Messages: []canonical.Message{
			{Role: canonical.RoleUser, Content: []canonical.ContentBlock{{Type: canonical.ContentText, Text: "Generate JSON"}}},
		},
		ResponseFormat: &canonical.ResponseFormat{
			Type: "json_schema",
			JsonSchema: &canonical.ResponseSchema{
				Name:   "test_schema",
				Schema: schema,
			},
		},
	}

	adapter := NewChatProviderAdapter()
	ctx := context.Background()

	httpReq, err := adapter.BuildRequest(ctx, req, "https://api.openai.com", "test-key")
	if err != nil {
		t.Fatalf("BuildRequest failed: %v", err)
	}

	body, err := io.ReadAll(httpReq.Body)
	if err != nil {
		t.Fatalf("Failed to read request body: %v", err)
	}

	var chatReq ChatCompletionRequest
	if err := json.Unmarshal(body, &chatReq); err != nil {
		t.Fatalf("Failed to unmarshal request body: %v", err)
	}

	if chatReq.ResponseFormat == nil {
		t.Fatal("Expected response_format, got nil")
	}
	if chatReq.ResponseFormat.Type != "json_schema" {
		t.Errorf("Expected response_format type 'json_schema', got '%s'", chatReq.ResponseFormat.Type)
	}
}

func TestParseResponse_Success(t *testing.T) {
	fixtures, err := test.LoadFixtures("01_basic_chat.json")
	if err != nil {
		t.Fatalf("Failed to load fixtures: %v", err)
	}

	var tc *test.TestCase
	for i := range fixtures {
		if fixtures[i].Name == "openai_chat_simple_text" {
			tc = &fixtures[i]
			break
		}
	}
	if tc == nil {
		t.Fatal("openai_chat_simple_text fixture not found")
	}

	adapter := NewChatProviderAdapter()
	ctx := context.Background()

	// Create HTTP response from provider_response
	body, err := json.Marshal(tc.ProviderResponse)
	if err != nil {
		t.Fatalf("Failed to marshal provider response: %v", err)
	}

	httpResp := &http.Response{
		StatusCode: tc.ProviderStatusCode,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader(body)),
	}
	httpResp.Header.Set("Content-Type", "application/json")

	resp, err := adapter.ParseResponse(ctx, httpResp)
	if err != nil {
		t.Fatalf("ParseResponse failed: %v", err)
	}

	// Verify response
	if resp.StatusCode != 200 {
		t.Errorf("Expected status code 200, got %d", resp.StatusCode)
	}
	if resp.ID != "chatcmpl-abc123" {
		t.Errorf("Expected ID 'chatcmpl-abc123', got '%s'", resp.ID)
	}
	if resp.Model != "gpt-4o" {
		t.Errorf("Expected model 'gpt-4o', got '%s'", resp.Model)
	}

	// Verify choices
	if len(resp.Choices) != 1 {
		t.Fatalf("Expected 1 choice, got %d", len(resp.Choices))
	}
	if resp.Choices[0].Message.Role != canonical.RoleAssistant {
		t.Errorf("Expected role 'assistant', got '%s'", resp.Choices[0].Message.Role)
	}
	if len(resp.Choices[0].Message.Content) != 1 {
		t.Fatalf("Expected 1 content block, got %d", len(resp.Choices[0].Message.Content))
	}
	if resp.Choices[0].Message.Content[0].Text != "Hi there!" {
		t.Errorf("Expected content 'Hi there!', got '%s'", resp.Choices[0].Message.Content[0].Text)
	}

	// Verify finish reason
	if resp.Choices[0].FinishReason == nil || *resp.Choices[0].FinishReason != "stop" {
		t.Errorf("Expected finish_reason 'stop', got %v", resp.Choices[0].FinishReason)
	}

	// Verify usage
	if resp.Usage == nil {
		t.Fatal("Expected usage, got nil")
	}
	if resp.Usage.PromptTokens != 5 {
		t.Errorf("Expected prompt_tokens 5, got %d", resp.Usage.PromptTokens)
	}
	if resp.Usage.CompletionTokens != 4 {
		t.Errorf("Expected completion_tokens 4, got %d", resp.Usage.CompletionTokens)
	}
}

func TestParseResponse_ToolCalls(t *testing.T) {
	fixtures, err := test.LoadFixtures("03_tool_calling.json")
	if err != nil {
		t.Fatalf("Failed to load fixtures: %v", err)
	}

	var tc *test.TestCase
	for i := range fixtures {
		if fixtures[i].Name == "openai_tool_call_response" {
			tc = &fixtures[i]
			break
		}
	}
	if tc == nil {
		t.Fatal("openai_tool_call_response fixture not found")
	}

	adapter := NewChatProviderAdapter()
	ctx := context.Background()

	body, err := json.Marshal(tc.ProviderResponse)
	if err != nil {
		t.Fatalf("Failed to marshal provider response: %v", err)
	}

	httpResp := &http.Response{
		StatusCode: tc.ProviderStatusCode,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader(body)),
	}
	httpResp.Header.Set("Content-Type", "application/json")

	resp, err := adapter.ParseResponse(ctx, httpResp)
	if err != nil {
		t.Fatalf("ParseResponse failed: %v", err)
	}

	// Verify tool_calls in response
	if len(resp.Choices) != 1 {
		t.Fatalf("Expected 1 choice, got %d", len(resp.Choices))
	}
	if len(resp.Choices[0].Message.ToolCalls) != 1 {
		t.Fatalf("Expected 1 tool_call, got %d", len(resp.Choices[0].Message.ToolCalls))
	}

	tcResult := resp.Choices[0].Message.ToolCalls[0]
	if tcResult.ID != "call_abc123" {
		t.Errorf("Expected tool_call id 'call_abc123', got '%s'", tcResult.ID)
	}
	if tcResult.Type != "function" {
		t.Errorf("Expected tool_call type 'function', got '%s'", tcResult.Type)
	}
	if tcResult.Name != "get_weather" {
		t.Errorf("Expected function name 'get_weather', got '%s'", tcResult.Name)
	}
	if tcResult.Arguments != "{\"location\": \"Tokyo\", \"unit\": \"celsius\"}" {
		t.Errorf("Expected arguments, got '%s'", tcResult.Arguments)
	}

	// Verify finish reason
	if resp.Choices[0].FinishReason == nil || *resp.Choices[0].FinishReason != "tool_calls" {
		t.Errorf("Expected finish_reason 'tool_calls', got %v", resp.Choices[0].FinishReason)
	}
}

func TestParseResponse_Error(t *testing.T) {
	adapter := NewChatProviderAdapter()
	ctx := context.Background()

	tests := []struct {
		name       string
		statusCode int
		body       string
		wantCode   string
		wantMsg    string
		wantType   string
	}{
		{
			name:       "invalid_api_key",
			statusCode: 401,
			body:       `{"error": {"code": "invalid_api_key", "message": "Invalid API key provided", "type": "invalid_request_error"}}`,
			wantCode:   "invalid_api_key",
			wantMsg:    "Invalid API key provided",
			wantType:   "invalid_request_error",
		},
		{
			name:       "rate_limit",
			statusCode: 429,
			body:       `{"error": {"code": "rate_limit_exceeded", "message": "Rate limit exceeded", "type": "rate_limit_error"}}`,
			wantCode:   "rate_limit_exceeded",
			wantMsg:    "Rate limit exceeded",
			wantType:   "rate_limit_error",
		},
		{
			name:       "generic_error",
			statusCode: 500,
			body:       `Internal Server Error`,
			wantCode:   "",
			wantMsg:    "Internal Server Error",
			wantType:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpResp := &http.Response{
				StatusCode: tt.statusCode,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(tt.body)),
			}
			httpResp.Header.Set("Content-Type", "application/json")

			resp, err := adapter.ParseResponse(ctx, httpResp)
			if err != nil {
				t.Fatalf("ParseResponse failed: %v", err)
			}

			if resp.StatusCode != tt.statusCode {
				t.Errorf("Expected status code %d, got %d", tt.statusCode, resp.StatusCode)
			}
			if resp.Error == nil {
				t.Fatal("Expected error, got nil")
			}
			if resp.Error.Code != tt.wantCode {
				t.Errorf("Expected error code '%s', got '%s'", tt.wantCode, resp.Error.Code)
			}
			if resp.Error.Message != tt.wantMsg {
				t.Errorf("Expected error message '%s', got '%s'", tt.wantMsg, resp.Error.Message)
			}
			if resp.Error.Type != tt.wantType {
				t.Errorf("Expected error type '%s', got '%s'", tt.wantType, resp.Error.Type)
			}
		})
	}
}

func TestParseResponse_WithUsage(t *testing.T) {
	adapter := NewChatProviderAdapter()
	ctx := context.Background()

	providerResp := ChatCompletionResponse{
		ID:      "chatcmpl-123",
		Object:  "chat.completion",
		Created: 1700000000,
		Model:   "gpt-4o",
		Choices: []ChatChoice{
			{
				Index: 0,
				Message: &ChatMessage{
					Role: "assistant",
					Content: MessageContent{Text: strPtr("Hello")},
				},
				FinishReason: strPtr("stop"),
			},
		},
		Usage: &Usage{
			PromptTokens:     100,
			CompletionTokens: 50,
			TotalTokens:      150,
			PromptTokensDetails: &PromptTokensDetails{
				CachedTokens: 20,
				AudioTokens:  5,
			},
			CompletionTokensDetails: &CompletionTokensDetails{
				ReasoningTokens: 10,
				AudioTokens:     2,
			},
		},
	}

	body, err := json.Marshal(providerResp)
	if err != nil {
		t.Fatalf("Failed to marshal response: %v", err)
	}

	httpResp := &http.Response{
		StatusCode: 200,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader(body)),
	}
	httpResp.Header.Set("Content-Type", "application/json")

	resp, err := adapter.ParseResponse(ctx, httpResp)
	if err != nil {
		t.Fatalf("ParseResponse failed: %v", err)
	}

	if resp.Usage == nil {
		t.Fatal("Expected usage, got nil")
	}
	if resp.Usage.PromptTokens != 100 {
		t.Errorf("Expected prompt_tokens 100, got %d", resp.Usage.PromptTokens)
	}
	if resp.Usage.CompletionTokens != 50 {
		t.Errorf("Expected completion_tokens 50, got %d", resp.Usage.CompletionTokens)
	}
	if resp.Usage.TotalTokens != 150 {
		t.Errorf("Expected total_tokens 150, got %d", resp.Usage.TotalTokens)
	}

	// Verify usage details
	if resp.Usage.PromptTokensDetails == nil {
		t.Fatal("Expected prompt tokens details, got nil")
	}
	if resp.Usage.PromptTokensDetails.CachedTokens != 20 {
		t.Errorf("Expected cached_tokens 20, got %d", resp.Usage.PromptTokensDetails.CachedTokens)
	}
	if resp.Usage.CompletionTokensDetails == nil {
		t.Fatal("Expected completion tokens details, got nil")
	}
	if resp.Usage.CompletionTokensDetails.ReasoningTokens != 10 {
		t.Errorf("Expected reasoning_tokens 10, got %d", resp.Usage.CompletionTokensDetails.ReasoningTokens)
	}
}

func TestParseStreamChunk(t *testing.T) {
	adapter := NewChatProviderAdapter()
	ctx := context.Background()

	tests := []struct {
		name       string
		input      string
		wantDone   bool
		wantID     string
		wantModel  string
		wantDelta  string
		wantFinish string
	}{
		{
			name:      "role_chunk",
			input:     `{"id":"chatcmpl-123","object":"chat.completion.chunk","created":1700000000,"model":"gpt-4o","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
			wantID:    "chatcmpl-123",
			wantModel: "gpt-4o",
			wantDelta: "", // role delta, no content
		},
		{
			name:      "content_chunk",
			input:     `{"id":"chatcmpl-123","object":"chat.completion.chunk","created":1700000000,"model":"gpt-4o","choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}`,
			wantID:    "chatcmpl-123",
			wantModel: "gpt-4o",
			wantDelta: "Hello",
		},
		{
			name:       "finish_chunk",
			input:      `{"id":"chatcmpl-123","object":"chat.completion.chunk","created":1700000000,"model":"gpt-4o","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
			wantID:     "chatcmpl-123",
			wantModel:  "gpt-4o",
			wantFinish: "stop",
		},
		{
			name:     "done_marker",
			input:    "[DONE]",
			wantDone: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chunk, err := adapter.ParseStreamChunk(ctx, []byte(tt.input))
			if err != nil {
				t.Fatalf("ParseStreamChunk failed: %v", err)
			}

			if tt.wantDone {
				if !chunk.Done {
					t.Error("Expected chunk.Done to be true")
				}
				return
			}

			if chunk.Done {
				t.Error("Expected chunk.Done to be false")
			}
			if chunk.ID != tt.wantID {
				t.Errorf("Expected ID '%s', got '%s'", tt.wantID, chunk.ID)
			}
			if chunk.Model != tt.wantModel {
				t.Errorf("Expected model '%s', got '%s'", tt.wantModel, chunk.Model)
			}

			if tt.wantDelta != "" {
				if len(chunk.Deltas) != 1 {
					t.Fatalf("Expected 1 delta, got %d", len(chunk.Deltas))
				}
				if len(chunk.Deltas[0].Delta.Content) != 1 {
					t.Fatalf("Expected 1 content block, got %d", len(chunk.Deltas[0].Delta.Content))
				}
				if chunk.Deltas[0].Delta.Content[0].Text != tt.wantDelta {
					t.Errorf("Expected delta content '%s', got '%s'", tt.wantDelta, chunk.Deltas[0].Delta.Content[0].Text)
				}
			}

			if tt.wantFinish != "" {
				if len(chunk.Deltas) != 1 {
					t.Fatalf("Expected 1 delta, got %d", len(chunk.Deltas))
				}
				if chunk.Deltas[0].FinishReason == nil || *chunk.Deltas[0].FinishReason != tt.wantFinish {
					t.Errorf("Expected finish_reason '%s', got %v", tt.wantFinish, chunk.Deltas[0].FinishReason)
				}
			}
		})
	}
}

func TestParseStreamChunk_ToolCalls(t *testing.T) {
	adapter := NewChatProviderAdapter()
	ctx := context.Background()

	input := `{"id":"chatcmpl-123","object":"chat.completion.chunk","created":1700000000,"model":"gpt-4o","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"get_weather","arguments":"{\"loc"}}]},"finish_reason":null}]}`

	chunk, err := adapter.ParseStreamChunk(ctx, []byte(input))
	if err != nil {
		t.Fatalf("ParseStreamChunk failed: %v", err)
	}

	if len(chunk.Deltas) != 1 {
		t.Fatalf("Expected 1 delta, got %d", len(chunk.Deltas))
	}
	if len(chunk.Deltas[0].Delta.ToolCalls) != 1 {
		t.Fatalf("Expected 1 tool_call, got %d", len(chunk.Deltas[0].Delta.ToolCalls))
	}

	tc := chunk.Deltas[0].Delta.ToolCalls[0]
	if tc.ID != "call_1" {
		t.Errorf("Expected tool_call id 'call_1', got '%s'", tc.ID)
	}
	if tc.Type != "function" {
		t.Errorf("Expected tool_call type 'function', got '%s'", tc.Type)
	}
	if tc.Name != "get_weather" {
		t.Errorf("Expected function name 'get_weather', got '%s'", tc.Name)
	}
	if tc.Arguments != "{\"loc" {
		t.Errorf("Expected arguments '{\"loc', got '%s'", tc.Arguments)
	}
}

func TestParseStreamChunk_Usage(t *testing.T) {
	adapter := NewChatProviderAdapter()
	ctx := context.Background()

	input := `{"id":"chatcmpl-123","object":"chat.completion.chunk","created":1700000000,"model":"gpt-4o","choices":[],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`

	chunk, err := adapter.ParseStreamChunk(ctx, []byte(input))
	if err != nil {
		t.Fatalf("ParseStreamChunk failed: %v", err)
	}

	if chunk.Usage == nil {
		t.Fatal("Expected usage, got nil")
	}
	if chunk.Usage.PromptTokens != 10 {
		t.Errorf("Expected prompt_tokens 10, got %d", chunk.Usage.PromptTokens)
	}
	if chunk.Usage.CompletionTokens != 5 {
		t.Errorf("Expected completion_tokens 5, got %d", chunk.Usage.CompletionTokens)
	}
	if chunk.Usage.TotalTokens != 15 {
		t.Errorf("Expected total_tokens 15, got %d", chunk.Usage.TotalTokens)
	}
}

func TestBuildRequest_TrailingSlash(t *testing.T) {
	req := &canonical.Request{
		Kind:  canonical.KindChat,
		Model: "gpt-4o",
		Messages: []canonical.Message{
			{Role: canonical.RoleUser, Content: []canonical.ContentBlock{{Type: canonical.ContentText, Text: "Hello"}}},
		},
	}

	adapter := NewChatProviderAdapter()
	ctx := context.Background()

	tests := []struct {
		name     string
		baseURL  string
		wantURL  string
	}{
		{
			name:    "no_trailing_slash",
			baseURL: "https://api.openai.com",
			wantURL: "https://api.openai.com/v1/chat/completions",
		},
		{
			name:    "with_trailing_slash",
			baseURL: "https://api.openai.com/",
			wantURL: "https://api.openai.com/v1/chat/completions",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpReq, err := adapter.BuildRequest(ctx, req, tt.baseURL, "test-key")
			if err != nil {
				t.Fatalf("BuildRequest failed: %v", err)
			}
			if httpReq.URL.String() != tt.wantURL {
				t.Errorf("Expected URL '%s', got '%s'", tt.wantURL, httpReq.URL.String())
			}
		})
	}
}

func TestParseStreamChunk_ReasoningContent(t *testing.T) {
	adapter := NewChatProviderAdapter()
	ctx := context.Background()

	input := `{"id":"chatcmpl-123","object":"chat.completion.chunk","created":1700000000,"model":"gpt-4o","choices":[{"index":0,"delta":{"reasoning_content":"Step 1: analyze..."},"finish_reason":null}]}`

	chunk, err := adapter.ParseStreamChunk(ctx, []byte(input))
	if err != nil {
		t.Fatalf("ParseStreamChunk failed: %v", err)
	}

	if len(chunk.Deltas) != 1 {
		t.Fatalf("Expected 1 delta, got %d", len(chunk.Deltas))
	}

	if chunk.Deltas[0].Delta.Reasoning == nil {
		t.Fatal("Expected reasoning content, got nil")
	}
	if *chunk.Deltas[0].Delta.Reasoning != "Step 1: analyze..." {
		t.Errorf("Expected reasoning 'Step 1: analyze...', got '%s'", *chunk.Deltas[0].Delta.Reasoning)
	}
}

// Integration test using httptest
func TestProviderAdapter_EndToEnd(t *testing.T) {
	// Create test server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		if r.Method != "POST" {
			t.Errorf("Expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("Expected path '/v1/chat/completions', got '%s'", r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Expected Content-Type 'application/json', got '%s'", r.Header.Get("Content-Type"))
		}

		// Return mock response
		resp := ChatCompletionResponse{
			ID:      "chatcmpl-test",
			Object:  "chat.completion",
			Created: 1700000000,
			Model:   "gpt-4o",
			Choices: []ChatChoice{
				{
					Index: 0,
					Message: &ChatMessage{
						Role:    "assistant",
						Content: MessageContent{Text: strPtr("Hello, world!")},
					},
					FinishReason: strPtr("stop"),
				},
			},
			Usage: &Usage{
				PromptTokens:     10,
				CompletionTokens: 5,
				TotalTokens:      15,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	// Build request
	providerAdapter := NewChatProviderAdapter()
	ctx := context.Background()

	req := &canonical.Request{
		Kind:  canonical.KindChat,
		Model: "gpt-4o",
		Messages: []canonical.Message{
			{Role: canonical.RoleUser, Content: []canonical.ContentBlock{{Type: canonical.ContentText, Text: "Hello"}}},
		},
	}

	httpReq, err := providerAdapter.BuildRequest(ctx, req, ts.URL, "test-key")
	if err != nil {
		t.Fatalf("BuildRequest failed: %v", err)
	}

	// Execute request
	client := &http.Client{}
	httpResp, err := client.Do(httpReq)
	if err != nil {
		t.Fatalf("HTTP request failed: %v", err)
	}
	defer httpResp.Body.Close()

	// Parse response
	resp, err := providerAdapter.ParseResponse(ctx, httpResp)
	if err != nil {
		t.Fatalf("ParseResponse failed: %v", err)
	}

	// Verify response
	if resp.ID != "chatcmpl-test" {
		t.Errorf("Expected ID 'chatcmpl-test', got '%s'", resp.ID)
	}
	if len(resp.Choices) != 1 {
		t.Fatalf("Expected 1 choice, got %d", len(resp.Choices))
	}
	if resp.Choices[0].Message.Content[0].Text != "Hello, world!" {
		t.Errorf("Expected content 'Hello, world!', got '%s'", resp.Choices[0].Message.Content[0].Text)
	}
}

