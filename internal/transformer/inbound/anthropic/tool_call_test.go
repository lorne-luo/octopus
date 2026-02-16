package anthropic

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/bestruirui/octopus/internal/transformer/model"
	"github.com/samber/lo"
)

// TestTransformResponse_ToolCalls tests that tool calls are correctly transformed
// from internal format to Anthropic format, including cache_control
func TestTransformResponse_ToolCalls(t *testing.T) {
	tests := []struct {
		name     string
		response *model.InternalLLMResponse
		want     string // expected JSON output
		wantErr  bool
	}{
		{
			name: "tool call without cache control",
			response: &model.InternalLLMResponse{
				ID:    "msg_123",
				Model: "claude-3-5-sonnet-20241022",
				Choices: []model.Choice{
					{
						Index: 0,
						Message: &model.Message{
							Role: "assistant",
							ToolCalls: []model.ToolCall{
								{
									ID:   "call_abc",
									Type: "function",
									Function: model.FunctionCall{
										Name:      "get_weather",
										Arguments: `{"location":"Beijing"}`,
									},
								},
							},
						},
						FinishReason: lo.ToPtr("tool_calls"),
					},
				},
			},
			want: `{
				"id": "msg_123",
				"type": "message",
				"role": "assistant",
				"model": "claude-3-5-sonnet-20241022",
				"content": [{
					"type": "tool_use",
					"id": "call_abc",
					"name": "get_weather",
					"input": {"location":"Beijing"}
				}],
				"stop_reason": "tool_use"
			}`,
		},
		{
			name: "tool call with cache control",
			response: &model.InternalLLMResponse{
				ID:    "msg_456",
				Model: "claude-3-5-sonnet-20241022",
				Choices: []model.Choice{
					{
						Index: 0,
						Message: &model.Message{
							Role: "assistant",
							ToolCalls: []model.ToolCall{
								{
									ID:   "call_xyz",
									Type: "function",
									Function: model.FunctionCall{
										Name:      "search_database",
										Arguments: `{"query":"test"}`,
									},
									CacheControl: &model.CacheControl{
										Type: "ephemeral",
									},
								},
							},
						},
						FinishReason: lo.ToPtr("tool_calls"),
					},
				},
			},
			want: `{
				"id": "msg_456",
				"type": "message",
				"role": "assistant",
				"model": "claude-3-5-sonnet-20241022",
				"content": [{
					"type": "tool_use",
					"id": "call_xyz",
					"name": "search_database",
					"input": {"query":"test"},
					"cache_control": {"type":"ephemeral"}
				}],
				"stop_reason": "tool_use"
			}`,
		},
		{
			name: "multiple tool calls with mixed cache control",
			response: &model.InternalLLMResponse{
				ID:    "msg_789",
				Model: "claude-3-5-sonnet-20241022",
				Choices: []model.Choice{
					{
						Index: 0,
						Message: &model.Message{
							Role: "assistant",
							ToolCalls: []model.ToolCall{
								{
									ID:   "call_1",
									Type: "function",
									Function: model.FunctionCall{
										Name:      "tool_one",
										Arguments: `{"arg":"value1"}`,
									},
									CacheControl: &model.CacheControl{
										Type: "ephemeral",
									},
								},
								{
									ID:   "call_2",
									Type: "function",
									Function: model.FunctionCall{
										Name:      "tool_two",
										Arguments: `{"arg":"value2"}`,
									},
									// No cache control
								},
							},
						},
						FinishReason: lo.ToPtr("tool_calls"),
					},
				},
			},
			want: `{
				"id": "msg_789",
				"type": "message",
				"role": "assistant",
				"model": "claude-3-5-sonnet-20241022",
				"content": [
					{
						"type": "tool_use",
						"id": "call_1",
						"name": "tool_one",
						"input": {"arg":"value1"},
						"cache_control": {"type":"ephemeral"}
					},
					{
						"type": "tool_use",
						"id": "call_2",
						"name": "tool_two",
						"input": {"arg":"value2"}
					}
				],
				"stop_reason": "tool_use"
			}`,
		},
		{
			name: "tool call with text content",
			response: &model.InternalLLMResponse{
				ID:    "msg_mixed",
				Model: "claude-3-5-sonnet-20241022",
				Choices: []model.Choice{
					{
						Index: 0,
						Message: &model.Message{
							Role: "assistant",
							Content: model.MessageContent{
								Content: lo.ToPtr("Let me check the weather for you."),
							},
							ToolCalls: []model.ToolCall{
								{
									ID:   "call_weather",
									Type: "function",
									Function: model.FunctionCall{
										Name:      "get_weather",
										Arguments: `{"location":"Shanghai"}`,
									},
								},
							},
						},
						FinishReason: lo.ToPtr("tool_calls"),
					},
				},
			},
			want: `{
				"id": "msg_mixed",
				"type": "message",
				"role": "assistant",
				"model": "claude-3-5-sonnet-20241022",
				"content": [
					{
						"type": "text",
						"text": "Let me check the weather for you."
					},
					{
						"type": "tool_use",
						"id": "call_weather",
						"name": "get_weather",
						"input": {"location":"Shanghai"}
					}
				],
				"stop_reason": "tool_use"
			}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inbound := &MessagesInbound{}
			got, err := inbound.TransformResponse(context.Background(), tt.response)
			if (err != nil) != tt.wantErr {
				t.Errorf("TransformResponse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Parse both got and want as JSON for comparison
			var gotJSON, wantJSON map[string]interface{}
			if err := json.Unmarshal(got, &gotJSON); err != nil {
				t.Fatalf("Failed to unmarshal got JSON: %v", err)
			}
			if err := json.Unmarshal([]byte(tt.want), &wantJSON); err != nil {
				t.Fatalf("Failed to unmarshal want JSON: %v", err)
			}

			// Compare JSON structures
			gotBytes, _ := json.MarshalIndent(gotJSON, "", "  ")
			wantBytes, _ := json.MarshalIndent(wantJSON, "", "  ")

			if string(gotBytes) != string(wantBytes) {
				t.Errorf("TransformResponse() mismatch:\nGot:\n%s\n\nWant:\n%s", gotBytes, wantBytes)
			}
		})
	}
}

// TestMergeToolCall tests the mergeToolCall function for correct behavior
func TestMergeToolCall(t *testing.T) {
	tests := []struct {
		name      string
		existing  []model.ToolCall
		delta     model.ToolCall
		want      []model.ToolCall
		wantIssue string // describe the bug if any
	}{
		{
			name:     "new tool call",
			existing: []model.ToolCall{},
			delta: model.ToolCall{
				Index: 0,
				ID:    "call_1",
				Type:  "function",
				Function: model.FunctionCall{
					Name:      "get_weather",
					Arguments: "",
				},
			},
			want: []model.ToolCall{
				{
					Index: 0,
					ID:    "call_1",
					Type:  "function",
					Function: model.FunctionCall{
						Name:      "get_weather",
						Arguments: "",
					},
				},
			},
		},
		{
			name: "append arguments",
			existing: []model.ToolCall{
				{
					Index: 0,
					ID:    "call_1",
					Type:  "function",
					Function: model.FunctionCall{
						Name:      "get_weather",
						Arguments: `{"location"`,
					},
				},
			},
			delta: model.ToolCall{
				Index: 0,
				Function: model.FunctionCall{
					Arguments: `:"Beijing"}`,
				},
			},
			want: []model.ToolCall{
				{
					Index: 0,
					ID:    "call_1",
					Type:  "function",
					Function: model.FunctionCall{
						Name:      "get_weather",
						Arguments: `{"location":"Beijing"}`,
					},
				},
			},
		},
		{
			name: "name should not be concatenated if already set",
			existing: []model.ToolCall{
				{
					Index: 0,
					ID:    "call_1",
					Type:  "function",
					Function: model.FunctionCall{
						Name:      "get_weather",
						Arguments: "",
					},
				},
			},
			delta: model.ToolCall{
				Index: 0,
				Function: model.FunctionCall{
					Name: "get_weather", // Same name sent again
				},
			},
			want: []model.ToolCall{
				{
					Index: 0,
					ID:    "call_1",
					Type:  "function",
					Function: model.FunctionCall{
						Name:      "get_weather", // Should NOT be duplicated
						Arguments: "",
					},
				},
			},
			wantIssue: "Function name should not be concatenated if already set",
		},
		{
			name: "cache control is preserved from delta",
			existing: []model.ToolCall{
				{
					Index: 0,
					ID:    "call_1",
					Type:  "function",
					Function: model.FunctionCall{
						Name:      "search",
						Arguments: "",
					},
				},
			},
			delta: model.ToolCall{
				Index: 0,
				Function: model.FunctionCall{
					Arguments: `{"q":"test"}`,
				},
				CacheControl: &model.CacheControl{
					Type: "ephemeral",
				},
			},
			want: []model.ToolCall{
				{
					Index: 0,
					ID:    "call_1",
					Type:  "function",
					Function: model.FunctionCall{
						Name:      "search",
						Arguments: `{"q":"test"}`,
					},
					CacheControl: &model.CacheControl{
						Type: "ephemeral",
					},
				},
			},
			wantIssue: "CacheControl should be preserved from delta",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mergeToolCall(tt.existing, tt.delta)

			// Compare
			gotJSON, _ := json.MarshalIndent(got, "", "  ")
			wantJSON, _ := json.MarshalIndent(tt.want, "", "  ")

			if string(gotJSON) != string(wantJSON) {
				if tt.wantIssue != "" {
					t.Logf("KNOWN BUG: %s", tt.wantIssue)
				}
				t.Errorf("mergeToolCall() mismatch:\nGot:\n%s\n\nWant:\n%s", gotJSON, wantJSON)
			}
		})
	}
}

// TestGetInternalResponse_ToolCallAggregation tests stream chunk aggregation for tool calls
func TestGetInternalResponse_ToolCallAggregation(t *testing.T) {
	inbound := &MessagesInbound{}

	// Simulate streaming chunks
	chunks := []*model.InternalLLMResponse{
		{
			ID:    "msg_stream",
			Model: "claude-3-5-sonnet-20241022",
			Choices: []model.Choice{
				{
					Index: 0,
					Delta: &model.Message{
						Role: "assistant",
					},
				},
			},
		},
		{
			ID:    "msg_stream",
			Model: "claude-3-5-sonnet-20241022",
			Choices: []model.Choice{
				{
					Index: 0,
					Delta: &model.Message{
						ToolCalls: []model.ToolCall{
							{
								Index: 0,
								ID:    "call_abc",
								Type:  "function",
								Function: model.FunctionCall{
									Name:      "get_weather",
									Arguments: "",
								},
							},
						},
					},
				},
			},
		},
		{
			ID:    "msg_stream",
			Model: "claude-3-5-sonnet-20241022",
			Choices: []model.Choice{
				{
					Index: 0,
					Delta: &model.Message{
						ToolCalls: []model.ToolCall{
							{
								Index: 0,
								Function: model.FunctionCall{
									Arguments: `{"location"`,
								},
							},
						},
					},
				},
			},
		},
		{
			ID:    "msg_stream",
			Model: "claude-3-5-sonnet-20241022",
			Choices: []model.Choice{
				{
					Index: 0,
					Delta: &model.Message{
						ToolCalls: []model.ToolCall{
							{
								Index: 0,
								Function: model.FunctionCall{
									Arguments: `:"Beijing"}`,
								},
							},
						},
					},
				},
			},
		},
		{
			ID:    "msg_stream",
			Model: "claude-3-5-sonnet-20241022",
			Choices: []model.Choice{
				{
					Index:        0,
					FinishReason: lo.ToPtr("tool_calls"),
				},
			},
		},
	}

	// Store chunks
	inbound.streamChunks = chunks

	// Get aggregated response
	got, err := inbound.GetInternalResponse(context.Background())
	if err != nil {
		t.Fatalf("GetInternalResponse() error = %v", err)
	}

	// Verify aggregated tool call
	if len(got.Choices) != 1 {
		t.Fatalf("Expected 1 choice, got %d", len(got.Choices))
	}

	choice := got.Choices[0]
	if len(choice.Message.ToolCalls) != 1 {
		t.Fatalf("Expected 1 tool call, got %d", len(choice.Message.ToolCalls))
	}

	toolCall := choice.Message.ToolCalls[0]
	if toolCall.ID != "call_abc" {
		t.Errorf("Expected ID 'call_abc', got '%s'", toolCall.ID)
	}
	if toolCall.Function.Name != "get_weather" {
		t.Errorf("Expected name 'get_weather', got '%s'", toolCall.Function.Name)
	}
	if toolCall.Function.Arguments != `{"location":"Beijing"}` {
		t.Errorf("Expected arguments '{\"location\":\"Beijing\"}', got '%s'", toolCall.Function.Arguments)
	}
}
