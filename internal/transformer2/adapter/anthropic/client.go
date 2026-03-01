package anthropic

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/bestruirui/octopus/internal/transformer2/canonical"
)

// ClientAdapter implements ClientAdapter for Anthropic Messages API.
type ClientAdapter struct {
	aggregator *streamAggregator
}

// streamAggregator accumulates Anthropic stream chunks.
type streamAggregator struct {
	chunks []*canonical.Chunk
	usage  *canonical.Usage
}

func newStreamAggregator() *streamAggregator {
	return &streamAggregator{
		chunks: make([]*canonical.Chunk, 0),
	}
}

func (a *streamAggregator) addChunk(c *canonical.Chunk) {
	a.chunks = append(a.chunks, c)
}

func (a *streamAggregator) accumulateUsage(usage *canonical.Usage) {
	if usage == nil {
		return
	}
	if a.usage == nil {
		a.usage = &canonical.Usage{}
	}
	// Accumulate usage (Anthropic sends input_tokens in message_start, output_tokens in message_delta)
	a.usage.PromptTokens += usage.PromptTokens
	a.usage.CompletionTokens += usage.CompletionTokens
	a.usage.TotalTokens += usage.TotalTokens
	if usage.CacheCreationInputTokens > 0 {
		a.usage.CacheCreationInputTokens += usage.CacheCreationInputTokens
	}
	if usage.CacheReadInputTokens > 0 {
		a.usage.CacheReadInputTokens += usage.CacheReadInputTokens
	}
	if usage.InputTokensAfterBreakpoint > 0 {
		a.usage.InputTokensAfterBreakpoint += usage.InputTokensAfterBreakpoint
	}
}

// NewClientAdapter creates a new ClientAdapter.
func NewClientAdapter() *ClientAdapter {
	return &ClientAdapter{
		aggregator: newStreamAggregator(),
	}
}

// ParseRequest converts Anthropic client request body to canonical Request.
func (a *ClientAdapter) ParseRequest(ctx context.Context, body []byte, header http.Header) (*canonical.Request, error) {
	var req MessageRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}

	creq := &canonical.Request{
		Kind:         canonical.KindChat,
		Model:        req.Model,
		Temperature:  req.Temperature,
		TopP:         req.TopP,
		TopK:         req.TopK,
		Stream:       req.Stream,
		SourceFormat: canonical.FormatAnthropic,
		Headers:      header,
		RawRequest:   body,
	}

	// Handle max_tokens
	if req.MaxTokens > 0 {
		creq.MaxTokens = &req.MaxTokens
	}

	// Handle stop_sequences
	if len(req.StopSequences) > 0 {
		if len(req.StopSequences) == 1 {
			creq.Stop = &canonical.StopSequences{Single: req.StopSequences[0]}
		} else {
			creq.Stop = &canonical.StopSequences{Multiple: req.StopSequences}
		}
	}

	// Handle thinking config
	if req.Thinking != nil {
		creq.Reasoning = convertAnthropicThinkingToCanonical(req.Thinking)
	}

	// Extract anthropic-beta and anthropic-version headers
	if header != nil {
		if beta := header.Get("anthropic-beta"); beta != "" {
			if creq.Headers == nil {
				creq.Headers = make(http.Header)
			}
			creq.Headers.Set("Anthropic-Beta", beta)
		}
		if version := header.Get("anthropic-version"); version != "" {
			if creq.Headers == nil {
				creq.Headers = make(http.Header)
			}
			creq.Headers.Set("Anthropic-Version", version)
		}
	}

	// Handle system prompt
	var systemMessages []canonical.Message
	if req.System.Text != "" || len(req.System.Blocks) > 0 {
		text, blocks, isArray := convertAnthropicSystemToCanonical(req.System)
		sysMsg := canonical.Message{Role: canonical.RoleSystem}
		if text != "" {
			sysMsg.Content = []canonical.ContentBlock{{Type: canonical.ContentText, Text: text}}
		} else if len(blocks) > 0 {
			sysMsg.Content = blocks
		}
		systemMessages = append(systemMessages, sysMsg)
		// Track array format for round-trip
		if isArray {
			creq.Hints.AnthropicSystemArrayFormat = true
		}
	}

	// Convert messages
	messages := make([]canonical.Message, 0, len(req.Messages))
	for _, msg := range req.Messages {
		cmsg := convertAnthropicMessageToCanonicalForClient(msg)
		messages = append(messages, cmsg)
	}

	// Combine system + regular messages
	creq.Messages = append(systemMessages, messages...)

	// Convert tools
	if len(req.Tools) > 0 {
		creq.Tools = make([]canonical.Tool, len(req.Tools))
		for i, tool := range req.Tools {
			creq.Tools[i] = convertAnthropicToolToCanonical(tool)
		}
	}

	// Convert tool_choice
	if req.ToolChoice != nil {
		creq.ToolChoice = convertAnthropicToolChoiceToCanonical(req.ToolChoice)
	}

	return creq, nil
}

