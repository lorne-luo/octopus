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
)

func TestResponsesBuildRequest_BasicInput(t *testing.T) {
	adapter := NewResponsesProviderAdapter()
	ctx := context.Background()

	// Create canonical request with simple user message
	req := &canonical.Request{
		Kind:  canonical.KindChat,
		Model: "gpt-4o",
		Messages: []canonical.Message{
			{
				Role: canonical.RoleUser,
				Content: []canonical.ContentBlock{{
					Type: canonical.ContentText,
					Text: "Hello, world!",
				}},
			},
		},
	}

	httpReq, err := adapter.BuildRequest(ctx, req, "https://api.openai.com/v1", "test-key")
	if err != nil {
		t.Fatalf("BuildRequest failed: %v", err)
	}

	// Verify HTTP method and URL
	if httpReq.Method != "POST" {
		t.Errorf("Expected POST method, got %s", httpReq.Method)
	}
	// Base URL with /v1 prefix, endpoint is /responses
	if httpReq.URL.String() != "https://api.openai.com/v1/responses" {
		t.Errorf("Expected URL 'https://api.openai.com/v1/responses', got %s", httpReq.URL.String())
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

	var respReq ResponsesRequest
	if err := json.Unmarshal(body, &respReq); err != nil {
		t.Fatalf("Failed to unmarshal request body: %v", err)
	}

	if respReq.Model != "gpt-4o" {
		t.Errorf("Expected model 'gpt-4o', got '%s'", respReq.Model)
	}

	// Verify input is a string (simple user message)
	if !respReq.Input.IsString() {
		t.Errorf("Expected input to be string, got items: %+v", respReq.Input.Items)
	}
	if respReq.Input.Text != "Hello, world!" {
		t.Errorf("Expected input text 'Hello, world!', got '%s'", respReq.Input.Text)
	}
}

func TestResponsesBuildRequest_ArrayInput(t *testing.T) {
	adapter := NewResponsesProviderAdapter()
	ctx := context.Background()

	trueVal := true
	req := &canonical.Request{
		Kind:  canonical.KindChat,
		Model: "gpt-4o",
		Messages: []canonical.Message{
			{
				Role: canonical.RoleUser,
				Content: []canonical.ContentBlock{{
					Type: canonical.ContentText,
					Text: "Hello",
				}},
			},
			{
				Role: canonical.RoleAssistant,
				Content: []canonical.ContentBlock{{
					Type: canonical.ContentText,
					Text: "Hi there!",
				}},
			},
			{
				Role: canonical.RoleUser,
				Content: []canonical.ContentBlock{{
					Type: canonical.ContentText,
					Text: "How are you?",
				}},
			},
		},
		Hints: canonical.TransformHints{
			ResponsesArrayInput: &trueVal,
		},
	}

	httpReq, err := adapter.BuildRequest(ctx, req, "https://api.openai.com/v1", "test-key")
	if err != nil {
		t.Fatalf("BuildRequest failed: %v", err)
	}

	body, err := io.ReadAll(httpReq.Body)
	if err != nil {
		t.Fatalf("Failed to read request body: %v", err)
	}

	var respReq ResponsesRequest
	if err := json.Unmarshal(body, &respReq); err != nil {
		t.Fatalf("Failed to unmarshal request body: %v", err)
	}

	// Verify input is an array
	if respReq.Input.IsString() {
		t.Errorf("Expected input to be array, got string: '%s'", respReq.Input.Text)
	}
	if len(respReq.Input.Items) != 3 {
		t.Errorf("Expected 3 input items, got %d", len(respReq.Input.Items))
	}
}

func TestResponsesBuildRequest_WithInstructions(t *testing.T) {
	adapter := NewResponsesProviderAdapter()
	ctx := context.Background()

	req := &canonical.Request{
		Kind:  canonical.KindChat,
		Model: "gpt-4o",
		Messages: []canonical.Message{
			{
				Role: canonical.RoleSystem,
				Content: []canonical.ContentBlock{{
					Type: canonical.ContentText,
					Text: "You are a helpful assistant.",
				}},
			},
			{
				Role: canonical.RoleUser,
				Content: []canonical.ContentBlock{{
					Type: canonical.ContentText,
					Text: "Hello",
				}},
			},
		},
	}

	httpReq, err := adapter.BuildRequest(ctx, req, "https://api.openai.com/v1", "test-key")
	if err != nil {
		t.Fatalf("BuildRequest failed: %v", err)
	}

	body, err := io.ReadAll(httpReq.Body)
	if err != nil {
		t.Fatalf("Failed to read request body: %v", err)
	}

	var respReq ResponsesRequest
	if err := json.Unmarshal(body, &respReq); err != nil {
		t.Fatalf("Failed to unmarshal request body: %v", err)
	}

	// Verify instructions
	if respReq.Instructions == nil || *respReq.Instructions != "You are a helpful assistant." {
		t.Errorf("Expected instructions 'You are a helpful assistant.', got %v", respReq.Instructions)
	}

	// Verify only user message is in input
	if respReq.Input.Text != "Hello" {
		t.Errorf("Expected input text 'Hello', got '%s'", respReq.Input.Text)
	}
}

func TestResponsesBuildRequest_WithTools(t *testing.T) {
	adapter := NewResponsesProviderAdapter()
	ctx := context.Background()

	params := json.RawMessage(`{"type": "object", "properties": {"location": {"type": "string"}}}`)
	req := &canonical.Request{
		Kind:  canonical.KindChat,
		Model: "gpt-4o",
		Messages: []canonical.Message{
			{
				Role: canonical.RoleUser,
				Content: []canonical.ContentBlock{{
					Type: canonical.ContentText,
					Text: "What's the weather?",
				}},
			},
		},
		Tools: []canonical.Tool{{
			Type:        "function",
			Name:        "get_weather",
			Description: "Get weather for a location",
			Parameters:  params,
		}},
		ToolChoice: &canonical.ToolChoice{
			Mode: "auto",
		},
	}

	httpReq, err := adapter.BuildRequest(ctx, req, "https://api.openai.com/v1", "test-key")
	if err != nil {
		t.Fatalf("BuildRequest failed: %v", err)
	}

	body, err := io.ReadAll(httpReq.Body)
	if err != nil {
		t.Fatalf("Failed to read request body: %v", err)
	}

	var respReq ResponsesRequest
	if err := json.Unmarshal(body, &respReq); err != nil {
		t.Fatalf("Failed to unmarshal request body: %v", err)
	}

	// Verify tools
	if len(respReq.Tools) != 1 {
		t.Fatalf("Expected 1 tool, got %d", len(respReq.Tools))
	}
	if respReq.Tools[0].Type != "function" {
		t.Errorf("Expected tool type 'function', got '%s'", respReq.Tools[0].Type)
	}
	if respReq.Tools[0].Name != "get_weather" {
		t.Errorf("Expected tool name 'get_weather', got '%s'", respReq.Tools[0].Name)
	}

	// Verify tool_choice
	if respReq.ToolChoice == nil || respReq.ToolChoice.Mode != "auto" {
		t.Errorf("Expected tool_choice mode 'auto', got %v", respReq.ToolChoice)
	}
}

func TestResponsesBuildRequest_ImageGenerationTool(t *testing.T) {
	adapter := NewResponsesProviderAdapter()
	ctx := context.Background()

	req := &canonical.Request{
		Kind:  canonical.KindChat,
		Model: "gpt-4o",
		Messages: []canonical.Message{
			{
				Role: canonical.RoleUser,
				Content: []canonical.ContentBlock{{
					Type: canonical.ContentText,
					Text: "Generate an image of a sunset",
				}},
			},
		},
		Tools: []canonical.Tool{{
			Type: "image_generation",
			ImageGeneration: &canonical.ImageGenerationConfig{
				Background:   "opaque",
				OutputFormat: "png",
				Quality:      "hd",
				Size:         "1024x1024",
			},
		}},
	}

	httpReq, err := adapter.BuildRequest(ctx, req, "https://api.openai.com/v1", "test-key")
	if err != nil {
		t.Fatalf("BuildRequest failed: %v", err)
	}

	body, err := io.ReadAll(httpReq.Body)
	if err != nil {
		t.Fatalf("Failed to read request body: %v", err)
	}

	var respReq ResponsesRequest
	if err := json.Unmarshal(body, &respReq); err != nil {
		t.Fatalf("Failed to unmarshal request body: %v", err)
	}

	// Verify image_generation tool
	if len(respReq.Tools) != 1 {
		t.Fatalf("Expected 1 tool, got %d", len(respReq.Tools))
	}
	if respReq.Tools[0].Type != "image_generation" {
		t.Errorf("Expected tool type 'image_generation', got '%s'", respReq.Tools[0].Type)
	}
	if respReq.Tools[0].Background != "opaque" {
		t.Errorf("Expected background 'opaque', got '%s'", respReq.Tools[0].Background)
	}
	if respReq.Tools[0].OutputFormat != "png" {
		t.Errorf("Expected output_format 'png', got '%s'", respReq.Tools[0].OutputFormat)
	}
}

func TestResponsesParseResponse_Success(t *testing.T) {
	adapter := NewResponsesProviderAdapter()
	ctx := context.Background()

	providerResp := ResponsesResponse{
		ID:      "resp_abc123",
		Object:  "response",
		Created: 1700000000,
		Model:   "gpt-4o",
		Status:  "completed",
		Output: []ResponsesOutputItem{
			{
				Type: "message",
				ID:   "msg_1",
				Role: "assistant",
				Content: []ResponsesContent{{
					Type: "output_text",
					Text: "Hello! How can I help you?",
				}},
			},
		},
		Usage: &ResponsesUsage{
			InputTokens:  10,
			OutputTokens: 8,
			TotalTokens:  18,
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

	// Verify response
	if resp.StatusCode != 200 {
		t.Errorf("Expected status code 200, got %d", resp.StatusCode)
	}
	if resp.ID != "resp_abc123" {
		t.Errorf("Expected ID 'resp_abc123', got '%s'", resp.ID)
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
	if resp.Choices[0].Message.Content[0].Text != "Hello! How can I help you?" {
		t.Errorf("Expected content 'Hello! How can I help you?', got '%s'", resp.Choices[0].Message.Content[0].Text)
	}

	// Verify finish reason
	if resp.Choices[0].FinishReason == nil || *resp.Choices[0].FinishReason != "stop" {
		t.Errorf("Expected finish_reason 'stop', got %v", resp.Choices[0].FinishReason)
	}

	// Verify usage
	if resp.Usage == nil {
		t.Fatal("Expected usage, got nil")
	}
	if resp.Usage.PromptTokens != 10 {
		t.Errorf("Expected prompt_tokens 10, got %d", resp.Usage.PromptTokens)
	}
	if resp.Usage.CompletionTokens != 8 {
		t.Errorf("Expected completion_tokens 8, got %d", resp.Usage.CompletionTokens)
	}
}

func TestResponsesParseResponse_FunctionCall(t *testing.T) {
	adapter := NewResponsesProviderAdapter()
	ctx := context.Background()

	providerResp := ResponsesResponse{
		ID:      "resp_tool1",
		Object:  "response",
		Created: 1700000000,
		Model:   "gpt-4o",
		Status:  "completed",
		Output: []ResponsesOutputItem{
			{
				Type:      "function_call",
				CallID:    "call_abc123",
				Name:      "get_weather",
				Arguments: `{"location": "Tokyo"}`,
			},
		},
		Usage: &ResponsesUsage{
			InputTokens:  15,
			OutputTokens: 10,
			TotalTokens:  25,
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
	if tcResult.Arguments != `{"location": "Tokyo"}` {
		t.Errorf("Expected arguments '{\"location\": \"Tokyo\"}', got '%s'", tcResult.Arguments)
	}

	// Verify finish reason
	if resp.Choices[0].FinishReason == nil || *resp.Choices[0].FinishReason != "tool_calls" {
		t.Errorf("Expected finish_reason 'tool_calls', got %v", resp.Choices[0].FinishReason)
	}
}

func TestResponsesParseResponse_Reasoning(t *testing.T) {
	adapter := NewResponsesProviderAdapter()
	ctx := context.Background()

	providerResp := ResponsesResponse{
		ID:      "resp_reasoning1",
		Object:  "response",
		Created: 1700000000,
		Model:   "o1",
		Status:  "completed",
		Output: []ResponsesOutputItem{
			{
				Type: "reasoning",
				Summary: []ResponsesSummary{{
					Type: "summary_text",
					Text: "Step 1: Analyze the problem...",
				}},
			},
			{
				Type: "message",
				ID:   "msg_1",
				Role: "assistant",
				Content: []ResponsesContent{{
					Type: "output_text",
					Text: "The answer is 42.",
				}},
			},
		},
		Usage: &ResponsesUsage{
			InputTokens:  20,
			OutputTokens: 50,
			TotalTokens:  70,
			OutputTokensDetails: &ResponsesOutputTokensDetails{
				ReasoningTokens: 30,
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

	// Verify reasoning content
	if resp.Choices[0].Message.Reasoning == nil {
		t.Fatal("Expected reasoning content, got nil")
	}
	if *resp.Choices[0].Message.Reasoning != "Step 1: Analyze the problem..." {
		t.Errorf("Expected reasoning 'Step 1: Analyze the problem...', got '%s'", *resp.Choices[0].Message.Reasoning)
	}

	// Verify text content
	if resp.Choices[0].Message.Content[0].Text != "The answer is 42." {
		t.Errorf("Expected content 'The answer is 42.', got '%s'", resp.Choices[0].Message.Content[0].Text)
	}

	// Verify reasoning tokens in usage
	if resp.Usage.CompletionTokensDetails == nil {
		t.Fatal("Expected completion tokens details, got nil")
	}
	if resp.Usage.CompletionTokensDetails.ReasoningTokens != 30 {
		t.Errorf("Expected reasoning_tokens 30, got %d", resp.Usage.CompletionTokensDetails.ReasoningTokens)
	}
}

func TestResponsesParseStreamChunk_Events(t *testing.T) {
	adapter := NewResponsesProviderAdapter()
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
			name: "response_created",
			input: `{"type":"response.created","response":{"id":"resp_123","model":"gpt-4o","created":1700000000,"status":"in_progress"}}`,
			wantID:    "resp_123",
			wantModel: "gpt-4o",
		},
		{
			name: "output_text_delta",
			input: `{"type":"response.output_text.delta","delta":"Hello","output_index":0,"content_index":0}`,
			wantDelta: "Hello",
		},
		{
			name: "function_call_arguments_delta",
			input: `{"type":"response.function_call_arguments.delta","delta":"{\"loc","output_index":0}`,
			wantDelta: "{\"loc",
		},
		{
			name: "response_completed",
			input: `{"type":"response.completed","response":{"id":"resp_123","model":"gpt-4o","status":"completed","usage":{"input_tokens":10,"output_tokens":5,"total_tokens":15}}}`,
			wantID:     "resp_123",
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

			if tt.wantID != "" && chunk.ID != tt.wantID {
				t.Errorf("Expected ID '%s', got '%s'", tt.wantID, chunk.ID)
			}
			if tt.wantModel != "" && chunk.Model != tt.wantModel {
				t.Errorf("Expected model '%s', got '%s'", tt.wantModel, chunk.Model)
			}

			if tt.wantDelta != "" {
				if len(chunk.Deltas) != 1 {
					t.Fatalf("Expected 1 delta, got %d", len(chunk.Deltas))
				}
				// Check for text content
				if len(chunk.Deltas[0].Delta.Content) > 0 {
					if chunk.Deltas[0].Delta.Content[0].Text != tt.wantDelta {
						t.Errorf("Expected delta content '%s', got '%s'", tt.wantDelta, chunk.Deltas[0].Delta.Content[0].Text)
					}
				}
				// Check for tool call arguments
				if len(chunk.Deltas[0].Delta.ToolCalls) > 0 {
					if chunk.Deltas[0].Delta.ToolCalls[0].Arguments != tt.wantDelta {
						t.Errorf("Expected tool call arguments '%s', got '%s'", tt.wantDelta, chunk.Deltas[0].Delta.ToolCalls[0].Arguments)
					}
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

func TestResponsesProviderAdapter_EndToEnd(t *testing.T) {
	// Create test server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		if r.Method != "POST" {
			t.Errorf("Expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/v1/responses" {
			t.Errorf("Expected path '/v1/responses', got '%s'", r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Expected Content-Type 'application/json', got '%s'", r.Header.Get("Content-Type"))
		}

		// Return mock response
		resp := ResponsesResponse{
			ID:      "resp_test",
			Object:  "response",
			Created: 1700000000,
			Model:   "gpt-4o",
			Status:  "completed",
			Output: []ResponsesOutputItem{
				{
					Type: "message",
					ID:   "msg_1",
					Role: "assistant",
					Content: []ResponsesContent{{
						Type: "output_text",
						Text: "Hello, world!",
					}},
				},
			},
			Usage: &ResponsesUsage{
				InputTokens:  10,
				OutputTokens: 5,
				TotalTokens:  15,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	// Build request
	providerAdapter := NewResponsesProviderAdapter()
	ctx := context.Background()

	req := &canonical.Request{
		Kind:  canonical.KindChat,
		Model: "gpt-4o",
		Messages: []canonical.Message{
			{
				Role: canonical.RoleUser,
				Content: []canonical.ContentBlock{{
					Type: canonical.ContentText,
					Text: "Hello",
				}},
			},
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
	if resp.ID != "resp_test" {
		t.Errorf("Expected ID 'resp_test', got '%s'", resp.ID)
	}
	if len(resp.Choices) != 1 {
		t.Fatalf("Expected 1 choice, got %d", len(resp.Choices))
	}
	if resp.Choices[0].Message.Content[0].Text != "Hello, world!" {
		t.Errorf("Expected content 'Hello, world!', got '%s'", resp.Choices[0].Message.Content[0].Text)
	}
}