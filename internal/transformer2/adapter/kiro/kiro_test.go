package kiro

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/bestruirui/octopus/internal/transformer2/canonical"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── NormalizeModelName Tests ────────────────────────────────────────────

func TestNormalizeModelName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		// Empty
		{"empty string", "", ""},

		// Pattern 1: claude-{family}-{major}-{minor}(-suffix)?
		{"sonnet-4-5", "claude-sonnet-4-5", "claude-sonnet-4.5"},
		{"haiku-4-0", "claude-haiku-4-0", "claude-haiku-4.0"},
		{"opus-4-1 with date", "claude-opus-4-1-20250514", "claude-opus-4.1"},
		{"sonnet-4-5 with latest", "claude-sonnet-4-5-latest", "claude-sonnet-4.5"},

		// Pattern 2: claude-{family}-{major}(-date)?
		{"sonnet-4", "claude-sonnet-4", "claude-sonnet-4"},
		{"haiku-4 with date", "claude-haiku-4-20250101", "claude-haiku-4"},
		{"opus-4", "claude-opus-4", "claude-opus-4"},

		// Pattern 3: legacy claude-{major}-{minor}-{family}(-suffix)?
		{"legacy 3-5-sonnet", "claude-3-5-sonnet", "claude-3.5-sonnet"},
		{"legacy 3-5-sonnet with date", "claude-3-5-sonnet-20241022", "claude-3.5-sonnet"},
		{"legacy 3-0-haiku", "claude-3-0-haiku", "claude-3.0-haiku"},

		// Pattern 4+5: inverted format - claude-4.5-sonnet-date → strips date
		{"inverted 4.5-sonnet with date", "claude-4.5-sonnet-20250514", "claude-4.5-sonnet"},

		// No match — returns as is
		{"unknown model", "gpt-4o", "gpt-4o"},
		{"already normalized", "claude-sonnet-4.5", "claude-sonnet-4.5"},

		// Case insensitive
		{"uppercase", "Claude-Sonnet-4-5", "claude-sonnet-4.5"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeModelName(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// ── ResolveModel Tests ──────────────────────────────────────────────────

func TestResolveModel(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		// Direct alias
		{"alias claude-sonnet", "claude-sonnet", "claude-sonnet-4.5"},
		{"alias claude-opus", "claude-opus", "claude-opus-4"},
		{"alias claude-haiku", "claude-haiku", "claude-haiku-4"},
		{"alias claude-3-5-sonnet", "claude-3-5-sonnet", "claude-sonnet-4.5"},
		{"alias claude-3-opus", "claude-3-opus", "claude-opus-4"},

		// ARN prefix
		{"ARN sonnet", "anthropic.claude-sonnet-4-5-20250514-v1:0", "claude-sonnet-4.5"},
		{"ARN haiku", "anthropic.claude-3-haiku-20240307-v1:0", "claude-haiku-4"},
		{"ARN opus", "anthropic.claude-3-opus-20240229-v1:0", "claude-opus-4"},

		// Case insensitive
		{"case insensitive alias", "Claude-Sonnet", "claude-sonnet-4.5"},

		// Falls through to NormalizeModelName
		{"normalize fallback", "claude-sonnet-4-5", "claude-sonnet-4.5"},

		// Unknown model passes through
		{"unknown model", "gpt-4o", "gpt-4o"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ResolveModel(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// ── SanitizeJSONSchema Tests ───────────────────────────────────────────

func TestSanitizeJSONSchema(t *testing.T) {
	t.Run("nil schema returns empty map", func(t *testing.T) {
		result := SanitizeJSONSchema(nil)
		assert.NotNil(t, result)
		assert.Empty(t, result)
	})

	t.Run("removes additionalProperties", func(t *testing.T) {
		schema := map[string]interface{}{
			"type":                 "object",
			"additionalProperties": false,
		}
		result := SanitizeJSONSchema(schema)
		assert.Equal(t, "object", result["type"])
		assert.NotContains(t, result, "additionalProperties")
	})

	t.Run("removes empty required array", func(t *testing.T) {
		schema := map[string]interface{}{
			"type":     "object",
			"required": []interface{}{},
		}
		result := SanitizeJSONSchema(schema)
		assert.NotContains(t, result, "required")
	})

	t.Run("keeps non-empty required array", func(t *testing.T) {
		schema := map[string]interface{}{
			"type":     "object",
			"required": []interface{}{"name"},
		}
		result := SanitizeJSONSchema(schema)
		assert.Contains(t, result, "required")
	})

	t.Run("recursively sanitizes nested properties", func(t *testing.T) {
		schema := map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"name": map[string]interface{}{
					"type":                 "string",
					"additionalProperties": false,
				},
			},
		}
		result := SanitizeJSONSchema(schema)
		props := result["properties"].(map[string]interface{})
		nameProps := props["name"].(map[string]interface{})
		assert.Equal(t, "string", nameProps["type"])
		assert.NotContains(t, nameProps, "additionalProperties")
	})
}

// ── Message Processing Tests ────────────────────────────────────────────

func TestEnsureFirstMessageIsUser(t *testing.T) {
	t.Run("empty messages", func(t *testing.T) {
		result := ensureFirstMessageIsUser(nil)
		assert.Nil(t, result)
	})

	t.Run("first message is user", func(t *testing.T) {
		msgs := []canonical.Message{{Role: canonical.RoleUser}}
		result := ensureFirstMessageIsUser(msgs)
		assert.Len(t, result, 1)
		assert.Equal(t, canonical.RoleUser, result[0].Role)
	})

	t.Run("first message is assistant prepends user", func(t *testing.T) {
		msgs := []canonical.Message{{Role: canonical.RoleAssistant}}
		result := ensureFirstMessageIsUser(msgs)
		assert.Len(t, result, 2)
		assert.Equal(t, canonical.RoleUser, result[0].Role)
		assert.Equal(t, "(empty)", result[0].Content[0].Text)
		assert.Equal(t, canonical.RoleAssistant, result[1].Role)
	})
}

func TestMergeConsecutiveUserMessages(t *testing.T) {
	t.Run("single message unchanged", func(t *testing.T) {
		msgs := []canonical.Message{{Role: canonical.RoleUser, Content: []canonical.ContentBlock{{Type: canonical.ContentText, Text: "hello"}}}}
		result := mergeConsecutiveUserMessages(msgs)
		assert.Len(t, result, 1)
	})

	t.Run("consecutive user messages merged", func(t *testing.T) {
		msgs := []canonical.Message{
			{Role: canonical.RoleUser, Content: []canonical.ContentBlock{{Type: canonical.ContentText, Text: "hello"}}},
			{Role: canonical.RoleUser, Content: []canonical.ContentBlock{{Type: canonical.ContentText, Text: "world"}}},
		}
		result := mergeConsecutiveUserMessages(msgs)
		assert.Len(t, result, 1)
		assert.Len(t, result[0].Content, 2)
	})

	t.Run("user-assistant-user not merged", func(t *testing.T) {
		msgs := []canonical.Message{
			{Role: canonical.RoleUser},
			{Role: canonical.RoleAssistant},
			{Role: canonical.RoleUser},
		}
		result := mergeConsecutiveUserMessages(msgs)
		assert.Len(t, result, 3)
	})
}

func TestEnsureAlternatingRoles(t *testing.T) {
	t.Run("user-user inserts assistant", func(t *testing.T) {
		msgs := []canonical.Message{
			{Role: canonical.RoleUser, Content: []canonical.ContentBlock{{Type: canonical.ContentText, Text: "q1"}}},
			{Role: canonical.RoleUser, Content: []canonical.ContentBlock{{Type: canonical.ContentText, Text: "q2"}}},
		}
		result := ensureAlternatingRoles(msgs)
		assert.Len(t, result, 3)
		assert.Equal(t, canonical.RoleUser, result[0].Role)
		assert.Equal(t, canonical.RoleAssistant, result[1].Role)
		assert.Equal(t, "(empty)", result[1].Content[0].Text)
		assert.Equal(t, canonical.RoleUser, result[2].Role)
	})
}

// ── buildKiroRequest Tests ──────────────────────────────────────────────

func TestBuildKiroRequest(t *testing.T) {
	t.Run("simple text request", func(t *testing.T) {
		req := &canonical.Request{
			Model: "claude-sonnet-4-5",
			Messages: []canonical.Message{
				{Role: canonical.RoleUser, Content: []canonical.ContentBlock{{Type: canonical.ContentText, Text: "Hello Kiro"}}},
			},
		}
		kiroReq, err := buildKiroRequest(req)
		require.NoError(t, err)
		require.NotNil(t, kiroReq)

		assert.Equal(t, "MANUAL", kiroReq.ConversationState.ChatTriggerType)
		assert.Equal(t, "claude-sonnet-4.5", kiroReq.ConversationState.CurrentMessage.UserInputMessage.ModelID)
		assert.Equal(t, "Hello Kiro", kiroReq.ConversationState.CurrentMessage.UserInputMessage.Content)
		assert.Equal(t, "AI_EDITOR", kiroReq.ConversationState.CurrentMessage.UserInputMessage.Origin)
		assert.NotEmpty(t, kiroReq.ConversationState.ConversationID)
		assert.Empty(t, kiroReq.ConversationState.History)
	})

	t.Run("with system prompt", func(t *testing.T) {
		req := &canonical.Request{
			Model: "claude-sonnet",
			Messages: []canonical.Message{
				{Role: canonical.RoleSystem, Content: []canonical.ContentBlock{{Type: canonical.ContentText, Text: "You are helpful"}}},
				{Role: canonical.RoleUser, Content: []canonical.ContentBlock{{Type: canonical.ContentText, Text: "Hi"}}},
			},
		}
		kiroReq, err := buildKiroRequest(req)
		require.NoError(t, err)

		content := kiroReq.ConversationState.CurrentMessage.UserInputMessage.Content
		assert.Contains(t, content, "You are helpful")
		assert.Contains(t, content, "Hi")
	})

	t.Run("multi-turn builds history", func(t *testing.T) {
		req := &canonical.Request{
			Model: "claude-sonnet",
			Messages: []canonical.Message{
				{Role: canonical.RoleUser, Content: []canonical.ContentBlock{{Type: canonical.ContentText, Text: "Q1"}}},
				{Role: canonical.RoleAssistant, Content: []canonical.ContentBlock{{Type: canonical.ContentText, Text: "A1"}}},
				{Role: canonical.RoleUser, Content: []canonical.ContentBlock{{Type: canonical.ContentText, Text: "Q2"}}},
			},
		}
		kiroReq, err := buildKiroRequest(req)
		require.NoError(t, err)

		assert.Equal(t, "Q2", kiroReq.ConversationState.CurrentMessage.UserInputMessage.Content)
		assert.Len(t, kiroReq.ConversationState.History, 2)

		// First history: user message
		h0 := kiroReq.ConversationState.History[0].(map[string]interface{})
		userMsg := h0["userInputMessage"].(map[string]interface{})
		assert.Equal(t, "Q1", userMsg["content"])

		// Second history: assistant message
		h1 := kiroReq.ConversationState.History[1].(map[string]interface{})
		assistantMsg := h1["assistantResponseMessage"].(map[string]interface{})
		assert.Equal(t, "A1", assistantMsg["body"])
	})

	t.Run("with tools", func(t *testing.T) {
		params := json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}}}`)
		req := &canonical.Request{
			Model: "claude-sonnet",
			Messages: []canonical.Message{
				{Role: canonical.RoleUser, Content: []canonical.ContentBlock{{Type: canonical.ContentText, Text: "Search for foo"}}},
			},
			Tools: []canonical.Tool{
				{Name: "search", Description: "Search the web", Parameters: params},
			},
		}
		kiroReq, err := buildKiroRequest(req)
		require.NoError(t, err)

		ctx := kiroReq.ConversationState.CurrentMessage.UserInputMessage.UserInputMessageContext
		require.NotNil(t, ctx)
		require.Len(t, ctx.Tools, 1)

		toolSpec := ctx.Tools[0]["toolSpecification"].(map[string]interface{})
		assert.Equal(t, "search", toolSpec["name"])
		assert.Equal(t, "Search the web", toolSpec["description"])
	})

	t.Run("with tool result as last message", func(t *testing.T) {
		toolCallID := "tc_123"
		req := &canonical.Request{
			Model: "claude-sonnet",
			Messages: []canonical.Message{
				{Role: canonical.RoleUser, Content: []canonical.ContentBlock{{Type: canonical.ContentText, Text: "Q"}}},
				{Role: canonical.RoleAssistant, Content: []canonical.ContentBlock{{Type: canonical.ContentText, Text: "Let me search"}},
					ToolCalls: []canonical.ToolCall{{ID: "tc_123", Name: "search", Arguments: `{"q":"foo"}`}}},
				{Role: canonical.RoleTool, ToolCallID: &toolCallID, Content: []canonical.ContentBlock{{Type: canonical.ContentText, Text: "Found: bar"}}},
			},
		}
		kiroReq, err := buildKiroRequest(req)
		require.NoError(t, err)

		// Last message is tool result, should be extracted
		ctx := kiroReq.ConversationState.CurrentMessage.UserInputMessage.UserInputMessageContext
		require.NotNil(t, ctx)
		require.Len(t, ctx.ToolResults, 1)
		assert.Equal(t, "tc_123", ctx.ToolResults[0]["toolUseId"])
	})

	t.Run("with image", func(t *testing.T) {
		req := &canonical.Request{
			Model: "claude-sonnet",
			Messages: []canonical.Message{
				{Role: canonical.RoleUser, Content: []canonical.ContentBlock{
					{Type: canonical.ContentText, Text: "What's in this image?"},
					{Type: canonical.ContentImage, Media: &canonical.MediaContent{
						MimeType: "image/png",
						Base64:   "iVBORw0KGgo=",
					}},
				}},
			},
		}
		kiroReq, err := buildKiroRequest(req)
		require.NoError(t, err)

		images := kiroReq.ConversationState.CurrentMessage.UserInputMessage.Images
		require.Len(t, images, 1)
		assert.Equal(t, "png", images[0]["format"])
		source := images[0]["source"].(map[string]interface{})
		assert.Equal(t, "iVBORw0KGgo=", source["bytes"])
	})
}

// ── kiroToolCallsToCanonical Tests ──────────────────────────────────────

func TestKiroToolCallsToCanonical(t *testing.T) {
	t.Run("nil returns nil", func(t *testing.T) {
		result := kiroToolCallsToCanonical(nil)
		assert.Nil(t, result)
	})

	t.Run("converts tool calls", func(t *testing.T) {
		calls := []kiroToolCall{
			{ID: "tc1", Name: "search", Arguments: `{"q":"foo"}`},
			{ID: "tc2", Name: "read_file", Arguments: `{"path":"/tmp"}`},
		}
		result := kiroToolCallsToCanonical(calls)
		require.Len(t, result, 2)

		assert.Equal(t, "tc1", result[0].ID)
		assert.Equal(t, "function", result[0].Type)
		assert.Equal(t, "search", result[0].Name)
		assert.Equal(t, `{"q":"foo"}`, result[0].Arguments)

		assert.Equal(t, "tc2", result[1].ID)
		assert.Equal(t, "read_file", result[1].Name)
	})
}

// ── Provider BuildRequest Tests ─────────────────────────────────────────

func TestProviderAdapter_BuildRequest(t *testing.T) {
	t.Run("builds valid HTTP request", func(t *testing.T) {
		adapter := NewProviderAdapter()
		req := &canonical.Request{
			Model: "claude-sonnet-4-5",
			Messages: []canonical.Message{
				{Role: canonical.RoleUser, Content: []canonical.ContentBlock{{Type: canonical.ContentText, Text: "Hello"}}},
			},
		}

		httpReq, err := adapter.BuildRequest(context.Background(), req, "https://api.example.com", "sk-test-key")
		require.NoError(t, err)
		require.NotNil(t, httpReq)

		assert.Equal(t, http.MethodPost, httpReq.Method)
		assert.Equal(t, "https://api.example.com/invoke", httpReq.URL.String())
		assert.Equal(t, "application/json", httpReq.Header.Get("Content-Type"))
		assert.Equal(t, "Bearer sk-test-key", httpReq.Header.Get("Authorization"))
	})

	t.Run("strips trailing slash from baseURL", func(t *testing.T) {
		adapter := NewProviderAdapter()
		req := &canonical.Request{
			Model: "claude-sonnet",
			Messages: []canonical.Message{
				{Role: canonical.RoleUser, Content: []canonical.ContentBlock{{Type: canonical.ContentText, Text: "Hi"}}},
			},
		}

		httpReq, err := adapter.BuildRequest(context.Background(), req, "https://api.example.com/", "key")
		require.NoError(t, err)
		assert.Equal(t, "https://api.example.com/invoke", httpReq.URL.String())
	})

	t.Run("sets profile ARN header when configured", func(t *testing.T) {
		adapter := NewProviderAdapter()
		adapter.SetProfileArn("arn:aws:iam::123456:profile/test")

		req := &canonical.Request{
			Model: "claude-sonnet",
			Messages: []canonical.Message{
				{Role: canonical.RoleUser, Content: []canonical.ContentBlock{{Type: canonical.ContentText, Text: "Hi"}}},
			},
		}

		httpReq, err := adapter.BuildRequest(context.Background(), req, "https://api.example.com", "key")
		require.NoError(t, err)
		assert.Equal(t, "arn:aws:iam::123456:profile/test", httpReq.Header.Get("x-amz-profile-arn"))
	})

	t.Run("request body contains correct model ID", func(t *testing.T) {
		adapter := NewProviderAdapter()
		req := &canonical.Request{
			Model: "claude-3-5-sonnet",
			Messages: []canonical.Message{
				{Role: canonical.RoleUser, Content: []canonical.ContentBlock{{Type: canonical.ContentText, Text: "Hi"}}},
			},
		}

		httpReq, err := adapter.BuildRequest(context.Background(), req, "https://api.example.com", "key")
		require.NoError(t, err)

		// Read and verify body
		var kiroReq KiroRequest
		err = json.NewDecoder(httpReq.Body).Decode(&kiroReq)
		require.NoError(t, err)
		assert.Equal(t, "claude-sonnet-4.5", kiroReq.ConversationState.CurrentMessage.UserInputMessage.ModelID)
	})
}

// ── Provider IsRawStream Tests ──────────────────────────────────────────

func TestProviderAdapter_IsRawStream(t *testing.T) {
	adapter := NewProviderAdapter()
	assert.True(t, adapter.IsRawStream())
}

// ── Provider buildFinalResponse Tests ───────────────────────────────────

func TestProviderAdapter_BuildFinalResponse(t *testing.T) {
	t.Run("builds response with accumulated text", func(t *testing.T) {
		adapter := NewProviderAdapter()
		adapter.accumulatedText.WriteString("Hello World")

		resp := adapter.buildFinalResponse()
		require.NotNil(t, resp)
		assert.Equal(t, "chat.completion", resp.Object)
		require.Len(t, resp.Choices, 1)
		require.Len(t, resp.Choices[0].Message.Content, 1)
		assert.Equal(t, "Hello World", resp.Choices[0].Message.Content[0].Text)
		assert.Equal(t, "stop", *resp.Choices[0].FinishReason)
	})

	t.Run("builds response with thinking text", func(t *testing.T) {
		adapter := NewProviderAdapter()
		adapter.accumulatedText.WriteString("Answer")
		adapter.thinkingText.WriteString("Let me think...")

		resp := adapter.buildFinalResponse()
		require.NotNil(t, resp.Choices[0].Message.Reasoning)
		assert.Equal(t, "Let me think...", *resp.Choices[0].Message.Reasoning)
	})

	t.Run("builds response with usage", func(t *testing.T) {
		adapter := NewProviderAdapter()
		adapter.accumulatedText.WriteString("test")
		adapter.usage = &canonical.Usage{CompletionTokens: 42}

		resp := adapter.buildFinalResponse()
		require.NotNil(t, resp.Usage)
		assert.Equal(t, int64(42), resp.Usage.CompletionTokens)
	})

	t.Run("empty text produces empty content", func(t *testing.T) {
		adapter := NewProviderAdapter()
		resp := adapter.buildFinalResponse()
		assert.Nil(t, resp.Choices[0].Message.Content)
		assert.Equal(t, "stop", *resp.Choices[0].FinishReason)
	})
}

// ── getTextContent Tests ────────────────────────────────────────────────

func TestGetTextContent(t *testing.T) {
	t.Run("extracts text from multiple blocks", func(t *testing.T) {
		msg := canonical.Message{
			Content: []canonical.ContentBlock{
				{Type: canonical.ContentText, Text: "Hello"},
				{Type: canonical.ContentImage}, // Should be skipped
				{Type: canonical.ContentText, Text: "World"},
			},
		}
		result := getTextContent(msg)
		assert.Equal(t, "Hello\nWorld", result)
	})

	t.Run("skips empty text", func(t *testing.T) {
		msg := canonical.Message{
			Content: []canonical.ContentBlock{
				{Type: canonical.ContentText, Text: ""},
				{Type: canonical.ContentText, Text: "hello"},
			},
		}
		result := getTextContent(msg)
		assert.Equal(t, "hello", result)
	})

	t.Run("empty content returns empty string", func(t *testing.T) {
		msg := canonical.Message{}
		result := getTextContent(msg)
		assert.Equal(t, "", result)
	})
}

// ── processTools Tests ──────────────────────────────────────────────────

func TestProcessTools(t *testing.T) {
	t.Run("short description stays inline", func(t *testing.T) {
		tools := []canonical.Tool{
			{Name: "search", Description: "Quick search"},
		}
		kiroTools, sysPrompt := processTools(tools, "")
		require.Len(t, kiroTools, 1)
		spec := kiroTools[0]["toolSpecification"].(map[string]interface{})
		assert.Equal(t, "Quick search", spec["description"])
		assert.Empty(t, sysPrompt)
	})

	t.Run("long description moves to system prompt", func(t *testing.T) {
		longDesc := strings.Repeat("x", 201)
		tools := []canonical.Tool{
			{Name: "search", Description: longDesc},
		}
		kiroTools, sysPrompt := processTools(tools, "")
		spec := kiroTools[0]["toolSpecification"].(map[string]interface{})
		assert.Contains(t, spec["description"].(string), "Full documentation in system prompt")
		assert.Contains(t, sysPrompt, "## Tool: search")
		assert.Contains(t, sysPrompt, longDesc)
	})

	t.Run("empty description uses tool name", func(t *testing.T) {
		tools := []canonical.Tool{
			{Name: "my_tool", Description: ""},
		}
		kiroTools, _ := processTools(tools, "")
		spec := kiroTools[0]["toolSpecification"].(map[string]interface{})
		assert.Equal(t, "Tool: my_tool", spec["description"])
	})
}

// ── maxInt Tests ────────────────────────────────────────────────────────

func TestMaxInt(t *testing.T) {
	assert.Equal(t, 5, maxInt(3, 5))
	assert.Equal(t, 5, maxInt(5, 3))
	assert.Equal(t, 0, maxInt(0, 0))
	assert.Equal(t, 0, maxInt(-1, 0))
}
