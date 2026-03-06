package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"

	"github.com/bestruirui/octopus/internal/transformer2/canonical"
)

// ClientAdapter implements ClientAdapter for Anthropic Messages API.
type ClientAdapter struct {
	aggregator *streamAggregator

	// Stream state for FormatStreamChunk
	hasStarted                bool
	hasTextContentStarted     bool
	hasThinkingContentStarted bool
	hasToolContentStarted     bool
	hasFinished               bool
	messageStopped            bool
	contentIndex              int
	stopReason                *string
	toolCallIndices           map[int]bool
	messageID                 string
	modelName                 string
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
		creq.Reasoning = convertAnthropicThinkingToCanonical(req.Thinking, req.OutputConfig)
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
	if req.System.Text != "" {
		// String format - single system message
		systemMessages = append(systemMessages, canonical.Message{
			Role:    canonical.RoleSystem,
			Content: []canonical.ContentBlock{{Type: canonical.ContentText, Text: req.System.Text}},
		})
	} else if len(req.System.Blocks) > 0 {
		// Array format - combine all blocks into a single system message
		// This preserves cache_control per block
		blocks := make([]canonical.ContentBlock, len(req.System.Blocks))
		for i, block := range req.System.Blocks {
			blocks[i] = canonical.ContentBlock{
				Type: canonical.ContentText,
				Text: block.Text,
			}
			if block.CacheControl != nil {
				blocks[i].CacheControl = &canonical.CacheControl{Type: block.CacheControl.Type}
			}
		}
		systemMessages = append(systemMessages, canonical.Message{
			Role:    canonical.RoleSystem,
			Content: blocks,
		})
		// Track array format for round-trip
		creq.Hints.AnthropicSystemArrayFormat = true
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
					Text: derefStr(block.Text),
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
					// Compact the JSON to remove whitespace
					var compacted bytes.Buffer
					if err := json.Compact(&compacted, block.Input); err == nil {
						tc.Arguments = compacted.String()
					} else {
						tc.Arguments = string(block.Input)
					}
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
										Text: derefStr(cb.Text),
									})
								}
							}
						}
					}
				}

			case ContentTypeThinking:
				// For assistant messages, set reasoning_content field
				// AND add to Content for interleaved thinking support
				cmsg.Reasoning = block.Thinking
				cmsg.ReasoningSignature = block.Signature
				cmsg.Content = append(cmsg.Content, canonical.ContentBlock{
					Type:      canonical.ContentThinking,
					Thinking:  derefStr(block.Thinking),
					Signature: derefStr(block.Signature),
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

	// Handle top-level Reasoning field (if not already in Content)
	if msg.Reasoning != nil && *msg.Reasoning != "" {
		// Check if ContentThinking is already in Content
		hasThinkingBlock := false
		for _, cb := range msg.Content {
			if cb.Type == canonical.ContentThinking {
				hasThinkingBlock = true
				break
			}
		}
		if !hasThinkingBlock {
			blocks = append(blocks, ContentBlock{
				Type:      ContentTypeThinking,
				Thinking:  msg.Reasoning,
				Signature: strPtr(""), // No signature available from Reasoning field
			})
		}
	}

	// Convert content blocks
	for _, cb := range msg.Content {
		block := ContentBlock{}

		switch cb.Type {
		case canonical.ContentText:
			block.Type = ContentTypeText
			block.Text = strPtr(cb.Text)

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
			block.Thinking = strPtr(cb.Thinking)
			block.Signature = strPtr(cb.Signature)
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
// This is a stateful function that tracks stream state to emit the complete
// Anthropic SSE event lifecycle:
//
//	message_start → content_block_start → content_block_delta* →
//	content_block_stop → message_delta → message_stop
func (a *ClientAdapter) FormatStreamChunk(ctx context.Context, chunk *canonical.Chunk) ([]byte, error) {
	// Store chunk for aggregation
	a.aggregator.addChunk(chunk)

	var events [][]byte

	// Cache ID and model from chunks
	if a.messageID == "" && chunk.ID != "" {
		a.messageID = chunk.ID
	}
	if a.modelName == "" && chunk.Model != "" {
		a.modelName = chunk.Model
	}

	// Emit message_start on the first chunk
	if !a.hasStarted {
		a.hasStarted = true

		usage := &Usage{}
		if chunk.Usage != nil {
			usage = &Usage{
				InputTokens:  chunk.Usage.PromptTokens,
				OutputTokens: chunk.Usage.CompletionTokens,
			}
		}

		startEvent := StreamEvent{
			Type: EventTypeMessageStart,
			Message: &MessageResponse{
				ID:      a.messageID,
				Type:    "message",
				Role:    "assistant",
				Model:   a.modelName,
				Content: []ContentBlock{},
				Usage:   *usage,
			},
		}
		data, err := json.Marshal(startEvent)
		if err != nil {
			return nil, err
		}
		events = append(events, formatSSE(EventTypeMessageStart, data))
	}

	// Process deltas
	if len(chunk.Deltas) > 0 {
		delta := chunk.Deltas[0]

		// Handle thinking/reasoning content
		if delta.Delta.Reasoning != nil && *delta.Delta.Reasoning != "" {
			// Close tool block if open
			if a.hasToolContentStarted {
				a.hasToolContentStarted = false
				events = append(events, a.emitContentBlockStop())
				a.contentIndex++
			}

			// Emit content_block_start for thinking if not started
			if !a.hasThinkingContentStarted {
				a.hasThinkingContentStarted = true
				events = append(events, a.emitContentBlockStart(&ContentBlock{
					Type:      ContentTypeThinking,
					Thinking:  strPtr(""),
					Signature: strPtr(""),
				}))
			}

			// Emit thinking delta
			deltaEvent := StreamEvent{
				Type:  EventTypeContentBlockDelta,
				Index: a.contentIndex,
				Delta: &ContentDelta{
					Type:     "thinking_delta",
					Thinking: *delta.Delta.Reasoning,
				},
			}
			data, _ := json.Marshal(deltaEvent)
			events = append(events, formatSSE(EventTypeContentBlockDelta, data))
		}

		// Handle reasoning signature
		if delta.Delta.ReasoningSignature != nil && *delta.Delta.ReasoningSignature != "" {
			sigEvent := StreamEvent{
				Type:  EventTypeContentBlockDelta,
				Index: a.contentIndex,
				Delta: &ContentDelta{
					Type:      "signature_delta",
					Signature: *delta.Delta.ReasoningSignature,
				},
			}
			data, _ := json.Marshal(sigEvent)
			events = append(events, formatSSE(EventTypeContentBlockDelta, data))
		}

		// Handle text content
		if len(delta.Delta.Content) > 0 {
			for _, cb := range delta.Delta.Content {
				if cb.Type == canonical.ContentText && cb.Text != "" {
					// Close thinking block if open
					if a.hasThinkingContentStarted {
						a.hasThinkingContentStarted = false
						events = append(events, a.emitContentBlockStop())
						a.contentIndex++
					}
					// Close tool block if open
					if a.hasToolContentStarted {
						a.hasToolContentStarted = false
						events = append(events, a.emitContentBlockStop())
						a.contentIndex++
					}

					// Emit content_block_start for text if not started
					if !a.hasTextContentStarted {
						a.hasTextContentStarted = true
						events = append(events, a.emitContentBlockStart(&ContentBlock{
							Type: ContentTypeText,
							Text: strPtr(""),
						}))
					}

					// Emit text delta
					deltaEvent := StreamEvent{
						Type:  EventTypeContentBlockDelta,
						Index: a.contentIndex,
						Delta: &ContentDelta{
							Type: "text_delta",
							Text: cb.Text,
						},
					}
					data, _ := json.Marshal(deltaEvent)
					events = append(events, formatSSE(EventTypeContentBlockDelta, data))
				}
			}
		}

		// Handle tool calls
		if len(delta.Delta.ToolCalls) > 0 {
			// Close thinking block if open
			if a.hasThinkingContentStarted {
				a.hasThinkingContentStarted = false
				events = append(events, a.emitContentBlockStop())
				a.contentIndex++
			}
			// Close text block if open
			if a.hasTextContentStarted {
				a.hasTextContentStarted = false
				events = append(events, a.emitContentBlockStop())
				a.contentIndex++
			}

			if a.toolCallIndices == nil {
				a.toolCallIndices = make(map[int]bool)
			}

			for _, tc := range delta.Delta.ToolCalls {
				toolCallIndex := tc.Index

				if !a.toolCallIndices[toolCallIndex] {
					// Close previous tool block if starting a new one
					if toolCallIndex > 0 && a.hasToolContentStarted {
						events = append(events, a.emitContentBlockStop())
						a.contentIndex++
					}

					a.toolCallIndices[toolCallIndex] = true
					a.hasToolContentStarted = true

					// Emit content_block_start for tool_use
					events = append(events, a.emitContentBlockStart(&ContentBlock{
						Type:  ContentTypeToolUse,
						ID:    tc.ID,
						Name:  tc.Name,
						Input: json.RawMessage("{}"),
					}))

					// Emit initial arguments delta if present
					if tc.Arguments != "" {
						argEvent := StreamEvent{
							Type:  EventTypeContentBlockDelta,
							Index: a.contentIndex,
							Delta: &ContentDelta{
								Type:        "input_json_delta",
								PartialJSON: tc.Arguments,
							},
						}
						data, _ := json.Marshal(argEvent)
						events = append(events, formatSSE(EventTypeContentBlockDelta, data))
					}
				} else {
					// Continuing tool call - emit input_json_delta
					argEvent := StreamEvent{
						Type:  EventTypeContentBlockDelta,
						Index: a.contentIndex,
						Delta: &ContentDelta{
							Type:        "input_json_delta",
							PartialJSON: tc.Arguments,
						},
					}
					data, _ := json.Marshal(argEvent)
					events = append(events, formatSSE(EventTypeContentBlockDelta, data))
				}
			}
		}

		// Handle finish reason
		if delta.FinishReason != nil && !a.hasFinished {
			a.hasFinished = true

			// Close any open content block
			events = append(events, a.emitContentBlockStop())

			// Store stop reason for message_delta
			stopReason := mapCanonicalFinishReasonToAnthropic(*delta.FinishReason)
			a.stopReason = &stopReason
		}
	}

	// When hasFinished and not yet stopped, emit message_delta + message_stop.
	// We emit immediately rather than waiting for a separate usage chunk,
	// because some providers (especially OpenAI-compatible) may not send
	// a usage-only chunk — they send finish_reason and then [DONE].
	// If usage is available (same chunk or separate), include it.
	if a.hasFinished && !a.messageStopped {
		a.messageStopped = true

		// Build usage for message_delta (may be zero if no usage available)
		usage := &Usage{}
		if chunk.Usage != nil {
			usage = &Usage{
				InputTokens:  chunk.Usage.PromptTokens,
				OutputTokens: chunk.Usage.CompletionTokens,
			}
		}

		// Emit message_delta with stop_reason and usage
		msgDeltaEvent := StreamEvent{
			Type:  EventTypeMessageDelta,
			Usage: usage,
		}
		if a.stopReason != nil {
			msgDeltaEvent.DeltaMessage = &MessageDeltaRaw{
				StopReason: *a.stopReason,
			}
		}
		data, _ := json.Marshal(msgDeltaEvent)
		events = append(events, formatSSE(EventTypeMessageDelta, data))

		// Emit message_stop
		msgStopEvent := StreamEvent{Type: EventTypeMessageStop}
		data, _ = json.Marshal(msgStopEvent)
		events = append(events, formatSSE(EventTypeMessageStop, data))
	}

	// If no events were generated, check if we have a finish without usage
	// (some providers send finish_reason and usage in the same chunk)
	if len(events) == 0 {
		return nil, nil
	}

	// Concatenate all events
	result := make([]byte, 0)
	for _, event := range events {
		result = append(result, event...)
	}

	return result, nil
}

// emitContentBlockStart generates a content_block_start SSE event.
func (a *ClientAdapter) emitContentBlockStart(block *ContentBlock) []byte {
	event := StreamEvent{
		Type:         EventTypeContentBlockStart,
		Index:        a.contentIndex,
		ContentBlock: block,
	}
	data, _ := json.Marshal(event)
	return formatSSE(EventTypeContentBlockStart, data)
}

// emitContentBlockStop generates a content_block_stop SSE event.
func (a *ClientAdapter) emitContentBlockStop() []byte {
	event := StreamEvent{
		Type:  EventTypeContentBlockStop,
		Index: a.contentIndex,
	}
	data, _ := json.Marshal(event)
	return formatSSE(EventTypeContentBlockStop, data)
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
