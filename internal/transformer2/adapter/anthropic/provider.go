package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/bestruirui/octopus/internal/transformer2/canonical"
	"github.com/bestruirui/octopus/internal/transformer2/urlutil"
)

// ProviderAdapter implements ProviderAdapter for Anthropic backends.
type ProviderAdapter struct{}

// NewProviderAdapter creates a new ProviderAdapter.
func NewProviderAdapter() *ProviderAdapter {
	return &ProviderAdapter{}
}

// BuildRequest builds an HTTP request for Anthropic from canonical Request.
func (a *ProviderAdapter) BuildRequest(ctx context.Context, req *canonical.Request, baseURL, key string) (*http.Request, error) {
	areq := convertCanonicalToAnthropicRequest(req)

	body, err := json.Marshal(areq)
	if err != nil {
		return nil, err
	}

	// Merge ExtraBody if present (user intent takes precedence)
	if len(req.ExtraBody) > 0 {
		body, err = mergeExtraBody(body, req.ExtraBody)
		if err != nil {
			return nil, err
		}
	}

	// Route to correct endpoint based on request kind
	endpoint := "/messages"
	if req.Kind == canonical.KindCountTokens {
		endpoint = "/messages/count_tokens"
	}

	reqURL, err := urlutil.BuildURL(baseURL, endpoint)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, "POST", reqURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", key)
	httpReq.Header.Set("anthropic-version", getAnthropicVersion(req))

	// Forward and supplement anthropic-beta headers
	betaHeaders := getAnthropicBetaHeaders(req)
	if len(betaHeaders) > 0 {
		httpReq.Header.Set("anthropic-beta", strings.Join(betaHeaders, ","))
	}

	return httpReq, nil
}

// getAnthropicVersion returns the anthropic-version header value.
func getAnthropicVersion(req *canonical.Request) string {
	if req.Headers != nil {
		if v := req.Headers.Get("Anthropic-Version"); v != "" {
			return v
		}
	}
	return "2023-06-01"
}

// getAnthropicBetaHeaders returns the anthropic-beta headers to include.
func getAnthropicBetaHeaders(req *canonical.Request) []string {
	var headers []string

	// Forward existing headers
	if req.Headers != nil {
		if beta := req.Headers.Get("Anthropic-Beta"); beta != "" {
			headers = append(headers, strings.Split(beta, ",")...)
		}
	}

	// Auto-detect and append needed beta headers
	// Extended thinking (budget_tokens)
	if req.Reasoning != nil && req.Reasoning.BudgetTokens != nil {
		headers = appendIfMissing(headers, "extended-thinking")
	}

	// Server tools - check for server tool types in Tools
	for _, tool := range req.Tools {
		if len(tool.RawJSON) > 0 {
			var raw map[string]interface{}
			if json.Unmarshal(tool.RawJSON, &raw) == nil {
				if toolType, ok := raw["type"].(string); ok {
					switch toolType {
					case "bash_20250611":
						headers = appendIfMissing(headers, "bash_20250611")
					case "text_editor_20250429":
						headers = appendIfMissing(headers, "text_editor_20250429")
					case "code_execution_20250522":
						headers = appendIfMissing(headers, "code_execution_20250522")
					case "web_search_20250305":
						headers = appendIfMissing(headers, "web_search_20250305")
					case "web_fetch_20250305":
						headers = appendIfMissing(headers, "web_fetch_20250305")
					}
				}
			}
		}
	}

	// Cache control - check messages and tools
	// Only append for older Claude 3 models where caching isn't GA
	if hasCacheControl(req) && isClaude3Model(req.Model) {
		headers = appendIfMissing(headers, "prompt-caching-2024-07-31")
	}

	return headers
}

// appendIfMissing appends a value to a slice if not already present.
func appendIfMissing(slice []string, s string) []string {
	for _, v := range slice {
		if v == s {
			return slice
		}
	}
	return append(slice, s)
}

// hasCacheControl checks if any message or tool has cache_control.
func hasCacheControl(req *canonical.Request) bool {
	for _, msg := range req.Messages {
		for _, cb := range msg.Content {
			if cb.CacheControl != nil {
				return true
			}
		}
		if msg.CacheControl != nil {
			return true
		}
	}
	for _, tool := range req.Tools {
		if tool.CacheControl != nil {
			return true
		}
	}
	return false
}

// isClaude3Model checks if model is a Claude 3 model.
func isClaude3Model(model string) bool {
	return strings.Contains(model, "claude-3-opus") ||
		strings.Contains(model, "claude-3-sonnet") ||
		strings.Contains(model, "claude-3-haiku")
}

