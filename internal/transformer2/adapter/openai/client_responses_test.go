package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/bestruirui/octopus/internal/transformer2/canonical"
)

func TestResponsesParseRequest_StringInput(t *testing.T) {
	adapter := NewResponsesClientAdapter()
	ctx := context.Background()

	body := `{
		"model": "gpt-4o",
		"input": "Hello, world!"
	}`

	req, err := adapter.ParseRequest(ctx, []byte(body), http.Header{})
	if err != nil {
		t.Fatalf("ParseRequest failed: %v", err)
	}

	// Verify basic fields
	if req.Model != "gpt-4o" {
		t.Errorf("Expected model 'gpt-4o', got '%s'", req.Model)
	}
	if req.Kind != canonical.KindChat {
		t.Errorf("Expected KindChat, got %d", req.Kind)
	}
	if req.SourceFormat != canonical.FormatOpenAIResponse {
		t.Errorf("Expected FormatOpenAIResponse, got '%s'", req.SourceFormat)
	}

	// Verify messages - string input becomes single user message
	if len(req.Messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(req.Messages))
	}
	if req.Messages[0].Role != canonical.RoleUser {
		t.Errorf("Expected role 'user', got '%s'", req.Messages[0].Role)
	}
	if len(req.Messages[0].Content) != 1 || req.Messages[0].Content[0].Text != "Hello, world!" {
		t.Errorf("Expected content 'Hello, world!', got %+v", req.Messages[0].Content)
	}

	// Verify hints - string input should NOT set ResponsesArrayInput
	if req.Hints.ResponsesArrayInput != nil && *req.Hints.ResponsesArrayInput {
		t.Error("Expected ResponsesArrayInput to be false/nil for string input")
	}
}

func TestResponsesParseRequest_ArrayInput(t *testing.T) {
	adapter := NewResponsesClientAdapter()
	ctx := context.Background()

	body := `{
		"model": "gpt-4o",
		"input": [
			{
				"type": "message",
				"role": "user",
				"content": [{"type": "input_text", "text": "Hello"}]
			},
			{
				"type": "message",
				"role": "assistant",
				"content": [{"type": "output_text", "text": "Hi there!"}]
			},
			{
				"type": "message",
				"role": "user",
				"content": [{"type": "input_text", "text": "How are you?"}]
			}
		]
	}`

	req, err := adapter.ParseRequest(ctx, []byte(body), http.Header{})
	if err != nil {
		t.Fatalf("ParseRequest failed: %v", err)
	}

	// Verify messages
	if len(req.Messages) != 3 {
		t.Fatalf("Expected 3 messages, got %d", len(req.Messages))
	}

	// Verify roles
	expectedRoles := []canonical.Role{canonical.RoleUser, canonical.RoleAssistant, canonical.RoleUser}
	for i, expected := range expectedRoles {
		if req.Messages[i].Role != expected {
			t.Errorf("Message %d: expected role '%s', got '%s'", i, expected, req.Messages[i].Role)
		}
	}

	// Verify ResponsesArrayInput hint is set
	if req.Hints.ResponsesArrayInput == nil || !*req.Hints.ResponsesArrayInput {
		t.Error("Expected ResponsesArrayInput hint to be true for array input")
	}
}

func TestResponsesParseRequest_WithInstructions(t *testing.T) {
	adapter := NewResponsesClientAdapter()
	ctx := context.Background()

	body := `{
		"model": "gpt-4o",
		"instructions": "You are a helpful assistant.",
		"input": "Hello"
	}`

	req, err := adapter.ParseRequest(ctx, []byte(body), http.Header{})
	if err != nil {
		t.Fatalf("ParseRequest failed: %v", err)
	}

	// Verify instructions become system message
	if len(req.Messages) != 2 {
		t.Fatalf("Expected 2 messages (system + user), got %d", len(req.Messages))
	}
	if req.Messages[0].Role != canonical.RoleSystem {
		t.Errorf("Expected first message role 'system', got '%s'", req.Messages[0].Role)
	}
	if req.Messages[0].Content[0].Text != "You are a helpful assistant." {
		t.Errorf("Expected system message 'You are a helpful assistant.', got '%s'", req.Messages[0].Content[0].Text)
	}
}

