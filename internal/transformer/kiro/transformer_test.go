package kiro

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/bestruirui/octopus/internal/transformer/model"
)

func TestResolveModel(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "exact match",
			input:    "claude-sonnet-4.5",
			expected: "claude-sonnet-4.5",
		},
		{
			name:     "alias mapping",
			input:    "claude-sonnet-4-5",
			expected: "claude-sonnet-4.5",
		},
		{
			name:     "bedrock style",
			input:    "anthropic.claude-3-5-sonnet-20241022-v2:0",
			expected: "claude-sonnet-4.5",
		},
		{
			name:     "unknown model passes through",
			input:    "unknown-model",
			expected: "unknown-model",
		},
		{
			name:     "claude-3-sonnet alias",
			input:    "claude-3-sonnet",
			expected: "claude-sonnet-4.5",
		},
		{
			name:     "claude-opus",
			input:    "claude-opus",
			expected: "claude-opus-4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ResolveModel(tt.input)
			if result != tt.expected {
				t.Errorf("ResolveModel(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestNormalizeModelName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		// Pattern 1: Standard format with minor version
		{
			name:     "standard format with minor - claude-haiku-4-5",
			input:    "claude-haiku-4-5",
			expected: "claude-haiku-4.5",
		},
		{
			name:     "standard format with date - claude-haiku-4-5-20251001",
			input:    "claude-haiku-4-5-20251001",
			expected: "claude-haiku-4.5",
		},
		{
			name:     "standard format sonnet",
			input:    "claude-sonnet-4-5",
			expected: "claude-sonnet-4.5",
		},
		// Pattern 2: Standard format without minor
		{
			name:     "no minor - claude-sonnet-4",
			input:    "claude-sonnet-4",
			expected: "claude-sonnet-4",
		},
		{
			name:     "no minor with date - claude-sonnet-4-20250514",
			input:    "claude-sonnet-4-20250514",
			expected: "claude-sonnet-4",
		},
		// Pattern 3: Legacy format
		{
			name:     "legacy format - claude-3-7-sonnet",
			input:    "claude-3-7-sonnet",
			expected: "claude-3.7-sonnet",
		},
		{
			name:     "legacy format with date - claude-3-7-sonnet-20250219",
			input:    "claude-3-7-sonnet-20250219",
			expected: "claude-3.7-sonnet",
		},
		// Pattern 4: Already normalized with date suffix
		{
			name:     "normalized with date - claude-haiku-4.5-20251001",
			input:    "claude-haiku-4.5-20251001",
			expected: "claude-haiku-4.5",
		},
		// Pattern 5: Inverted format with suffix
		{
			name:     "inverted format - claude-4.5-opus-high",
			input:    "claude-4.5-opus-high",
			expected: "claude-opus-4.5",
		},
		// Edge cases
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "unknown model",
			input:    "gpt-4",
			expected: "gpt-4",
		},
		{
			name:     "already normalized",
			input:    "claude-sonnet-4.5",
			expected: "claude-sonnet-4.5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeModelName(tt.input)
			if result != tt.expected {
				t.Errorf("NormalizeModelName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestIsClaudeModel(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"claude in name", "claude-sonnet-4.5", true},
		{"anthropic in name", "anthropic.claude-3-opus", true},
		{"not claude", "gpt-4", false},
		{"mixed case", "CLAUDE-SONNET", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsClaudeModel(tt.input)
			if result != tt.expected {
				t.Errorf("IsClaudeModel(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestGetModelFamily(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"opus family", "claude-opus-4", "opus"},
		{"sonnet family", "claude-sonnet-4.5", "sonnet"},
		{"haiku family", "claude-haiku-4", "haiku"},
		{"unknown family", "unknown-model", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetModelFamily(tt.input)
			if result != tt.expected {
				t.Errorf("GetModelFamily(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestMergeAdjacentMessages(t *testing.T) {
	content1 := "Hello"
	content2 := "World"
	content3 := "How are you?"

	messages := []model.Message{
		{Role: "user", Content: model.MessageContent{Content: &content1}},
		{Role: "user", Content: model.MessageContent{Content: &content2}},
		{Role: "assistant", Content: model.MessageContent{Content: &content3}},
	}

	result := MergeAdjacentMessages(messages)

	// Should merge adjacent user messages
	if len(result) != 2 {
		t.Fatalf("expected 2 messages after merge, got %d", len(result))
	}

	// First message should have merged content
	merged := result[0].Content.GetFullContent()
	if merged != "Hello\nWorld" {
		t.Errorf("expected merged content 'Hello\\nWorld', got %q", merged)
	}
}

func TestEnsureFirstMessageIsUser(t *testing.T) {
	content := "Hello"

	// Test with assistant first
	messages := []model.Message{
		{Role: "assistant", Content: model.MessageContent{Content: &content}},
		{Role: "user", Content: model.MessageContent{Content: &content}},
	}

	result := EnsureFirstMessageIsUser(messages)

	if len(result) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(result))
	}

	if result[0].Role != "user" {
		t.Errorf("expected first message to be user, got %s", result[0].Role)
	}

	// Test with user first - should not change
	messages = []model.Message{
		{Role: "user", Content: model.MessageContent{Content: &content}},
	}
	result = EnsureFirstMessageIsUser(messages)
	if len(result) != 1 {
		t.Errorf("expected 1 message, got %d", len(result))
	}
}

func TestEnsureAlternatingRoles(t *testing.T) {
	content := "Hello"

	// Test consecutive user messages
	messages := []model.Message{
		{Role: "user", Content: model.MessageContent{Content: &content}},
		{Role: "user", Content: model.MessageContent{Content: &content}},
		{Role: "assistant", Content: model.MessageContent{Content: &content}},
	}

	result := EnsureAlternatingRoles(messages)

	// Should insert assistant message
	if len(result) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(result))
	}

	if result[1].Role != "assistant" {
		t.Errorf("expected second message to be assistant, got %s", result[1].Role)
	}
}

func TestNormalizeMessageRoles(t *testing.T) {
	content := "Hello"

	messages := []model.Message{
		{Role: "user", Content: model.MessageContent{Content: &content}},
		{Role: "unknown", Content: model.MessageContent{Content: &content}},
		{Role: "assistant", Content: model.MessageContent{Content: &content}},
		{Role: "tool", Content: model.MessageContent{Content: &content}},
	}

	result := NormalizeMessageRoles(messages)

	if result[1].Role != "user" {
		t.Errorf("expected unknown role to be normalized to user, got %s", result[1].Role)
	}

	// user, assistant, and tool should remain unchanged
	if result[0].Role != "user" || result[2].Role != "assistant" || result[3].Role != "tool" {
		t.Error("expected user, assistant, and tool roles to remain unchanged")
	}
}

func TestEnsureFirstMessageIsUser_WithToolRole(t *testing.T) {
	content := "Hello"

	// Test with tool first - should NOT prepend since tool is user-like
	messages := []model.Message{
		{Role: "tool", Content: model.MessageContent{Content: &content}},
		{Role: "assistant", Content: model.MessageContent{Content: &content}},
	}

	result := EnsureFirstMessageIsUser(messages)

	if len(result) != 2 {
		t.Fatalf("expected 2 messages (no prepend), got %d", len(result))
	}

	if result[0].Role != "tool" {
		t.Errorf("expected first message to remain tool, got %s", result[0].Role)
	}
}

func TestEnsureAlternatingRoles_WithToolRole(t *testing.T) {
	content := "Hello"

	// Test consecutive user and tool messages
	messages := []model.Message{
		{Role: "user", Content: model.MessageContent{Content: &content}},
		{Role: "tool", Content: model.MessageContent{Content: &content}},
		{Role: "assistant", Content: model.MessageContent{Content: &content}},
	}

	result := EnsureAlternatingRoles(messages)

	// Should insert assistant message between user and tool
	if len(result) != 4 {
		t.Fatalf("expected 4 messages after inserting assistant, got %d", len(result))
	}

	if result[1].Role != "assistant" {
		t.Errorf("expected second message to be assistant, got %s", result[1].Role)
	}
}

func TestStripAllToolContent_WithToolRole(t *testing.T) {
	content := "Tool result content"
	toolCallID := "toolu_123"

	// Message with tool role
	messages := []model.Message{
		{
			Role:       "tool",
			Content:    model.MessageContent{Content: &content},
			ToolCallID: &toolCallID,
		},
	}

	result, hadToolContent := StripAllToolContent(messages)

	if !hadToolContent {
		t.Error("expected hadToolContent to be true")
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}

	// Tool role should be converted to user
	if result[0].Role != "user" {
		t.Errorf("expected role to be converted to user, got %s", result[0].Role)
	}

	// Should contain tool result text
	resultContent := result[0].Content.GetFullContent()
	if !strings.Contains(resultContent, "[Tool Result") {
		t.Errorf("expected content to contain tool result text, got %s", resultContent)
	}
}

func TestMergeAdjacentMessages_WithToolRole(t *testing.T) {
	content1 := "Hello"
	content2 := "Tool result"

	// Adjacent user and tool messages should be merged
	messages := []model.Message{
		{Role: "user", Content: model.MessageContent{Content: &content1}},
		{Role: "tool", Content: model.MessageContent{Content: &content2}},
	}

	result := MergeAdjacentMessages(messages)

	// Should merge into single message
	if len(result) != 1 {
		t.Fatalf("expected 1 message after merge, got %d", len(result))
	}

	merged := result[0].Content.GetFullContent()
	if !strings.Contains(merged, content1) || !strings.Contains(merged, content2) {
		t.Errorf("expected merged content to contain both messages, got %s", merged)
	}
}

func TestStripAllToolContent(t *testing.T) {
	content := "Hello"

	// Message with tool call
	toolArgs := `{"arg":"value"}`
	messages := []model.Message{
		{
			Role:    "assistant",
			Content: model.MessageContent{Content: &content},
			ToolCalls: []model.ToolCall{
				{
					ID:   "call_1",
					Type: "function",
					Function: model.FunctionCall{
						Name:      "test_func",
						Arguments: toolArgs,
					},
				},
			},
		},
	}

	result, hadToolContent := StripAllToolContent(messages)

	if !hadToolContent {
		t.Error("expected hadToolContent to be true")
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}

	if len(result[0].ToolCalls) != 0 {
		t.Error("expected tool calls to be stripped")
	}
}

func TestExtractTextContent(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected string
	}{
		{
			name:     "string input",
			input:    "Hello World",
			expected: "Hello World",
		},
		{
			name:     "MessageContent input",
			input:    model.MessageContent{Content: ptr("Test")},
			expected: "Test",
		},
		{
			name:     "nil input",
			input:    nil,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractTextContent(tt.input)
			if result != tt.expected {
				t.Errorf("ExtractTextContent() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func ptr(s string) *string {
	return &s
}

func TestSanitizeJSONSchema(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]interface{}
		expected map[string]interface{}
	}{
		{
			name:     "nil schema",
			input:    nil,
			expected: map[string]interface{}{},
		},
		{
			name: "remove additionalProperties",
			input: map[string]interface{}{
				"type":                 "object",
				"additionalProperties": true,
			},
			expected: map[string]interface{}{
				"type": "object",
			},
		},
		{
			name: "remove empty required array",
			input: map[string]interface{}{
				"type":     "object",
				"required": []interface{}{},
			},
			expected: map[string]interface{}{
				"type": "object",
			},
		},
		{
			name: "keep non-empty required array",
			input: map[string]interface{}{
				"type":     "object",
				"required": []interface{}{"name"},
			},
			expected: map[string]interface{}{
				"type":     "object",
				"required": []interface{}{"name"},
			},
		},
		{
			name: "sanitize nested properties",
			input: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"name": map[string]interface{}{
						"type":                 "string",
						"additionalProperties": false,
					},
				},
			},
			expected: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"name": map[string]interface{}{
						"type": "string",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeJSONSchema(tt.input)
			resultJSON, _ := json.Marshal(result)
			expectedJSON, _ := json.Marshal(tt.expected)
			if string(resultJSON) != string(expectedJSON) {
				t.Errorf("SanitizeJSONSchema() = %s, want %s", resultJSON, expectedJSON)
			}
		})
	}
}
