package test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/bestruirui/octopus/internal/transformer2/adapter/anthropic"
	"github.com/bestruirui/octopus/internal/transformer2/adapter/openai"
	"github.com/bestruirui/octopus/internal/transformer2/canonical"
)

// TestCanonical_OpenAIChatClient tests the OpenAI Chat client adapter ParseRequest.
func TestCanonical_OpenAIChatClient(t *testing.T) {
	cases, err := LoadAllFixtures()
	if err != nil {
		t.Fatalf("Failed to load fixtures: %v", err)
	}

	adapter := openai.NewChatClientAdapter()
	ctx := context.Background()

	for _, tc := range cases {
		// Skip if not relevant to this adapter
		if tc.InboundFormat != "openai_chat" {
			continue
		}
		if len(tc.ClientRequest) == 0 {
			continue
		}

		t.Run(tc.Name, func(t *testing.T) {
			// Parse client request
			req, err := adapter.ParseRequest(ctx, tc.ClientRequest, nil)
			if err != nil {
				t.Fatalf("ParseRequest failed: %v", err)
			}

			// If expected_internal_request is provided, assert partial match
			if len(tc.ExpectedInternalRequest) > 0 {
				var expected map[string]interface{}
				if err := json.Unmarshal(tc.ExpectedInternalRequest, &expected); err != nil {
					t.Fatalf("Failed to parse expected_internal_request: %v", err)
				}

				// Convert actual request to map for comparison
				actual := canonicalRequestToMap(req)
				if !AssertFieldsMatch(expected, actual) {
					actualJSON, _ := json.MarshalIndent(actual, "", "  ")
					expectedJSON, _ := json.MarshalIndent(expected, "", "  ")
					t.Errorf("Request mismatch:\nGot:\n%s\n\nExpected (partial):\n%s", actualJSON, expectedJSON)
				}
			}
		})
	}
}

// TestCanonical_OpenAIChatProvider tests the OpenAI Chat provider adapter BuildRequest.
func TestCanonical_OpenAIChatProvider(t *testing.T) {
	cases, err := LoadAllFixtures()
	if err != nil {
		t.Fatalf("Failed to load fixtures: %v", err)
	}

	clientAdapter := openai.NewChatClientAdapter()
	providerAdapter := openai.NewChatProviderAdapter()
	ctx := context.Background()

	for _, tc := range cases {
		// Skip if not relevant
		if tc.OutboundFormat != "openai_chat" {
			continue
		}
		if len(tc.ExpectedProviderRequest) == 0 {
			continue
		}

		t.Run(tc.Name, func(t *testing.T) {
			// First, parse client request to get canonical request
			var creq *canonical.Request
			if len(tc.ClientRequest) > 0 {
				var err error
				creq, err = clientAdapter.ParseRequest(ctx, tc.ClientRequest, nil)
				if err != nil {
					// If there's no client request for openai_chat, create from expected_internal_request
					t.Fatalf("ParseRequest failed: %v", err)
				}
			}

			// Build provider request
			httpReq, err := providerAdapter.BuildRequest(ctx, creq, "https://api.openai.com", "test-key")
			if err != nil {
				t.Fatalf("BuildRequest failed: %v", err)
			}

			// Read and decode body
			body, err := io.ReadAll(httpReq.Body)
			if err != nil {
				t.Fatalf("Failed to read body: %v", err)
			}

			var actual map[string]interface{}
			if err := json.Unmarshal(body, &actual); err != nil {
				t.Fatalf("Failed to parse actual request: %v", err)
			}

			// Parse expected
			var expected map[string]interface{}
			if err := json.Unmarshal(tc.ExpectedProviderRequest, &expected); err != nil {
				t.Fatalf("Failed to parse expected_provider_request: %v", err)
			}

			if !AssertFieldsMatch(expected, actual) {
				actualJSON, _ := json.MarshalIndent(actual, "", "  ")
				expectedJSON, _ := json.MarshalIndent(expected, "", "  ")
				t.Errorf("Provider request mismatch:\nGot:\n%s\n\nExpected (partial):\n%s", actualJSON, expectedJSON)
			}
		})
	}
}