func TestResponsesParseRequest_ImageGenerationTool(t *testing.T) {
	adapter := NewResponsesClientAdapter()
	ctx := context.Background()

	body := `{
		"model": "gpt-4o",
		"input": "Generate an image",
		"tools": [
			{
				"type": "image_generation",
				"background": "opaque",
				"output_format": "png",
				"quality": "hd",
				"size": "1024x1024"
			}
		]
	}`

	req, err := adapter.ParseRequest(ctx, []byte(body), http.Header{})
	if err != nil {
		t.Fatalf("ParseRequest failed: %v", err)
	}

	// Verify tools
	if len(req.Tools) != 1 {
		t.Fatalf("Expected 1 tool, got %d", len(req.Tools))
	}
	if req.Tools[0].Type != "image_generation" {
		t.Errorf("Expected tool type 'image_generation', got '%s'", req.Tools[0].Type)
	}
	if req.Tools[0].ImageGeneration == nil {
		t.Fatal("Expected ImageGeneration config, got nil")
	}
	if req.Tools[0].ImageGeneration.Background != "opaque" {
		t.Errorf("Expected background 'opaque', got '%s'", req.Tools[0].ImageGeneration.Background)
	}
	if req.Tools[0].ImageGeneration.OutputFormat != "png" {
		t.Errorf("Expected output_format 'png', got '%s'", req.Tools[0].ImageGeneration.OutputFormat)
	}
}

func TestResponsesParseRequest_FunctionTool(t *testing.T) {
	adapter := NewResponsesClientAdapter()
	ctx := context.Background()

	body := `{
		"model": "gpt-4o",
		"input": "What's the weather?",
		"tools": [
			{
				"type": "function",
				"name": "get_weather",
				"description": "Get weather for a location",
				"parameters": {"type": "object", "properties": {"location": {"type": "string"}}}
			}
		],
		"tool_choice": {"type": "function", "name": "get_weather"}
	}`

	req, err := adapter.ParseRequest(ctx, []byte(body), http.Header{})
	if err != nil {
		t.Fatalf("ParseRequest failed: %v", err)
	}

	// Verify function tool
	if len(req.Tools) != 1 {
		t.Fatalf("Expected 1 tool, got %d", len(req.Tools))
	}
	if req.Tools[0].Type != "function" {
		t.Errorf("Expected tool type 'function', got '%s'", req.Tools[0].Type)
	}
	if req.Tools[0].Name != "get_weather" {
		t.Errorf("Expected tool name 'get_weather', got '%s'", req.Tools[0].Name)
	}

	// Verify tool_choice
	if req.ToolChoice == nil {
		t.Fatal("Expected tool_choice, got nil")
	}
	if req.ToolChoice.Mode != "tool" {
		t.Errorf("Expected tool_choice mode 'tool', got '%s'", req.ToolChoice.Mode)
	}
	if req.ToolChoice.Function == nil || *req.ToolChoice.Function != "get_weather" {
		t.Errorf("Expected tool_choice function 'get_weather', got %v", req.ToolChoice.Function)
	}
}

func TestResponsesParseRequest_WithReasoning(t *testing.T) {
	adapter := NewResponsesClientAdapter()
	ctx := context.Background()

	body := `{
		"model": "o1",
		"input": "Solve this problem",
		"reasoning": {"effort": "high"}
	}`

	req, err := adapter.ParseRequest(ctx, []byte(body), http.Header{})
	if err != nil {
		t.Fatalf("ParseRequest failed: %v", err)
	}

	// Verify reasoning config
	if req.Reasoning == nil {
		t.Fatal("Expected reasoning config, got nil")
	}
	if req.Reasoning.Effort == nil || *req.Reasoning.Effort != "high" {
		t.Errorf("Expected reasoning effort 'high', got %v", req.Reasoning.Effort)
	}
}