// convertCanonicalToAnthropicRequest converts canonical Request to Anthropic format.
func convertCanonicalToAnthropicRequest(req *canonical.Request) *MessageRequest {
	areq := &MessageRequest{
		Model:       req.Model,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		TopK:        req.TopK,
		Stream:      req.Stream,
	}

	// Handle max_tokens (required in Anthropic)
	if req.MaxTokens != nil {
		areq.MaxTokens = *req.MaxTokens
	} else {
		// Default to 4096 if not specified (safe for most Claude models)
		areq.MaxTokens = 4096
	}

	// Handle stop_sequences
	if req.Stop != nil {
		if req.Stop.Single != "" {
			areq.StopSequences = []string{req.Stop.Single}
		} else if len(req.Stop.Multiple) > 0 {
			areq.StopSequences = req.Stop.Multiple
		}
	}

	// Convert thinking config
	if req.Reasoning != nil {
		areq.Thinking = convertCanonicalToAnthropicThinking(req.Reasoning)

		// Emit output_config.effort for adaptive thinking mode
		if areq.Thinking != nil && areq.Thinking.Type == "adaptive" && req.Reasoning.Effort != nil {
			effort := mapCanonicalEffortToAnthropic(*req.Reasoning.Effort)
			if effort != "" {
				areq.OutputConfig = &AnthropicOutputConfig{Effort: effort}
			}
		}
	}

	// Extract system messages and regular messages
	system, messages := extractSystemAndMessages(req)
	areq.System = system
	areq.Messages = messages

	// Convert tools
	if len(req.Tools) > 0 {
		areq.Tools = make([]Tool, len(req.Tools))
		for i, tool := range req.Tools {
			areq.Tools[i] = convertCanonicalToAnthropicTool(tool)
		}
	}

	// Convert tool_choice
	if req.ToolChoice != nil {
		areq.ToolChoice = convertCanonicalToAnthropicToolChoice(req.ToolChoice)
	}

	return areq
}

// mapCanonicalEffortToAnthropic maps canonical reasoning effort to Anthropic output_config effort.
func mapCanonicalEffortToAnthropic(effort string) string {
	switch effort {
	case "minimal", "low":
		return "low"
	case "medium":
		return "medium"
	case "high":
		return "high"
	case "xhigh", "max":
		return "max"
	case "none":
		return "" // omit thinking entirely
	default:
		return effort // passthrough unknown values
	}
}

// extractSystemAndMessages extracts system prompt and aggregates tool results.
func extractSystemAndMessages(req *canonical.Request) (SystemContent, []MessageParam) {
	var system SystemContent
	var messages []MessageParam

	// First pass - extract system messages
	var systemBlocks []SystemBlock
	var hasCacheControl bool

	for _, msg := range req.Messages {
		if msg.Role == canonical.RoleSystem || msg.Role == canonical.RoleDeveloper {
			// Extract system content
			for _, cb := range msg.Content {
				systemBlocks = append(systemBlocks, SystemBlock{
					Type:         "text",
					Text:         cb.Text,
					CacheControl: convertCanonicalCacheControl(cb.CacheControl),
				})
				if cb.CacheControl != nil {
					hasCacheControl = true
				}
			}
		}
	}

	// Build system content - use string format for simple single text block
	// Use array format when there are multiple blocks or cache_control
	if len(systemBlocks) == 1 && !hasCacheControl {
		system.Text = systemBlocks[0].Text
	} else if len(systemBlocks) > 0 {
		system.Blocks = systemBlocks
	}

	// Second pass - aggregate tool results and build messages
	for i := 0; i < len(req.Messages); i++ {
		msg := req.Messages[i]

		// Skip system/developer messages
		if msg.Role == canonical.RoleSystem || msg.Role == canonical.RoleDeveloper {
			continue
		}

		// Handle tool result aggregation
		if msg.Role == canonical.RoleTool {
			// Collect consecutive tool results
			toolResults := []ContentBlock{convertToolResultToAnthropic(msg)}

			for i+1 < len(req.Messages) && req.Messages[i+1].Role == canonical.RoleTool {
				i++
				toolResults = append(toolResults, convertToolResultToAnthropic(req.Messages[i]))
			}

			// Create a single user message with all tool results
			messages = append(messages, MessageParam{
				Role:    "user",
				Content: MessageContent{Blocks: toolResults},
			})
			continue
		}

		// Regular message
		aparam := MessageParam{
			Role:    string(msg.Role),
			Content: convertCanonicalToAnthropicContent(msg.Content),
		}

		// Handle assistant messages with tool calls
		if msg.Role == canonical.RoleAssistant && len(msg.ToolCalls) > 0 {
			// Add tool_use blocks to content
			for _, tc := range msg.ToolCalls {
				aparam.Content.Blocks = append(aparam.Content.Blocks, ContentBlock{
					Type:  ContentTypeToolUse,
					ID:    tc.ID,
					Name:  tc.Name,
					Input: json.RawMessage(tc.Arguments),
				})
			}
		}

		messages = append(messages, aparam)
	}

	return system, messages
}