// TestCanonical_AnthropicClient tests the Anthropic client adapter ParseRequest.
func TestCanonical_AnthropicClient(t *testing.T) {
	cases, err := LoadAllFixtures()
	if err != nil {
		t.Fatalf("Failed to load fixtures: %v", err)
	}

	adapter := anthropic.NewClientAdapter()
	ctx := context.Background()

	for _, tc := range cases {
		// Skip if not relevant to this adapter
		if tc.InboundFormat != "anthropic" {
			continue
		}
		if len(tc.ClientRequest) == 0 {
			continue
		}

		t.Run(tc.Name, func(t *testing.T) {
			// Parse client request
			req, err := adapter.ParseRequest(ctx, tc.ClientRequest, nil)
			if err != nil {
				t.Fatalf("ParseRequest failed: %v", err)
			}

			// If expected_internal_request is provided, assert partial match
			if len(tc.ExpectedInternalRequest) > 0 {
				var expected map[string]interface{}
				if err := json.Unmarshal(tc.ExpectedInternalRequest, &expected); err != nil {
					t.Fatalf("Failed to parse expected_internal_request: %v", err)
				}

				// Convert actual request to map for comparison
				actual := canonicalRequestToMap(req)
				if !AssertFieldsMatch(expected, actual) {
					actualJSON, _ := json.MarshalIndent(actual, "", "  ")
					expectedJSON, _ := json.MarshalIndent(expected, "", "  ")
					t.Errorf("Request mismatch:\nGot:\n%s\n\nExpected (partial):\n%s", actualJSON, expectedJSON)
				}
			}
		})
	}
}

// TestCanonical_AnthropicProvider tests the Anthropic provider adapter BuildRequest.
func TestCanonical_AnthropicProvider(t *testing.T) {
	cases, err := LoadAllFixtures()
	if err != nil {
		t.Fatalf("Failed to load fixtures: %v", err)
	}

	// Use OpenAI chat client to parse initial requests (for cross-format tests)
	clientAdapter := openai.NewChatClientAdapter()
	providerAdapter := anthropic.NewProviderAdapter()
	ctx := context.Background()

	for _, tc := range cases {
		// Skip if not relevant
		if tc.OutboundFormat != "anthropic" {
			continue
		}
		if len(tc.ExpectedProviderRequest) == 0 {
			continue
		}

		t.Run(tc.Name, func(t *testing.T) {
			// Parse client request to get canonical request
			var creq *canonical.Request
			var err error

			if len(tc.ClientRequest) > 0 {
				// Use appropriate client adapter based on inbound format
				if tc.InboundFormat == "openai_chat" {
					creq, err = clientAdapter.ParseRequest(ctx, tc.ClientRequest, nil)
				} else {
					t.Fatalf("Unsupported inbound format: %s", tc.InboundFormat)
				}
				if err != nil {
					t.Fatalf("ParseRequest failed: %v", err)
				}
			}

			// Build provider request
			httpReq, err := providerAdapter.BuildRequest(ctx, creq, "https://api.anthropic.com", "test-key")
			if err != nil {
				t.Fatalf("BuildRequest failed: %v", err)
			}

			// Read and decode body
			body, err := io.ReadAll(httpReq.Body)
			if err != nil {
				t.Fatalf("Failed to read body: %v", err)
			}

			var actual map[string]interface{}
			if err := json.Unmarshal(body, &actual); err != nil {
				t.Fatalf("Failed to parse actual request: %v", err)
			}

			// Parse expected
			var expected map[string]interface{}
			if err := json.Unmarshal(tc.ExpectedProviderRequest, &expected); err != nil {
				t.Fatalf("Failed to parse expected_provider_request: %v", err)
			}

			if !AssertFieldsMatch(expected, actual) {
				actualJSON, _ := json.MarshalIndent(actual, "", "  ")
				expectedJSON, _ := json.MarshalIndent(expected, "", "  ")
				t.Errorf("Provider request mismatch:\nGot:\n%s\n\nExpected (partial):\n%s", actualJSON, expectedJSON)
			}

			// Verify headers
			if httpReq.Header.Get("Content-Type") != "application/json" {
				t.Errorf("Expected Content-Type header to be application/json, got %s", httpReq.Header.Get("Content-Type"))
			}
			if httpReq.Header.Get("x-api-key") != "test-key" {
				t.Errorf("Expected x-api-key header to be test-key, got %s", httpReq.Header.Get("x-api-key"))
			}
		})
	}
}