func TestResponsesParseRequest_WithTextFormat(t *testing.T) {
	adapter := NewResponsesClientAdapter()
	ctx := context.Background()

	body := `{
		"model": "gpt-4o",
		"input": "Generate JSON",
		"text": {
			"format": {
				"type": "json_schema",
				"name": "my_schema",
				"schema": {"type": "object", "properties": {"name": {"type": "string"}}}
			}
		}
	}`

	req, err := adapter.ParseRequest(ctx, []byte(body), http.Header{})
	if err != nil {
		t.Fatalf("ParseRequest failed: %v", err)
	}

	// Verify response format
	if req.ResponseFormat == nil {
		t.Fatal("Expected response_format, got nil")
	}
	if req.ResponseFormat.Type != "json_schema" {
		t.Errorf("Expected response_format type 'json_schema', got '%s'", req.ResponseFormat.Type)
	}
	if req.ResponseFormat.JsonSchema == nil {
		t.Fatal("Expected json_schema, got nil")
	}
	if req.ResponseFormat.JsonSchema.Name != "my_schema" {
		t.Errorf("Expected schema name 'my_schema', got '%s'", req.ResponseFormat.JsonSchema.Name)
	}
}

func TestResponsesFormatResponse_OutputItems(t *testing.T) {
	adapter := NewResponsesClientAdapter()
	ctx := context.Background()

	// Create canonical response
	finish := "stop"
	resp := &canonical.Response{
		ID:      "resp_abc123",
		Model:   "gpt-4o",
		Created: 1700000000,
		Choices: []canonical.Choice{
			{
				Index: 0,
				Message: canonical.Message{
					Role: canonical.RoleAssistant,
					Content: []canonical.ContentBlock{{
						Type: canonical.ContentText,
						Text: "Hello! How can I help you?",
					}},
				},
				FinishReason: &finish,
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

	// Verify output is valid JSON
	var respOut ResponsesResponse
	if err := json.Unmarshal(output, &respOut); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	// Verify key fields
	if respOut.ID != "resp_abc123" {
		t.Errorf("Expected ID 'resp_abc123', got '%s'", respOut.ID)
	}
	if respOut.Model != "gpt-4o" {
		t.Errorf("Expected model 'gpt-4o', got '%s'", respOut.Model)
	}
	if respOut.Status != "completed" {
		t.Errorf("Expected status 'completed', got '%s'", respOut.Status)
	}

	// Verify output items
	if len(respOut.Output) == 0 {
		t.Fatal("Expected at least 1 output item, got 0")
	}

	// Find message item
	var foundMessage bool
	for _, item := range respOut.Output {
		if item.Type == "message" {
			foundMessage = true
			if item.Role != "assistant" {
				t.Errorf("Expected message role 'assistant', got '%s'", item.Role)
			}
			if len(item.Content) == 0 {
				t.Fatal("Expected content in message item")
			}
			if item.Content[0].Type != "output_text" {
				t.Errorf("Expected content type 'output_text', got '%s'", item.Content[0].Type)
			}
			if item.Content[0].Text != "Hello! How can I help you?" {
				t.Errorf("Expected content 'Hello! How can I help you?', got '%s'", item.Content[0].Text)
			}
		}
	}
	if !foundMessage {
		t.Error("Expected to find message output item")
	}

	// Verify usage
	if respOut.Usage == nil {
		t.Error("Expected usage, got nil")
	} else {
		if respOut.Usage.InputTokens != 10 {
			t.Errorf("Expected input_tokens 10, got %d", respOut.Usage.InputTokens)
		}
		if respOut.Usage.OutputTokens != 8 {
			t.Errorf("Expected output_tokens 8, got %d", respOut.Usage.OutputTokens)
		}
	}
}

func TestResponsesFormatResponse_WithToolCalls(t *testing.T) {
	adapter := NewResponsesClientAdapter()
	ctx := context.Background()

	finish := "tool_calls"
	resp := &canonical.Response{
		ID:      "resp_tool1",
		Model:   "gpt-4o",
		Created: 1700000000,
		Choices: []canonical.Choice{
			{
				Index: 0,
				Message: canonical.Message{
					Role: canonical.RoleAssistant,
					ToolCalls: []canonical.ToolCall{{
						ID:        "call_abc123",
						Type:      "function",
						Name:      "get_weather",
						Arguments: `{"location": "Tokyo"}`,
					}},
				},
				FinishReason: &finish,
			},
		},
	}

	output, err := adapter.FormatResponse(ctx, resp)
	if err != nil {
		t.Fatalf("FormatResponse failed: %v", err)
	}

	var respOut ResponsesResponse
	if err := json.Unmarshal(output, &respOut); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	// Find function_call item
	var foundFunctionCall bool
	for _, item := range respOut.Output {
		if item.Type == "function_call" {
			foundFunctionCall = true
			if item.CallID != "call_abc123" {
				t.Errorf("Expected call_id 'call_abc123', got '%s'", item.CallID)
			}
			if item.Name != "get_weather" {
				t.Errorf("Expected name 'get_weather', got '%s'", item.Name)
			}
			if item.Arguments != `{"location": "Tokyo"}` {
				t.Errorf("Expected arguments '{\"location\": \"Tokyo\"}', got '%s'", item.Arguments)
			}
		}
	}
	if !foundFunctionCall {
		t.Error("Expected to find function_call output item")
	}
}

func TestResponsesFormatResponse_WithReasoning(t *testing.T) {
	adapter := NewResponsesClientAdapter()
	ctx := context.Background()

	finish := "stop"
	reasoning := "Step 1: Analyze the problem..."
	resp := &canonical.Response{
		ID:      "resp_reasoning1",
		Model:   "o1",
		Created: 1700000000,
		Choices: []canonical.Choice{
			{
				Index: 0,
				Message: canonical.Message{
					Role: canonical.RoleAssistant,
					Reasoning: &reasoning,
					Content: []canonical.ContentBlock{{
						Type: canonical.ContentText,
						Text: "The answer is 42.",
					}},
				},
				FinishReason: &finish,
			},
		},
		Usage: &canonical.Usage{
			PromptTokens:     20,
			CompletionTokens: 50,
			TotalTokens:      70,
			CompletionTokensDetails: &canonical.CompletionTokensDetails{
				ReasoningTokens: 30,
			},
		},
	}

	output, err := adapter.FormatResponse(ctx, resp)
	if err != nil {
		t.Fatalf("FormatResponse failed: %v", err)
	}

	var respOut ResponsesResponse
	if err := json.Unmarshal(output, &respOut); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	// Find reasoning item
	var foundReasoning bool
	for _, item := range respOut.Output {
		if item.Type == "reasoning" {
			foundReasoning = true
			if len(item.Summary) == 0 {
				t.Fatal("Expected summary in reasoning item")
			}
			if item.Summary[0].Text != "Step 1: Analyze the problem..." {
				t.Errorf("Expected reasoning text 'Step 1: Analyze the problem...', got '%s'", item.Summary[0].Text)
			}
		}
	}
	if !foundReasoning {
		t.Error("Expected to find reasoning output item")
	}

	// Verify reasoning tokens in usage
	if respOut.Usage == nil || respOut.Usage.OutputTokensDetails == nil {
		t.Fatal("Expected output tokens details, got nil")
	}
	if respOut.Usage.OutputTokensDetails.ReasoningTokens != 30 {
		t.Errorf("Expected reasoning_tokens 30, got %d", respOut.Usage.OutputTokensDetails.ReasoningTokens)
	}
}

func TestResponsesFormatStreamChunk_StatefulEvents(t *testing.T) {
	adapter := NewResponsesClientAdapter()
	ctx := context.Background()

	// First chunk should trigger response.created and in_progress
	chunk1 := &canonical.Chunk{
		ID:      "resp_123",
		Model:   "gpt-4o",
		Created: 1700000000,
		Deltas: []canonical.ChoiceDelta{
			{
				Index: 0,
				Delta: canonical.Message{
					Role: canonical.RoleAssistant,
				},
			},
		},
	}

	output1, err := adapter.FormatStreamChunk(ctx, chunk1)
	if err != nil {
		t.Fatalf("FormatStreamChunk failed: %v", err)
	}

	outputStr1 := string(output1)
	if !contains(outputStr1, "response.created") {
		t.Error("Expected response.created event")
	}
	if !contains(outputStr1, "response.in_progress") {
		t.Error("Expected response.in_progress event")
	}

	// Second chunk with text content
	chunk2 := &canonical.Chunk{
		ID:    "resp_123",
		Model: "gpt-4o",
		Deltas: []canonical.ChoiceDelta{
			{
				Index: 0,
				Delta: canonical.Message{
					Content: []canonical.ContentBlock{{
						Type: canonical.ContentText,
						Text: "Hello",
					}},
				},
			},
		},
	}

	output2, err := adapter.FormatStreamChunk(ctx, chunk2)
	if err != nil {
		t.Fatalf("FormatStreamChunk failed: %v", err)
	}

	outputStr2 := string(output2)
	if !contains(outputStr2, "response.output_text.delta") {
		t.Error("Expected output_text.delta event")
	}
	if !contains(outputStr2, "Hello") {
		t.Error("Expected 'Hello' in output")
	}

	// Final chunk with finish reason
	finish := "stop"
	chunk3 := &canonical.Chunk{
		ID:    "resp_123",
		Model: "gpt-4o",
		Deltas: []canonical.ChoiceDelta{
			{
				Index:        0,
				FinishReason: &finish,
			},
		},
		Usage: &canonical.Usage{
			PromptTokens:     10,
			CompletionTokens: 5,
			TotalTokens:      15,
		},
	}

	output3, err := adapter.FormatStreamChunk(ctx, chunk3)
	if err != nil {
		t.Fatalf("FormatStreamChunk failed: %v", err)
	}

	outputStr3 := string(output3)
	if !contains(outputStr3, "response.completed") {
		t.Error("Expected response.completed event")
	}

	// Done marker
	chunk4 := &canonical.Chunk{Done: true}
	output4, err := adapter.FormatStreamChunk(ctx, chunk4)
	if err != nil {
		t.Fatalf("FormatStreamChunk failed: %v", err)
	}
	if string(output4) != "data: [DONE]\n\n" {
		t.Errorf("Expected 'data: [DONE]\\n\\n', got '%s'", string(output4))
	}
}

func TestResponsesFormatStreamChunk_ToolCalls(t *testing.T) {
	adapter := NewResponsesClientAdapter()
	ctx := context.Background()

	// First chunk to initialize response
	chunk1 := &canonical.Chunk{
		ID:      "resp_123",
		Model:   "gpt-4o",
		Created: 1700000000,
		Deltas: []canonical.ChoiceDelta{
			{
				Index: 0,
				Delta: canonical.Message{
					Role: canonical.RoleAssistant,
				},
			},
		},
	}
	adapter.FormatStreamChunk(ctx, chunk1)

	// Tool call start
	chunk2 := &canonical.Chunk{
		ID:    "resp_123",
		Model: "gpt-4o",
		Deltas: []canonical.ChoiceDelta{
			{
				Index: 0,
				Delta: canonical.Message{
					ToolCalls: []canonical.ToolCall{{
						ID:    "call_123",
						Type:  "function",
						Name:  "get_weather",
						Index: 0,
					}},
				},
			},
		},
	}

	output2, err := adapter.FormatStreamChunk(ctx, chunk2)
	if err != nil {
		t.Fatalf("FormatStreamChunk failed: %v", err)
	}

	outputStr2 := string(output2)
	if !contains(outputStr2, "response.output_item.added") {
		t.Error("Expected output_item.added event for function_call")
	}

	// Tool call arguments delta
	chunk3 := &canonical.Chunk{
		ID:    "resp_123",
		Model: "gpt-4o",
		Deltas: []canonical.ChoiceDelta{
			{
				Index: 0,
				Delta: canonical.Message{
					ToolCalls: []canonical.ToolCall{{
						Index:     0,
						Arguments: "{\"loc",
					}},
				},
			},
		},
	}

	output3, err := adapter.FormatStreamChunk(ctx, chunk3)
	if err != nil {
		t.Fatalf("FormatStreamChunk failed: %v", err)
	}

	outputStr3 := string(output3)
	if !contains(outputStr3, "response.function_call_arguments.delta") {
		t.Error("Expected function_call_arguments.delta event")
	}
	// Note: JSON escaping means the delta will contain escaped quotes
	// The actual delta content should be present
	if !contains(outputStr3, "loc") {
		t.Errorf("Expected 'loc' in output, got: %s", outputStr3)
	}
}

func TestResponsesFormatStreamChunk_Reasoning(t *testing.T) {
	adapter := NewResponsesClientAdapter()
	ctx := context.Background()

	// First chunk to initialize response
	chunk1 := &canonical.Chunk{
		ID:      "resp_123",
		Model:   "o1",
		Created: 1700000000,
		Deltas: []canonical.ChoiceDelta{
			{
				Index: 0,
				Delta: canonical.Message{
					Role: canonical.RoleAssistant,
				},
			},
		},
	}
	adapter.FormatStreamChunk(ctx, chunk1)

	// Reasoning delta
	reasoning := "Step 1: "
	chunk2 := &canonical.Chunk{
		ID:    "resp_123",
		Model: "o1",
		Deltas: []canonical.ChoiceDelta{
			{
				Index: 0,
				Delta: canonical.Message{
					Reasoning: &reasoning,
				},
			},
		},
	}

	output2, err := adapter.FormatStreamChunk(ctx, chunk2)
	if err != nil {
		t.Fatalf("FormatStreamChunk failed: %v", err)
	}

	outputStr2 := string(output2)
	if !contains(outputStr2, "response.reasoning_summary_text.delta") {
		t.Error("Expected reasoning_summary_text.delta event")
	}
	if !contains(outputStr2, "Step 1: ") {
		t.Error("Expected 'Step 1: ' in output")
	}
}

func TestResponsesAggregateStream(t *testing.T) {
	adapter := NewResponsesClientAdapter()
	ctx := context.Background()

	// Create chunks for streaming simulation
	chunks := []*canonical.Chunk{
		{
			ID:      "resp_123",
			Model:   "gpt-4o",
			Created: 1700000000,
			Deltas: []canonical.ChoiceDelta{
				{
					Index: 0,
					Delta: canonical.Message{
						Role: canonical.RoleAssistant,
					},
				},
			},
		},
		{
			ID:    "resp_123",
			Model: "gpt-4o",
			Deltas: []canonical.ChoiceDelta{
				{
					Index: 0,
					Delta: canonical.Message{
						Content: []canonical.ContentBlock{{
							Type: canonical.ContentText,
							Text: "Hello",
						}},
					},
				},
			},
		},
		{
			ID:    "resp_123",
			Model: "gpt-4o",
			Deltas: []canonical.ChoiceDelta{
				{
					Index: 0,
					Delta: canonical.Message{
						Content: []canonical.ContentBlock{{
							Type: canonical.ContentText,
							Text: " there!",
						}},
					},
				},
			},
		},
		{
			ID:    "resp_123",
			Model: "gpt-4o",
			Usage: &canonical.Usage{
				PromptTokens:     5,
				CompletionTokens: 3,
				TotalTokens:      8,
			},
		},
	}

	for _, chunk := range chunks {
		adapter.FormatStreamChunk(ctx, chunk)
	}

	// Aggregate the final response
	resp, err := adapter.AggregateStream(ctx)
	if err != nil {
		t.Fatalf("AggregateStream failed: %v", err)
	}

	// Verify aggregated response
	if resp.ID != "resp_123" {
		t.Errorf("Expected ID 'resp_123', got '%s'", resp.ID)
	}
	if resp.Model != "gpt-4o" {
		t.Errorf("Expected model 'gpt-4o', got '%s'", resp.Model)
	}

	// Verify content was aggregated
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

	// Verify usage
	if resp.Usage == nil {
		t.Error("Expected usage, got nil")
	} else {
		if resp.Usage.PromptTokens != 5 {
			t.Errorf("Expected prompt_tokens 5, got %d", resp.Usage.PromptTokens)
		}
		if resp.Usage.CompletionTokens != 3 {
			t.Errorf("Expected completion_tokens 3, got %d", resp.Usage.CompletionTokens)
		}
	}
}

func TestResponsesFormatError(t *testing.T) {
	adapter := NewResponsesClientAdapter()
	ctx := context.Background()

	canonicalErr := &canonical.Error{
		Code:       "invalid_api_key",
		Message:    "Invalid API key provided",
		Type:       "invalid_request_error",
		StatusCode: 401,
	}

	output, err := adapter.FormatError(ctx, canonicalErr)
	if err != nil {
		t.Fatalf("FormatError failed: %v", err)
	}

	// Verify output is valid JSON
	var errResp struct {
		Error ResponsesError `json:"error"`
	}
	if err := json.Unmarshal(output, &errResp); err != nil {
		t.Fatalf("Failed to unmarshal error: %v", err)
	}

	if errResp.Error.Message != "Invalid API key provided" {
		t.Errorf("Expected message 'Invalid API key provided', got '%s'", errResp.Error.Message)
	}
	if errResp.Error.Code != "invalid_api_key" {
		t.Errorf("Expected code 'invalid_api_key', got '%s'", errResp.Error.Code)
	}
	if errResp.Error.Type != "invalid_request_error" {
		t.Errorf("Expected type 'invalid_request_error', got '%s'", errResp.Error.Type)
	}
}

func TestResponsesParseRequest_FunctionCallInput(t *testing.T) {
	adapter := NewResponsesClientAdapter()
	ctx := context.Background()

	body := `{
		"model": "gpt-4o",
		"input": [
			{
				"type": "message",
				"role": "user",
				"content": [{"type": "input_text", "text": "What's the weather?"}]
			},
			{
				"type": "function_call",
				"call_id": "call_123",
				"name": "get_weather",
				"arguments": "{\"location\": \"Tokyo\"}"
			},
			{
				"type": "function_call_output",
				"call_id": "call_123",
				"output": "Sunny, 25C"
			}
		]
	}`

	req, err := adapter.ParseRequest(ctx, []byte(body), http.Header{})
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

	// Second message: assistant with tool_call
	if req.Messages[1].Role != canonical.RoleAssistant {
		t.Errorf("Expected second message role 'assistant', got '%s'", req.Messages[1].Role)
	}
	if len(req.Messages[1].ToolCalls) != 1 {
		t.Fatalf("Expected 1 tool_call, got %d", len(req.Messages[1].ToolCalls))
	}
	if req.Messages[1].ToolCalls[0].ID != "call_123" {
		t.Errorf("Expected tool_call id 'call_123', got '%s'", req.Messages[1].ToolCalls[0].ID)
	}
	if req.Messages[1].ToolCalls[0].Name != "get_weather" {
		t.Errorf("Expected tool_call name 'get_weather', got '%s'", req.Messages[1].ToolCalls[0].Name)
	}

	// Third message: tool result
	if req.Messages[2].Role != canonical.RoleTool {
		t.Errorf("Expected third message role 'tool', got '%s'", req.Messages[2].Role)
	}
	if req.Messages[2].ToolCallID == nil || *req.Messages[2].ToolCallID != "call_123" {
		t.Errorf("Expected tool_call_id 'call_123', got %v", req.Messages[2].ToolCallID)
	}
}

// Note: contains and findSubstring helpers are defined in client_chat_test.go