// convertAnthropicMessageToCanonicalForClient converts an Anthropic message for client parsing.
// Handles tool_result blocks specially.
func convertAnthropicMessageToCanonicalForClient(msg MessageParam) canonical.Message {
	cmsg := canonical.Message{
		Role: canonical.Role(msg.Role),
	}

	// Handle content
	if msg.Content.Text != "" {
		cmsg.Content = []canonical.ContentBlock{
			{Type: canonical.ContentText, Text: msg.Content.Text},
		}
	} else if len(msg.Content.Blocks) > 0 {
		cmsg.Content = make([]canonical.ContentBlock, 0, len(msg.Content.Blocks))
		var toolCalls []canonical.ToolCall
		var toolCallID *string
		var toolCallName *string
		var toolCallIsError *bool

		for _, block := range msg.Content.Blocks {
			switch block.Type {
			case ContentTypeText:
				cmsg.Content = append(cmsg.Content, canonical.ContentBlock{
					Type: canonical.ContentText,
					Text: block.Text,
				})

			case ContentTypeImage:
				cb := canonical.ContentBlock{Type: canonical.ContentImage}
				if block.Source != nil {
					cb.Media = &canonical.MediaContent{
						MimeType: block.Source.MediaType,
						Base64:   block.Source.Data,
						URL:      block.Source.URL,
					}
				}
				cmsg.Content = append(cmsg.Content, cb)

			case ContentTypeToolUse:
				// Tool call in assistant message
				tc := canonical.ToolCall{
					ID:   block.ID,
					Type: "function",
					Name: block.Name,
				}
				if len(block.Input) > 0 {
					tc.Arguments = string(block.Input)
				}
				toolCalls = append(toolCalls, tc)

			case ContentTypeToolResult:
				// Tool result - set RoleTool and extract tool_use_id
				cmsg.Role = canonical.RoleTool
				toolCallID = &block.ToolUseID
				isError := block.IsError
				toolCallIsError = &isError
				// Content for tool result
				if block.Content != nil {
					var contentStr string
					if err := json.Unmarshal(block.Content, &contentStr); err == nil {
						cmsg.Content = append(cmsg.Content, canonical.ContentBlock{
							Type: canonical.ContentText,
							Text: contentStr,
						})
					} else {
						// Array content
						var contentBlocks []ContentBlock
						if err := json.Unmarshal(block.Content, &contentBlocks); err == nil {
							for _, cb := range contentBlocks {
								if cb.Type == ContentTypeText {
									cmsg.Content = append(cmsg.Content, canonical.ContentBlock{
										Type: canonical.ContentText,
										Text: cb.Text,
									})
								}
							}
						}
					}
				}

			case ContentTypeThinking:
				cmsg.Content = append(cmsg.Content, canonical.ContentBlock{
					Type:      canonical.ContentThinking,
					Thinking: block.Thinking,
					Signature: block.Signature,
				})
			}
		}

		if len(toolCalls) > 0 {
			cmsg.ToolCalls = toolCalls
		}
		if toolCallID != nil {
			cmsg.ToolCallID = toolCallID
		}
		if toolCallName != nil {
			cmsg.ToolCallName = toolCallName
		}
		if toolCallIsError != nil {
			cmsg.ToolCallIsError = toolCallIsError
		}
	}

	return cmsg
}