// TestCanonical_OpenAIResponsesClient tests the OpenAI Responses API client adapter ParseRequest.
func TestCanonical_OpenAIResponsesClient(t *testing.T) {
	cases, err := LoadAllFixtures()
	if err != nil {
		t.Fatalf("Failed to load fixtures: %v", err)
	}

	adapter := openai.NewResponsesClientAdapter()
	ctx := context.Background()

	for _, tc := range cases {
		// Skip if not relevant to this adapter
		if tc.InboundFormat != "openai_responses" {
			continue
		}
		if len(tc.ClientRequest) == 0 {
			continue
		}

		t.Run(tc.Name, func(t *testing.T) {
			// Parse client request
			req, err := adapter.ParseRequest(ctx, tc.ClientRequest, nil)
			if err != nil {
				t.Fatalf("ParseRequest failed: %v", err)
			}

			// If expected_internal_request is provided, assert partial match
			if len(tc.ExpectedInternalRequest) > 0 {
				var expected map[string]interface{}
				if err := json.Unmarshal(tc.ExpectedInternalRequest, &expected); err != nil {
					t.Fatalf("Failed to parse expected_internal_request: %v", err)
				}

				// Convert actual request to map for comparison
				actual := canonicalRequestToMap(req)
				if !AssertFieldsMatch(expected, actual) {
					actualJSON, _ := json.MarshalIndent(actual, "", "  ")
					expectedJSON, _ := json.MarshalIndent(expected, "", "  ")
					t.Errorf("Request mismatch:\nGot:\n%s\n\nExpected (partial):\n%s", actualJSON, expectedJSON)
				}
			}
		})
	}
}

// TestCanonical_OpenAIResponsesProvider tests the OpenAI Responses API provider adapter BuildRequest.
func TestCanonical_OpenAIResponsesProvider(t *testing.T) {
	cases, err := LoadAllFixtures()
	if err != nil {
		t.Fatalf("Failed to load fixtures: %v", err)
	}

	clientAdapter := openai.NewResponsesClientAdapter()
	providerAdapter := openai.NewResponsesProviderAdapter()
	ctx := context.Background()

	for _, tc := range cases {
		// Skip if not relevant
		if tc.OutboundFormat != "openai_responses" {
			continue
		}
		if len(tc.ExpectedProviderRequest) == 0 {
			continue
		}

		t.Run(tc.Name, func(t *testing.T) {
			// First, parse client request to get canonical request
			var creq *canonical.Request
			if len(tc.ClientRequest) > 0 {
				var err error
				creq, err = clientAdapter.ParseRequest(ctx, tc.ClientRequest, nil)
				if err != nil {
					t.Fatalf("ParseRequest failed: %v", err)
				}
			}

			// Build provider request
			httpReq, err := providerAdapter.BuildRequest(ctx, creq, "https://api.openai.com", "test-key")
			if err != nil {
				t.Fatalf("BuildRequest failed: %v", err)
			}

			// Verify URL
			if !strings.HasSuffix(httpReq.URL.Path, "/v1/responses") {
				t.Errorf("Expected URL path to end with /v1/responses, got %s", httpReq.URL.Path)
			}

			// Read and decode body
			body, err := io.ReadAll(httpReq.Body)
			if err != nil {
				t.Fatalf("Failed to read body: %v", err)
			}

			var actual map[string]interface{}
			if err := json.Unmarshal(body, &actual); err != nil {
				t.Fatalf("Failed to parse actual request: %v", err)
			}

			// Parse expected
			var expected map[string]interface{}
			if err := json.Unmarshal(tc.ExpectedProviderRequest, &expected); err != nil {
				t.Fatalf("Failed to parse expected_provider_request: %v", err)
			}

			if !AssertFieldsMatch(expected, actual) {
				actualJSON, _ := json.MarshalIndent(actual, "", "  ")
				expectedJSON, _ := json.MarshalIndent(expected, "", "  ")
				t.Errorf("Provider request mismatch:\nGot:\n%s\n\nExpected (partial):\n%s", actualJSON, expectedJSON)
			}
		})
	}
}

