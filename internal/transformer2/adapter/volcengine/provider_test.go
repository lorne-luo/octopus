package volcengine

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

func TestVolcengineBuildRequest_Basic(t *testing.T) {
	adapter := NewProviderAdapter()
	ctx := context.Background()

	req := &canonical.Request{
		Kind:  canonical.KindChat,
		Model: "doubao-pro-32k",
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

	httpReq, err := adapter.BuildRequest(ctx, req, "https://api.volcengine.com", "test-key")
	if err != nil {
		t.Fatalf("BuildRequest failed: %v", err)
	}

	// Verify HTTP method and URL
	if httpReq.Method != "POST" {
		t.Errorf("Expected POST method, got %s", httpReq.Method)
	}
	if httpReq.URL.String() != "https://api.volcengine.com/v1/responses" {
		t.Errorf("Expected URL 'https://api.volcengine.com/v1/responses', got %s", httpReq.URL.String())
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

	var respReq map[string]interface{}
	if err := json.Unmarshal(body, &respReq); err != nil {
		t.Fatalf("Failed to unmarshal request body: %v", err)
	}

	if respReq["model"] != "doubao-pro-32k" {
		t.Errorf("Expected model 'doubao-pro-32k', got '%s'", respReq["model"])
	}

	// Verify metadata is not present (Volcengine does not support it)
	if _, ok := respReq["metadata"]; ok {
		t.Errorf("Expected metadata to be stripped, but it was present")
	}
}

func TestVolcengineBuildRequest_ThinkingConfig(t *testing.T) {
	tests := []struct {
		name              string
		reasoningEffort   string
		expectedThinking  string
	}{
		{
			name:             "minimal effort maps to disabled",
			reasoningEffort:  "minimal",
			expectedThinking: "disabled",
		},
		{
			name:             "low effort maps to enabled",
			reasoningEffort:  "low",
			expectedThinking: "enabled",
		},
		{
			name:             "medium effort maps to enabled",
			reasoningEffort:  "medium",
			expectedThinking: "enabled",
		},
		{
			name:             "high effort maps to enabled",
			reasoningEffort:  "high",
			expectedThinking: "enabled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter := NewProviderAdapter()
			ctx := context.Background()

			req := &canonical.Request{
				Kind:  canonical.KindChat,
				Model: "doubao-seed-1-8-251228", // Use a supported reasoning model
				Messages: []canonical.Message{
					{
						Role: canonical.RoleUser,
						Content: []canonical.ContentBlock{{
							Type: canonical.ContentText,
							Text: "Hello",
						}},
					},
				},
				Reasoning: &canonical.ReasoningConfig{
					Effort: &tt.reasoningEffort,
				},
			}

			httpReq, err := adapter.BuildRequest(ctx, req, "https://api.volcengine.com", "test-key")
			if err != nil {
				t.Fatalf("BuildRequest failed: %v", err)
			}

			body, err := io.ReadAll(httpReq.Body)
			if err != nil {
				t.Fatalf("Failed to read request body: %v", err)
			}

			var respReq map[string]interface{}
			if err := json.Unmarshal(body, &respReq); err != nil {
				t.Fatalf("Failed to unmarshal request body: %v", err)
			}

			thinking, ok := respReq["thinking"].(map[string]interface{})
			if !ok {
				t.Fatalf("Expected thinking object, got %v", respReq["thinking"])
			}

			if thinking["type"] != tt.expectedThinking {
				t.Errorf("Expected thinking type '%s', got '%s'", tt.expectedThinking, thinking["type"])
			}
		})
	}
}

func TestVolcengineBuildRequest_MetadataStripping(t *testing.T) {
	adapter := NewProviderAdapter()
	ctx := context.Background()

	metadata := map[string]string{
		"custom_field": "custom_value",
		"request_id":   "12345",
	}

	req := &canonical.Request{
		Kind:  canonical.KindChat,
		Model: "doubao-pro-32k",
		Messages: []canonical.Message{
			{
				Role: canonical.RoleUser,
				Content: []canonical.ContentBlock{{
					Type: canonical.ContentText,
					Text: "Hello",
				}},
			},
		},
		Metadata: metadata,
	}

	httpReq, err := adapter.BuildRequest(ctx, req, "https://api.volcengine.com", "test-key")
	if err != nil {
		t.Fatalf("BuildRequest failed: %v", err)
	}

	body, err := io.ReadAll(httpReq.Body)
	if err != nil {
		t.Fatalf("Failed to read request body: %v", err)
	}

	var respReq map[string]interface{}
	if err := json.Unmarshal(body, &respReq); err != nil {
		t.Fatalf("Failed to unmarshal request body: %v", err)
	}

	// Verify metadata field is not present
	if _, ok := respReq["metadata"]; ok {
		t.Errorf("Expected metadata to be stripped, but it was present in request")
	}
}

func TestVolcengineBuildRequest_ModelFiltering(t *testing.T) {
	tests := []struct {
		name                string
		model               string
		reasoningEffort     string
		expectReasoning     bool
		expectThinkingField bool
	}{
		{
			name:                "supported reasoning model with effort",
			model:               "doubao-seed-1-8-251228",
			reasoningEffort:     "high",
			expectReasoning:     true,
			expectThinkingField: true,
		},
		{
			name:                "supported reasoning model doubao-seed-1-6-lite",
			model:               "doubao-seed-1-6-lite-251015",
			reasoningEffort:     "medium",
			expectReasoning:     true,
			expectThinkingField: true,
		},
		{
			name:                "supported reasoning model doubao-seed-1-6",
			model:               "doubao-seed-1-6-251015",
			reasoningEffort:     "low",
			expectReasoning:     true,
			expectThinkingField: true,
		},
		{
			name:                "unsupported reasoning model - reasoning stripped",
			model:               "doubao-pro-32k",
			reasoningEffort:     "high",
			expectReasoning:     false,
			expectThinkingField: false,
		},
		{
			name:                "no reasoning effort specified",
			model:               "doubao-seed-1-8-251228",
			reasoningEffort:     "",
			expectReasoning:     false,
			expectThinkingField: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter := NewProviderAdapter()
			ctx := context.Background()

			req := &canonical.Request{
				Kind:  canonical.KindChat,
				Model: tt.model,
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

			if tt.reasoningEffort != "" {
				req.Reasoning = &canonical.ReasoningConfig{
					Effort: &tt.reasoningEffort,
				}
			}

			httpReq, err := adapter.BuildRequest(ctx, req, "https://api.volcengine.com", "test-key")
			if err != nil {
				t.Fatalf("BuildRequest failed: %v", err)
			}

			body, err := io.ReadAll(httpReq.Body)
			if err != nil {
				t.Fatalf("Failed to read request body: %v", err)
			}

			var respReq map[string]interface{}
			if err := json.Unmarshal(body, &respReq); err != nil {
				t.Fatalf("Failed to unmarshal request body: %v", err)
			}

			_, hasReasoning := respReq["reasoning"]
			_, hasThinking := respReq["thinking"]

			if tt.expectReasoning && !hasReasoning {
				t.Errorf("Expected reasoning field to be present for model %s", tt.model)
			}
			if !tt.expectReasoning && hasReasoning {
				t.Errorf("Expected reasoning field to be stripped for model %s", tt.model)
			}
			if tt.expectThinkingField && !hasThinking {
				t.Errorf("Expected thinking field to be present for model %s", tt.model)
			}
			if !tt.expectThinkingField && hasThinking {
				t.Errorf("Expected thinking field to be absent for model %s", tt.model)
			}
		})
	}
}

func TestVolcengineBuildRequest_PartialAssistantMessage(t *testing.T) {
	adapter := NewProviderAdapter()
	ctx := context.Background()

	req := &canonical.Request{
		Kind:  canonical.KindChat,
		Model: "doubao-pro-32k",
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
		},
	}

	httpReq, err := adapter.BuildRequest(ctx, req, "https://api.volcengine.com", "test-key")
	if err != nil {
		t.Fatalf("BuildRequest failed: %v", err)
	}

	body, err := io.ReadAll(httpReq.Body)
	if err != nil {
		t.Fatalf("Failed to read request body: %v", err)
	}

	var respReq map[string]interface{}
	if err := json.Unmarshal(body, &respReq); err != nil {
		t.Fatalf("Failed to unmarshal request body: %v", err)
	}

	// Verify input is an array
	input, ok := respReq["input"].([]interface{})
	if !ok {
		t.Fatalf("Expected input to be an array, got %v", respReq["input"])
	}

	if len(input) != 2 {
		t.Fatalf("Expected 2 input items, got %d", len(input))
	}

	// Verify last assistant message has partial: true
	lastItem, ok := input[1].(map[string]interface{})
	if !ok {
		t.Fatalf("Expected last item to be an object, got %v", input[1])
	}

	if lastItem["role"] != "assistant" {
		t.Errorf("Expected last item role to be 'assistant', got '%s'", lastItem["role"])
	}

	partial, ok := lastItem["partial"].(bool)
	if !ok || !partial {
		t.Errorf("Expected last assistant message to have partial: true, got %v", lastItem["partial"])
	}
}