// convertToolResultToAnthropic converts a tool result message to Anthropic tool_result block.
func convertToolResultToAnthropic(msg canonical.Message) ContentBlock {
	block := ContentBlock{
		Type:      ContentTypeToolResult,
		IsError:   false,
	}

	// Guard against nil ToolCallID
	if msg.ToolCallID != nil {
		block.ToolUseID = *msg.ToolCallID
	} else {
		// Generate a placeholder when ToolCallID is missing
		block.ToolUseID = "missing_tool_call_id"
	}

	// Handle content
	if len(msg.Content) == 1 && msg.Content[0].Type == canonical.ContentText {
		block.Content = json.RawMessage(`"` + jsonEscape(msg.Content[0].Text) + `"`)
	} else if len(msg.Content) > 0 {
		// Multiple content blocks
		blocks := make([]ContentBlock, len(msg.Content))
		for i, cb := range msg.Content {
			blocks[i] = ContentBlock{Type: string(cb.Type)}
			switch cb.Type {
			case canonical.ContentText:
				blocks[i].Text = cb.Text
			case canonical.ContentImage:
				if cb.Media != nil {
					blocks[i].Source = &ImageSource{
						Type:      "base64",
						MediaType: cb.Media.MimeType,
						Data:      cb.Media.Base64,
					}
				}
			}
		}
		block.Content = marshalJSON(blocks)
	}

	if msg.ToolCallIsError != nil && *msg.ToolCallIsError {
		block.IsError = true
	}

	return block
}

// convertCanonicalCacheControl converts canonical CacheControl to Anthropic format.
func convertCanonicalCacheControl(cc *canonical.CacheControl) *CacheControl {
	if cc == nil {
		return nil
	}
	return &CacheControl{Type: cc.Type}
}

// jsonEscape escapes a string for JSON.
func jsonEscape(s string) string {
	var buf bytes.Buffer
	json.HTMLEscape(&buf, []byte(s))
	return buf.String()
}

// ParseResponse parses Anthropic HTTP response to canonical Response.
func (a *ProviderAdapter) ParseResponse(ctx context.Context, resp *http.Response) (*canonical.Response, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		var errResp ErrorResponse
		if json.Unmarshal(body, &errResp) == nil && errResp.Error.Message != "" {
			return &canonical.Response{
				StatusCode: resp.StatusCode,
				Error: &canonical.Error{
					Code:       errResp.Error.Type,
					Message:    errResp.Error.Message,
					Type:       errResp.Error.Type,
					StatusCode: resp.StatusCode,
				},
			}, nil
		}
		return &canonical.Response{
			StatusCode: resp.StatusCode,
			Error: &canonical.Error{
				Message:    string(body),
				StatusCode: resp.StatusCode,
			},
		}, nil
	}

	// Check if this is a count_tokens response
	// count_tokens returns {input_tokens: N} instead of a full message response
	var ctResp CountTokensResponse
	if json.Unmarshal(body, &ctResp) == nil && ctResp.InputTokens > 0 {
		// Check if it's actually a count_tokens response (no "id" field)
		var raw map[string]interface{}
		if json.Unmarshal(body, &raw) == nil {
			if _, hasID := raw["id"]; !hasID {
				return &canonical.Response{
					StatusCode: resp.StatusCode,
					Usage: &canonical.Usage{
						PromptTokens: ctResp.InputTokens,
						TotalTokens:  ctResp.InputTokens,
					},
				}, nil
			}
		}
	}

	var aresp MessageResponse
	if err := json.Unmarshal(body, &aresp); err != nil {
		return nil, err
	}

	return convertAnthropicResponseToCanonical(&aresp, resp.StatusCode), nil
}