// TestCanonical_CrossFormat tests cross-format routing from 07_cross_format_routing.json.
func TestCanonical_CrossFormat(t *testing.T) {
	cases, err := LoadFixtures("07_cross_format_routing.json")
	if err != nil {
		t.Fatalf("Failed to load cross-format fixtures: %v", err)
	}

	ctx := context.Background()
	openaiChatClient := openai.NewChatClientAdapter()
	openaiChatProvider := openai.NewChatProviderAdapter()
	anthropicClient := anthropic.NewClientAdapter()
	anthropicProvider := anthropic.NewProviderAdapter()

	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			// Determine client adapter based on inbound format
			var creq *canonical.Request
			var err error

			switch tc.InboundFormat {
			case "openai_chat":
				creq, err = openaiChatClient.ParseRequest(ctx, tc.ClientRequest, nil)
			case "anthropic":
				creq, err = anthropicClient.ParseRequest(ctx, tc.ClientRequest, nil)
			case "openai_responses":
				adapter := openai.NewResponsesClientAdapter()
				creq, err = adapter.ParseRequest(ctx, tc.ClientRequest, nil)
			default:
				t.Skipf("Unsupported inbound format: %s", tc.InboundFormat)
				return
			}
			if err != nil {
				t.Fatalf("Client ParseRequest failed: %v", err)
			}

			// Check expected_internal_request if provided
			if len(tc.ExpectedInternalRequest) > 0 {
				var expected map[string]interface{}
				if err := json.Unmarshal(tc.ExpectedInternalRequest, &expected); err != nil {
					t.Fatalf("Failed to parse expected_internal_request: %v", err)
				}
				actual := canonicalRequestToMap(creq)
				if !AssertFieldsMatch(expected, actual) {
					actualJSON, _ := json.MarshalIndent(actual, "", "  ")
					expectedJSON, _ := json.MarshalIndent(expected, "", "  ")
					t.Errorf("Internal request mismatch:\nGot:\n%s\n\nExpected (partial):\n%s", actualJSON, expectedJSON)
				}
			}

			// Build provider request and check against expected_provider_request
			if len(tc.ExpectedProviderRequest) > 0 {
				var httpReq *http.Request
				var err error

				switch tc.OutboundFormat {
				case "openai_chat":
					httpReq, err = openaiChatProvider.BuildRequest(ctx, creq, "https://api.openai.com", "test-key")
				case "anthropic":
					httpReq, err = anthropicProvider.BuildRequest(ctx, creq, "https://api.anthropic.com", "test-key")
				default:
					t.Skipf("Unsupported outbound format: %s", tc.OutboundFormat)
					return
				}
				if err != nil {
					t.Fatalf("Provider BuildRequest failed: %v", err)
				}

				body, err := io.ReadAll(httpReq.Body)
				if err != nil {
					t.Fatalf("Failed to read body: %v", err)
				}

				var actual map[string]interface{}
				if err := json.Unmarshal(body, &actual); err != nil {
					t.Fatalf("Failed to parse actual request: %v", err)
				}

				var expected map[string]interface{}
				if err := json.Unmarshal(tc.ExpectedProviderRequest, &expected); err != nil {
					t.Fatalf("Failed to parse expected_provider_request: %v", err)
				}

				if !AssertFieldsMatch(expected, actual) {
					actualJSON, _ := json.MarshalIndent(actual, "", "  ")
					expectedJSON, _ := json.MarshalIndent(expected, "", "  ")
					t.Errorf("Provider request mismatch:\nGot:\n%s\n\nExpected (partial):\n%s", actualJSON, expectedJSON)
				}

				// Check URL expectations if specified
				if baseURL, ok := getBaseURL(tc); ok {
					if !strings.Contains(httpReq.URL.String(), baseURL) {
						t.Errorf("Expected URL to contain %s, got %s", baseURL, httpReq.URL.String())
					}
				}
			}
		})
	}
}