// FormatResponse converts canonical Response to Anthropic format.
func (a *ClientAdapter) FormatResponse(ctx context.Context, resp *canonical.Response) ([]byte, error) {
	aresp := &MessageResponse{
		ID:      resp.ID,
		Type:    "message",
		Role:    "assistant",
		Model:   resp.Model,
		Content: []ContentBlock{},
	}

	// Convert choices
	if len(resp.Choices) > 0 {
		choice := resp.Choices[0]
		aresp.Content = convertCanonicalMessageToAnthropicContent(choice.Message)

		// Map finish reason
		if choice.FinishReason != nil {
			aresp.StopReason = mapCanonicalFinishReasonToAnthropic(*choice.FinishReason)
		}

		// Handle stop sequence
		if choice.StopSequence != nil {
			aresp.StopReason = StopReasonStopSequence
			aresp.StopSequence = *choice.StopSequence
		}
	}

	// Convert usage
	if resp.Usage != nil {
		aresp.Usage = Usage{
			InputTokens:              resp.Usage.PromptTokens,
			OutputTokens:             resp.Usage.CompletionTokens,
			CacheCreationInputTokens: resp.Usage.CacheCreationInputTokens,
			CacheReadInputTokens:     resp.Usage.CacheReadInputTokens,
		}
	}

	return json.Marshal(aresp)
}

// convertCanonicalMessageToAnthropicContent converts canonical Message to Anthropic content blocks.
func convertCanonicalMessageToAnthropicContent(msg canonical.Message) []ContentBlock {
	blocks := make([]ContentBlock, 0)

	// Convert content blocks
	for _, cb := range msg.Content {
		block := ContentBlock{}

		switch cb.Type {
		case canonical.ContentText:
			block.Type = ContentTypeText
			block.Text = cb.Text

		case canonical.ContentImage:
			block.Type = ContentTypeImage
			if cb.Media != nil {
				block.Source = &ImageSource{
					MediaType: cb.Media.MimeType,
					Data:      cb.Media.Base64,
					URL:       cb.Media.URL,
				}
				if cb.Media.Base64 != "" {
					block.Source.Type = "base64"
				} else if cb.Media.URL != "" {
					block.Source.Type = "url"
				}
			}

		case canonical.ContentThinking:
			block.Type = ContentTypeThinking
			block.Thinking = cb.Thinking
			block.Signature = cb.Signature
		}

		blocks = append(blocks, block)
	}

	// Convert tool calls
	for _, tc := range msg.ToolCalls {
		blocks = append(blocks, ContentBlock{
			Type:  ContentTypeToolUse,
			ID:    tc.ID,
			Name:  tc.Name,
			Input: json.RawMessage(tc.Arguments),
		})
	}

	return blocks
}

// FormatStreamChunk converts canonical Chunk to Anthropic SSE format.
func (a *ClientAdapter) FormatStreamChunk(ctx context.Context, chunk *canonical.Chunk) ([]byte, error) {
	// For Anthropic, we format chunks as SSE events
	if chunk.Done {
		// Final chunk - format as message_stop
		event := StreamEvent{Type: EventTypeMessageStop}
		data, _ := json.Marshal(event)
		return formatSSE(EventTypeMessageStop, data), nil
	}

	// Format based on content
	if len(chunk.Deltas) > 0 {
		// Content delta
		delta := chunk.Deltas[0]
		if len(delta.Delta.Content) > 0 {
			// Text content delta
			for _, cb := range delta.Delta.Content {
				if cb.Type == canonical.ContentText {
					event := StreamEvent{
						Type:  EventTypeContentBlockDelta,
						Index: delta.Index,
						Delta: &ContentDelta{
							Type: "text_delta",
							Text: cb.Text,
						},
					}
					data, _ := json.Marshal(event)
					return formatSSE(EventTypeContentBlockDelta, data), nil
				}
			}
		}

		// Tool call delta
		for _, tc := range delta.Delta.ToolCalls {
			event := StreamEvent{
				Type:  EventTypeContentBlockDelta,
				Index: delta.Index,
				Delta: &ContentDelta{
					Type:       "input_json_delta",
					PartialJSON: tc.Arguments,
				},
			}
			data, _ := json.Marshal(event)
			return formatSSE(EventTypeContentBlockDelta, data), nil
		}

		// Finish reason
		if delta.FinishReason != nil {
			stopReason := mapCanonicalFinishReasonToAnthropic(*delta.FinishReason)
			event := StreamEvent{
				Type: EventTypeMessageDelta,
				DeltaMessage: &MessageDeltaRaw{
					StopReason: stopReason,
				},
			}
			data, _ := json.Marshal(event)
			return formatSSE(EventTypeMessageDelta, data), nil
		}
	}

	// Usage chunk
	if chunk.Usage != nil {
		event := StreamEvent{
			Type: EventTypeMessageDelta,
			Usage: &Usage{
				InputTokens:  chunk.Usage.PromptTokens,
				OutputTokens: chunk.Usage.CompletionTokens,
			},
		}
		data, _ := json.Marshal(event)
		return formatSSE(EventTypeMessageDelta, data), nil
	}

	// Empty chunk
	return nil, nil
}