// convertAnthropicResponseToCanonical converts Anthropic response to canonical format.
func convertAnthropicResponseToCanonical(resp *MessageResponse, statusCode int) *canonical.Response {
	creq := &canonical.Response{
		ID:         resp.ID,
		Model:      resp.Model,
		Object:     "chat.completion",
		StatusCode: statusCode,
	}

	// Convert content blocks
	if len(resp.Content) > 0 {
		msg := canonical.Message{
			Role:    canonical.RoleAssistant,
			Content: make([]canonical.ContentBlock, 0),
		}

		for _, block := range resp.Content {
			switch block.Type {
			case ContentTypeText:
				msg.Content = append(msg.Content, canonical.ContentBlock{
					Type: canonical.ContentText,
					Text: block.Text,
				})

			case ContentTypeToolUse:
				msg.ToolCalls = append(msg.ToolCalls, canonical.ToolCall{
					ID:        block.ID,
					Type:      "function",
					Name:      block.Name,
					Arguments: string(block.Input),
				})

			case ContentTypeThinking:
				msg.Reasoning = &block.Thinking
				msg.ReasoningSignature = &block.Signature
				msg.Content = append(msg.Content, canonical.ContentBlock{
					Type:      canonical.ContentThinking,
					Thinking:  block.Thinking,
					Signature: block.Signature,
				})
			}
		}

		creq.Choices = []canonical.Choice{
			{
				Index:   0,
				Message: msg,
			},
		}

		// Map stop_reason
		if resp.StopReason != "" {
			creq.Choices[0].FinishReason = mapAnthropicStopReasonToCanonical(resp.StopReason)
		}

		// Handle stop_sequence
		if resp.StopReason == StopReasonStopSequence {
			creq.Choices[0].StopSequence = &resp.StopSequence
		}
	}

	// Convert usage
	creq.Usage = convertAnthropicUsageToCanonical(resp.Usage)

	return creq
}

// ParseStreamChunk parses Anthropic SSE data to canonical Chunk.
func (a *ProviderAdapter) ParseStreamChunk(ctx context.Context, data []byte) (*canonical.Chunk, error) {
	var event StreamEvent
	if err := json.Unmarshal(data, &event); err != nil {
		return nil, err
	}

	switch event.Type {
	case EventTypeMessageStart:
		// Initial message with usage.input_tokens
		if event.Message != nil {
			return &canonical.Chunk{
				ID:    event.Message.ID,
				Model: event.Message.Model,
				Usage: convertAnthropicUsageToCanonical(event.Message.Usage),
			}, nil
		}

	case EventTypeContentBlockStart:
		// Start of content block
		if event.ContentBlock != nil {
			chunk := &canonical.Chunk{}
			// Handle tool_use block start
			if event.ContentBlock.Type == ContentTypeToolUse {
				chunk.Deltas = []canonical.ChoiceDelta{{
					Index: event.Index,
					Delta: canonical.Message{
						ToolCalls: []canonical.ToolCall{{
							Index: event.Index,
							ID:    event.ContentBlock.ID,
							Type:  "function",
							Name:  event.ContentBlock.Name,
						}},
					},
				}}
			}
			return chunk, nil
		}

	case EventTypeContentBlockDelta:
		// Content delta
		if event.Delta != nil {
			chunk := &canonical.Chunk{}

			switch event.Delta.Type {
			case "text_delta":
				chunk.Deltas = []canonical.ChoiceDelta{{
					Index: event.Index,
					Delta: canonical.Message{
						Content: []canonical.ContentBlock{{
							Type: canonical.ContentText,
							Text: event.Delta.Text,
						}},
					},
				}}

			case "input_json_delta":
				chunk.Deltas = []canonical.ChoiceDelta{{
					Index: event.Index,
					Delta: canonical.Message{
						ToolCalls: []canonical.ToolCall{{
							Index:     event.Index,
							Arguments: event.Delta.PartialJSON,
						}},
					},
				}}

			case "thinking_delta":
				chunk.Deltas = []canonical.ChoiceDelta{{
					Index: event.Index,
					Delta: canonical.Message{
						Reasoning: &event.Delta.Thinking,
					},
				}}

			case "signature_delta":
				// Signature delta - handled separately from thinking content
				// Could be added to ChoiceDelta if needed
			}

			return chunk, nil
		}

	case EventTypeContentBlockStop:
		// End of content block - no content needed
		return &canonical.Chunk{}, nil

	case EventTypeMessageDelta:
		// Message-level delta with stop_reason and output_tokens
		chunk := &canonical.Chunk{}

		if event.DeltaMessage != nil {
			chunk.Deltas = []canonical.ChoiceDelta{{
				Index: 0,
			}}
			if event.DeltaMessage.StopReason != "" {
				chunk.Deltas[0].FinishReason = mapAnthropicStopReasonToCanonical(event.DeltaMessage.StopReason)
			}
		}

		if event.Usage != nil {
			chunk.Usage = &canonical.Usage{
				CompletionTokens: event.Usage.OutputTokens,
			}
		}

		return chunk, nil

	case EventTypeMessageStop:
		// End of stream
		return &canonical.Chunk{Done: true}, nil

	case EventTypePing:
		// Ping event - ignore
		return &canonical.Chunk{}, nil

	case EventTypeError:
		// Error event
		return &canonical.Chunk{
			Done: true,
		}, nil
	}

	return &canonical.Chunk{}, nil
}