// TestCanonical_OpenAIChatProviderResponse tests provider ParseResponse for OpenAI Chat.
func TestCanonical_OpenAIChatProviderResponse(t *testing.T) {
	cases, err := LoadAllFixtures()
	if err != nil {
		t.Fatalf("Failed to load fixtures: %v", err)
	}

	providerAdapter := openai.NewChatProviderAdapter()
	ctx := context.Background()

	for _, tc := range cases {
		if len(tc.ProviderResponse) == 0 {
			continue
		}
		if tc.OutboundFormat != "openai_chat" && tc.InboundFormat != "openai_chat" {
			continue
		}

		t.Run(tc.Name, func(t *testing.T) {
			// Parse provider response
			resp := &http.Response{
				StatusCode: tc.ProviderStatusCode,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(string(tc.ProviderResponse))),
			}
			resp.Header.Set("Content-Type", "application/json")

			creq, err := providerAdapter.ParseResponse(ctx, resp)
			if err != nil {
				t.Fatalf("ParseResponse failed: %v", err)
			}

			// Check expected_internal_response if provided
			if len(tc.ExpectedInternalResponse) > 0 {
				var expected map[string]interface{}
				if err := json.Unmarshal(tc.ExpectedInternalResponse, &expected); err != nil {
					t.Fatalf("Failed to parse expected_internal_response: %v", err)
				}
				actual := canonicalResponseToMap(creq)
				if !AssertFieldsMatch(expected, actual) {
					actualJSON, _ := json.MarshalIndent(actual, "", "  ")
					expectedJSON, _ := json.MarshalIndent(expected, "", "  ")
					t.Errorf("Internal response mismatch:\nGot:\n%s\n\nExpected (partial):\n%s", actualJSON, expectedJSON)
				}
			}
		})
	}
}

// TestCanonical_AnthropicProviderResponse tests provider ParseResponse for Anthropic.
func TestCanonical_AnthropicProviderResponse(t *testing.T) {
	cases, err := LoadAllFixtures()
	if err != nil {
		t.Fatalf("Failed to load fixtures: %v", err)
	}

	providerAdapter := anthropic.NewProviderAdapter()
	ctx := context.Background()

	for _, tc := range cases {
		if len(tc.ProviderResponse) == 0 {
			continue
		}
		if tc.InboundFormat != "anthropic" {
			continue
		}

		t.Run(tc.Name, func(t *testing.T) {
			// Parse provider response
			resp := &http.Response{
				StatusCode: tc.ProviderStatusCode,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(string(tc.ProviderResponse))),
			}
			resp.Header.Set("Content-Type", "application/json")

			creq, err := providerAdapter.ParseResponse(ctx, resp)
			if err != nil {
				t.Fatalf("ParseResponse failed: %v", err)
			}

			// Check expected_internal_response if provided
			if len(tc.ExpectedInternalResponse) > 0 {
				var expected map[string]interface{}
				if err := json.Unmarshal(tc.ExpectedInternalResponse, &expected); err != nil {
					t.Fatalf("Failed to parse expected_internal_response: %v", err)
				}
				actual := canonicalResponseToMap(creq)
				if !AssertFieldsMatch(expected, actual) {
					actualJSON, _ := json.MarshalIndent(actual, "", "  ")
					expectedJSON, _ := json.MarshalIndent(expected, "", "  ")
					t.Errorf("Internal response mismatch:\nGot:\n%s\n\nExpected (partial):\n%s", actualJSON, expectedJSON)
				}
			}
		})
	}
}

// Helper functions to convert canonical types to maps for comparison.