func TestVolcengineParseResponse_Success(t *testing.T) {
	adapter := NewProviderAdapter()
	ctx := context.Background()

	// Use OpenAI Responses API format for response
	providerResp := map[string]interface{}{
		"id":      "resp_abc123",
		"object":  "response",
		"created": float64(1700000000),
		"model":   "doubao-pro-32k",
		"status":  "completed",
		"output": []interface{}{
			map[string]interface{}{
				"type": "message",
				"id":   "msg_1",
				"role": "assistant",
				"content": []interface{}{
					map[string]interface{}{
						"type": "output_text",
						"text": "Hello! How can I help you?",
					},
				},
			},
		},
		"usage": map[string]interface{}{
			"input_tokens":  float64(10),
			"output_tokens": float64(8),
			"total_tokens":  float64(18),
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
	if resp.Model != "doubao-pro-32k" {
		t.Errorf("Expected model 'doubao-pro-32k', got '%s'", resp.Model)
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
}

func TestVolcengineParseStreamChunk(t *testing.T) {
	adapter := NewProviderAdapter()
	ctx := context.Background()

	tests := []struct {
		name      string
		input     string
		wantDone  bool
		wantID    string
		wantDelta string
	}{
		{
			name:   "done_marker",
			input:  "[DONE]",
			wantDone: true,
		},
		{
			name:      "output_text_delta",
			input:     `{"type":"response.output_text.delta","delta":"Hello","output_index":0}`,
			wantDelta: "Hello",
		},
		{
			name:    "response_created",
			input:   `{"type":"response.created","response":{"id":"resp_123","model":"doubao-pro-32k","created":1700000000}}`,
			wantID:  "resp_123",
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

			if tt.wantDelta != "" {
				if len(chunk.Deltas) != 1 {
					t.Fatalf("Expected 1 delta, got %d", len(chunk.Deltas))
				}
				if len(chunk.Deltas[0].Delta.Content) > 0 {
					if chunk.Deltas[0].Delta.Content[0].Text != tt.wantDelta {
						t.Errorf("Expected delta content '%s', got '%s'", tt.wantDelta, chunk.Deltas[0].Delta.Content[0].Text)
					}
				}
			}
		})
	}
}

func TestVolcengineProviderAdapter_EndToEnd(t *testing.T) {
	// Create test server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		if r.Method != "POST" {
			t.Errorf("Expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/v1/responses" {
			t.Errorf("Expected path '/v1/responses', got '%s'", r.URL.Path)
		}

		// Return mock response
		resp := map[string]interface{}{
			"id":      "resp_test",
			"object":  "response",
			"created": float64(1700000000),
			"model":   "doubao-pro-32k",
			"status":  "completed",
			"output": []interface{}{
				map[string]interface{}{
					"type": "message",
					"id":   "msg_1",
					"role": "assistant",
					"content": []interface{}{
						map[string]interface{}{
							"type": "output_text",
							"text": "Hello, world!",
						},
					},
				},
			},
			"usage": map[string]interface{}{
				"input_tokens":  float64(10),
				"output_tokens": float64(5),
				"total_tokens":  float64(15),
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	// Build request
	providerAdapter := NewProviderAdapter()
	ctx := context.Background()

	req := &canonical.Request{
		Kind:  canonical.KindChat,
		Model: "doubao-pro-32k",
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