// formatSSE formats data as SSE event.
func formatSSE(eventType string, data []byte) []byte {
	return []byte("event: " + eventType + "\ndata: " + string(data) + "\n\n")
}

// AggregateStream aggregates all collected chunks into a full response.
func (a *ClientAdapter) AggregateStream(ctx context.Context) (*canonical.Response, error) {
	if len(a.aggregator.chunks) == 0 {
		return &canonical.Response{}, nil
	}

	// Build response from chunks
	resp := &canonical.Response{
		ID:     a.aggregator.chunks[0].ID,
		Model:  a.aggregator.chunks[0].Model,
		Object: "chat.completion",
	}

	// Aggregate content by choice index
	choiceDeltas := make(map[int]*canonical.Message)
	var finishReason *string

	for _, chunk := range a.aggregator.chunks {
		// Accumulate usage
		a.aggregator.accumulateUsage(chunk.Usage)

		// Process deltas
		for _, delta := range chunk.Deltas {
			if _, ok := choiceDeltas[delta.Index]; !ok {
				choiceDeltas[delta.Index] = &canonical.Message{
					Role:    delta.Delta.Role,
					Content: make([]canonical.ContentBlock, 0),
				}
			}

			msg := choiceDeltas[delta.Index]

			// Concatenate text content
			for _, cb := range delta.Delta.Content {
				if cb.Type == canonical.ContentText {
					// Find or append text block
					found := false
					for i := range msg.Content {
						if msg.Content[i].Type == canonical.ContentText {
							msg.Content[i].Text += cb.Text
							found = true
							break
						}
					}
					if !found {
						msg.Content = append(msg.Content, cb)
					}
				} else {
					msg.Content = append(msg.Content, cb)
				}
			}

			// Aggregate tool calls by index
			for _, tc := range delta.Delta.ToolCalls {
				found := false
				for i := range msg.ToolCalls {
					if msg.ToolCalls[i].Index == tc.Index {
						msg.ToolCalls[i].Arguments += tc.Arguments
						found = true
						break
					}
				}
				if !found {
					msg.ToolCalls = append(msg.ToolCalls, tc)
				}
			}

			// Track finish reason
			if delta.FinishReason != nil {
				finishReason = delta.FinishReason
			}
		}
	}

	// Build choices
	for idx, msg := range choiceDeltas {
		resp.Choices = append(resp.Choices, canonical.Choice{
			Index:        idx,
			Message:      *msg,
			FinishReason: finishReason,
		})
	}

	// Set usage
	if a.aggregator.usage != nil {
		resp.Usage = a.aggregator.usage
	}

	return resp, nil
}

// FormatError converts canonical Error to Anthropic error format.
func (a *ClientAdapter) FormatError(ctx context.Context, err *canonical.Error) ([]byte, error) {
	aerr := ErrorResponse{
		Type: "error",
		Error: ErrorDetail{
			Type:    err.Type,
			Message: err.Message,
		},
	}
	return json.Marshal(aerr)
}