func canonicalRequestToMap(req *canonical.Request) map[string]interface{} {
	result := make(map[string]interface{})

	if req.Model != "" {
		result["model"] = req.Model
	}
	if req.Temperature != nil {
		result["temperature"] = *req.Temperature
	}
	if req.TopP != nil {
		result["top_p"] = *req.TopP
	}
	if req.TopK != nil {
		result["top_k"] = *req.TopK
	}
	if req.MaxTokens != nil {
		result["max_tokens"] = *req.MaxTokens
	}
	if req.MaxCompletionTokens != nil {
		result["max_completion_tokens"] = *req.MaxCompletionTokens
	}
	if req.FrequencyPenalty != nil {
		result["frequency_penalty"] = *req.FrequencyPenalty
	}
	if req.PresencePenalty != nil {
		result["presence_penalty"] = *req.PresencePenalty
	}
	if req.Seed != nil {
		result["seed"] = *req.Seed
	}
	if req.User != nil {
		result["user"] = *req.User
	}
	if req.Stream {
		result["stream"] = true
	}
	if req.Logprobs != nil {
		result["logprobs"] = *req.Logprobs
	}
	if req.TopLogprobs != nil {
		result["top_logprobs"] = *req.TopLogprobs
	}
	if req.Stop != nil {
		if req.Stop.Single != "" {
			result["stop"] = req.Stop.Single
		} else if len(req.Stop.Multiple) > 0 {
			result["stop"] = req.Stop.Multiple
		}
	}
	if len(req.Messages) > 0 {
		result["messages"] = messagesToSlice(req.Messages)
	}
	if len(req.Tools) > 0 {
		result["tools"] = toolsToSlice(req.Tools)
	}
	if req.ToolChoice != nil {
		result["tool_choice"] = toolChoiceToMap(req.ToolChoice)
	}
	if req.ResponseFormat != nil {
		result["response_format"] = responseFormatToMap(req.ResponseFormat)
	}
	if req.Reasoning != nil && req.Reasoning.Effort != nil {
		result["reasoning_effort"] = *req.Reasoning.Effort
	}
	if req.Hints.AnthropicSystemArrayFormat {
		if result["transformer_metadata"] == nil {
			result["transformer_metadata"] = map[string]interface{}{
				"anthropic_system_array_format": "true",
			}
		}
	}

	return result
}

func messagesToSlice(messages []canonical.Message) []interface{} {
	result := make([]interface{}, len(messages))
	for i, msg := range messages {
		m := map[string]interface{}{
			"role": string(msg.Role),
		}
		if len(msg.Content) > 0 {
			// Check if single text content
			if len(msg.Content) == 1 && msg.Content[0].Type == canonical.ContentText {
				m["content"] = msg.Content[0].Text
			} else {
				m["content"] = contentBlocksToSlice(msg.Content)
			}
		}
		if msg.Name != nil {
			m["name"] = *msg.Name
		}
		if len(msg.ToolCalls) > 0 {
			m["tool_calls"] = toolCallsToSlice(msg.ToolCalls)
		}
		if msg.ToolCallID != nil {
			m["tool_call_id"] = *msg.ToolCallID
		}
		result[i] = m
	}
	return result
}

func contentBlocksToSlice(blocks []canonical.ContentBlock) []interface{} {
	result := make([]interface{}, len(blocks))
	for i, block := range blocks {
		b := map[string]interface{}{
			"type": string(block.Type),
		}
		if block.Type == canonical.ContentText {
			b["text"] = block.Text
		} else if block.Type == canonical.ContentImage && block.Media != nil {
			img := map[string]interface{}{}
			if block.Media.URL != "" {
				img["url"] = block.Media.URL
			}
			if block.Media.Base64 != "" {
				img["data"] = block.Media.Base64
			}
			if block.Media.MimeType != "" {
				img["media_type"] = block.Media.MimeType
			}
			b["source"] = map[string]interface{}{
				"type":       "base64",
				"media_type": block.Media.MimeType,
				"data":       block.Media.Base64,
			}
			if block.Media.URL != "" {
				b["source"] = map[string]interface{}{
					"type": "url",
					"url":  block.Media.URL,
				}
			}
			_ = img // silence unused variable warning
		}
		result[i] = b
	}
	return result
}

func toolCallsToSlice(calls []canonical.ToolCall) []interface{} {
	result := make([]interface{}, len(calls))
	for i, tc := range calls {
		result[i] = map[string]interface{}{
			"id":   tc.ID,
			"type": tc.Type,
			"function": map[string]interface{}{
				"name":      tc.Name,
				"arguments": tc.Arguments,
			},
		}
	}
	return result
}

func toolsToSlice(tools []canonical.Tool) []interface{} {
	result := make([]interface{}, len(tools))
	for i, tool := range tools {
		t := map[string]interface{}{
			"type": tool.Type,
		}
		if tool.Name != "" {
			t["name"] = tool.Name
		}
		if tool.Description != "" {
			t["description"] = tool.Description
		}
		if tool.Parameters != nil {
			t["parameters"] = json.RawMessage(tool.Parameters)
		}
		result[i] = t
	}
	return result
}

func toolChoiceToMap(tc *canonical.ToolChoice) interface{} {
	if tc == nil {
		return nil
	}
	m := map[string]interface{}{}
	if tc.Mode != "" {
		m["type"] = tc.Mode
	}
	if tc.Function != nil {
		m["function"] = map[string]interface{}{
			"name": *tc.Function,
		}
	}
	return m
}

func responseFormatToMap(rf *canonical.ResponseFormat) interface{} {
	m := map[string]interface{}{
		"type": rf.Type,
	}
	if rf.JsonSchema != nil {
		m["json_schema"] = map[string]interface{}{
			"name":   rf.JsonSchema.Name,
			"strict": rf.JsonSchema.Strict,
		}
		if rf.JsonSchema.Schema != nil {
			m["json_schema"].(map[string]interface{})["schema"] = rf.JsonSchema.Schema
		}
	}
	return m
}

func canonicalResponseToMap(resp *canonical.Response) map[string]interface{} {
	result := make(map[string]interface{})

	if resp.ID != "" {
		result["id"] = resp.ID
	}
	if resp.Model != "" {
		result["model"] = resp.Model
	}
	if resp.Object != "" {
		result["object"] = resp.Object
	}

	if len(resp.Choices) > 0 {
		choices := make([]interface{}, len(resp.Choices))
		for i, choice := range resp.Choices {
			c := map[string]interface{}{
				"index": choice.Index,
			}
			c["message"] = messageToMap(choice.Message)
			if choice.FinishReason != nil {
				c["finish_reason"] = *choice.FinishReason
			}
			choices[i] = c
		}
		result["choices"] = choices
	}

	if resp.Usage != nil {
		result["usage"] = map[string]interface{}{
			"prompt_tokens":     resp.Usage.PromptTokens,
			"completion_tokens": resp.Usage.CompletionTokens,
			"total_tokens":      resp.Usage.TotalTokens,
		}
	}

	return result
}

func messageToMap(msg canonical.Message) map[string]interface{} {
	m := map[string]interface{}{
		"role": string(msg.Role),
	}
	if len(msg.Content) > 0 {
		if len(msg.Content) == 1 && msg.Content[0].Type == canonical.ContentText {
			m["content"] = msg.Content[0].Text
		} else {
			contents := make([]interface{}, len(msg.Content))
			for i, c := range msg.Content {
				cm := map[string]interface{}{"type": string(c.Type)}
				if c.Type == canonical.ContentText {
					cm["text"] = c.Text
				}
				contents[i] = cm
			}
			m["content"] = contents
		}
	}
	if len(msg.ToolCalls) > 0 {
		m["tool_calls"] = toolCallsToSlice(msg.ToolCalls)
	}
	return m
}

// getBaseURL extracts base_url from a test case.
// This is a placeholder - in a real implementation, you might need to
// parse additional fields from the TestCase struct.
func getBaseURL(tc TestCase) (string, bool) {
	// Check if there's a base_url field in the raw test case
	// Since TestCase doesn't have this field, we'd need to extend it
	// For now, return false
	return "", false
}