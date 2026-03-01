package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"

	"github.com/bestruirui/octopus/internal/transformer2/canonical"
)

// ResponsesClientAdapter implements ClientAdapter for OpenAI Responses API.
type ResponsesClientAdapter struct {
	mu sync.Mutex

	// Stream state tracking
	hasResponseCreated   bool
	hasMessageItemStarted bool
	hasContentPartStarted bool
	hasReasoningItemStarted bool
	currentToolCallIndex  int
	sequenceNumber        int

	// Aggregator for stream chunks
	aggregator *responsesStreamAggregator
}

// responsesStreamAggregator accumulates stream chunks for Responses API.
type responsesStreamAggregator struct {
	chunks []*canonical.Chunk
	full   *canonical.Response
	mu     sync.Mutex
}

func newResponsesStreamAggregator() *responsesStreamAggregator {
	return &responsesStreamAggregator{
		chunks: make([]*canonical.Chunk, 0),
	}
}

func (a *responsesStreamAggregator) addChunk(c *canonical.Chunk) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.chunks = append(a.chunks, c)
}

func (a *responsesStreamAggregator) setFull(r *canonical.Response) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.full = r
}

func (a *responsesStreamAggregator) aggregate() (*canonical.Response, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.full != nil {
		return a.full, nil
	}

	if len(a.chunks) == 0 {
		return &canonical.Response{}, nil
	}

	// Build response from chunks
	resp := &canonical.Response{
		ID:      a.chunks[0].ID,
		Model:   a.chunks[0].Model,
		Created: a.chunks[0].Created,
		Object:  "response",
	}

	// Aggregate by choice index (similar to Chat adapter)
	choiceDeltas := make(map[int]*canonical.Message)
	var finishReason *string
	var lastUsage *canonical.Usage

	for _, chunk := range a.chunks {
		if chunk.Usage != nil {
			lastUsage = chunk.Usage
		}

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
						if tc.ID != "" {
							msg.ToolCalls[i].ID = tc.ID
						}
						if tc.Name != "" {
							msg.ToolCalls[i].Name = tc.Name
						}
						found = true
						break
					}
				}
				if !found {
					msg.ToolCalls = append(msg.ToolCalls, tc)
				}
			}

			// Aggregate reasoning
			if delta.Delta.Reasoning != nil {
				if msg.Reasoning == nil {
					msg.Reasoning = new(string)
				}
				*msg.Reasoning += *delta.Delta.Reasoning
			}

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

	if lastUsage != nil {
		resp.Usage = lastUsage
	}

	return resp, nil
}

// NewResponsesClientAdapter creates a new ResponsesClientAdapter.
func NewResponsesClientAdapter() *ResponsesClientAdapter {
	return &ResponsesClientAdapter{
		aggregator: newResponsesStreamAggregator(),
	}
}

// ParseRequest converts OpenAI Responses API request to canonical Request.
func (a *ResponsesClientAdapter) ParseRequest(ctx context.Context, body []byte, header http.Header) (*canonical.Request, error) {
	var req ResponsesRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}

	creq := &canonical.Request{
		Kind:             canonical.KindChat,
		Model:            req.Model,
		MaxCompletionTokens: req.MaxOutputTokens,
		Temperature:      req.Temperature,
		TopP:             req.TopP,
		Store:            req.Store,
		ServiceTier:      req.ServiceTier,
		User:             req.User,
		SourceFormat:     canonical.FormatOpenAIResponse,
		Headers:          header,
		RawRequest:       body,
	}

	// Handle stream
	if req.Stream != nil {
		creq.Stream = *req.Stream
	}

	// Parse input
	creq.Messages = parseResponsesInputToMessages(req.Input, req.Instructions)

	// Set hints for array input format
	if len(req.Input.Items) > 0 {
		trueVal := true
		creq.Hints.ResponsesArrayInput = &trueVal
	}

	// Parse include
	if len(req.Include) > 0 {
		creq.Hints.Include = req.Include
	}

	// Parse tools
	if len(req.Tools) > 0 {
		creq.Tools = make([]canonical.Tool, len(req.Tools))
		for i, tool := range req.Tools {
			creq.Tools[i] = parseResponsesToolToCanonical(tool)
		}
	}

	// Parse tool_choice
	if req.ToolChoice != nil {
		creq.ToolChoice = parseResponsesToolChoiceToCanonical(req.ToolChoice)
	}

	// Parse parallel_tool_calls
	if req.ParallelToolCalls != nil {
		disabled := !*req.ParallelToolCalls
		if creq.ToolChoice == nil {
			creq.ToolChoice = &canonical.ToolChoice{Mode: "auto"}
		}
		creq.ToolChoice.DisableParallelToolUse = &disabled
	}

	// Parse reasoning
	if req.Reasoning != nil && req.Reasoning.Effort != "" {
		creq.Reasoning = &canonical.ReasoningConfig{
			Effort: &req.Reasoning.Effort,
		}
	}

	// Parse text format
	if req.Text != nil && req.Text.Format != nil {
		creq.ResponseFormat = parseResponsesTextFormatToCanonical(req.Text.Format)
	}

	return creq, nil
}

// parseResponsesInputToMessages converts ResponsesInput to canonical Messages.
func parseResponsesInputToMessages(input ResponsesInput, instructions *string) []canonical.Message {
	var messages []canonical.Message

	// Add instructions as system message if present
	if instructions != nil && *instructions != "" {
		messages = append(messages, canonical.Message{
			Role: canonical.RoleSystem,
			Content: []canonical.ContentBlock{{
				Type: canonical.ContentText,
				Text: *instructions,
			}},
		})
	}

	// Handle string input
	if input.IsString() {
		messages = append(messages, canonical.Message{
			Role: canonical.RoleUser,
			Content: []canonical.ContentBlock{{
				Type: canonical.ContentText,
				Text: input.Text,
			}},
		})
		return messages
	}

	// Handle array input
	for _, item := range input.Items {
		msg := parseResponsesInputItemToMessage(item)
		if msg.Role != "" {
			messages = append(messages, msg)
		}
	}

	return messages
}

// parseResponsesInputItemToMessage converts ResponsesInputItem to canonical Message.
func parseResponsesInputItemToMessage(item ResponsesInputItem) canonical.Message {
	msg := canonical.Message{}

	switch item.Type {
	case "message":
		msg.Role = canonical.Role(item.Role)
		msg.Content = parseResponsesContentToCanonical(item.Content)
	case "function_call":
		msg.Role = canonical.RoleAssistant
		msg.ToolCalls = []canonical.ToolCall{{
			ID:        item.CallID,
			Type:      "function",
			Name:      item.Name,
			Arguments: item.Arguments,
		}}
	case "function_call_output":
		msg.Role = canonical.RoleTool
		msg.ToolCallID = &item.CallID
		msg.Content = []canonical.ContentBlock{{
			Type: canonical.ContentText,
			Text: item.Output,
		}}
	}

	return msg
}

// parseResponsesContentToCanonical converts []ResponsesContent to []ContentBlock.
func parseResponsesContentToCanonical(content []ResponsesContent) []canonical.ContentBlock {
	if len(content) == 0 {
		return nil
	}

	blocks := make([]canonical.ContentBlock, 0, len(content))
	for _, c := range content {
		switch c.Type {
		case "input_text", "output_text":
			blocks = append(blocks, canonical.ContentBlock{
				Type: canonical.ContentText,
				Text: c.Text,
			})
		case "input_image":
			blocks = append(blocks, canonical.ContentBlock{
				Type: canonical.ContentImage,
				Media: &canonical.MediaContent{
					URL:     c.ImageURL,
					Detail:  &c.Detail,
					FileID:  c.FileID,
					Base64:  c.FileData,
					MimeType: c.MimeType,
				},
			})
		case "refusal":
			blocks = append(blocks, canonical.ContentBlock{
				Type: canonical.ContentText,
				Text: c.Refusal,
			})
		}
	}

	return blocks
}

// parseResponsesToolToCanonical converts ResponsesTool to canonical Tool.
func parseResponsesToolToCanonical(tool ResponsesTool) canonical.Tool {
	ctool := canonical.Tool{
		Type: tool.Type,
	}

	if tool.Type == "function" {
		ctool.Name = tool.Name
		ctool.Description = tool.Description
		ctool.Parameters = tool.Parameters
		ctool.Strict = tool.Strict
	}

	if tool.Type == "image_generation" {
		ctool.ImageGeneration = &canonical.ImageGenerationConfig{
			Background:        tool.Background,
			OutputFormat:      tool.OutputFormat,
			Quality:           tool.Quality,
			Size:              tool.Size,
			OutputCompression: tool.OutputCompression,
		}
	}

	return ctool
}

// parseResponsesToolChoiceToCanonical converts ResponsesToolChoice to canonical ToolChoice.
func parseResponsesToolChoiceToCanonical(tc *ResponsesToolChoice) *canonical.ToolChoice {
	if tc == nil {
		return nil
	}

	ctc := &canonical.ToolChoice{}

	if tc.Mode == "tool" && tc.Name != "" {
		ctc.Mode = "tool"
		ctc.Function = &tc.Name
	} else {
		ctc.Mode = tc.Mode
	}

	return ctc
}

// parseResponsesTextFormatToCanonical converts ResponsesTextFormat to canonical ResponseFormat.
func parseResponsesTextFormatToCanonical(tf *ResponsesTextFormat) *canonical.ResponseFormat {
	if tf == nil {
		return nil
	}

	rf := &canonical.ResponseFormat{
		Type: tf.Type,
	}

	if tf.Type == "json_schema" && tf.Schema != nil {
		rf.JsonSchema = &canonical.ResponseSchema{
			Name:        tf.Name,
			Description: tf.Description,
			Schema:      tf.Schema,
		}
		if tf.Strict != nil {
			rf.JsonSchema.Strict = *tf.Strict
		}
	}

	return rf
}

// FormatResponse converts canonical Response to Responses API format.
func (a *ResponsesClientAdapter) FormatResponse(ctx context.Context, resp *canonical.Response) ([]byte, error) {
	oresp := convertCanonicalToResponsesResponse(resp)
	return json.Marshal(oresp)
}

// convertCanonicalToResponsesResponse converts canonical Response to ResponsesResponse.
func convertCanonicalToResponsesResponse(resp *canonical.Response) *ResponsesResponse {
	oresp := &ResponsesResponse{
		ID:      resp.ID,
		Object:  "response",
		Created: resp.Created,
		Model:   resp.Model,
		Status:  "completed",
		Output:  make([]ResponsesOutputItem, 0),
	}

	// Map finish reason to status
	if len(resp.Choices) > 0 && resp.Choices[0].FinishReason != nil {
		switch *resp.Choices[0].FinishReason {
		case "stop":
			oresp.Status = "completed"
		case "length":
			oresp.Status = "incomplete"
		case "error":
			oresp.Status = "failed"
		}
	}

	// Convert choices to output items
	for _, choice := range resp.Choices {
		msg := choice.Message

		// Create message output item
		item := ResponsesOutputItem{
			Type:    "message",
			ID:      "msg_" + resp.ID,
			Role:    "assistant",
			Content: make([]ResponsesContent, 0),
			Status:  "completed",
		}

		// Add text content
		for _, cb := range msg.Content {
			if cb.Type == canonical.ContentText && cb.Text != "" {
				item.Content = append(item.Content, ResponsesContent{
					Type: "output_text",
					Text: cb.Text,
				})
			} else if cb.Type == canonical.ContentImage && cb.Media != nil {
				item.Content = append(item.Content, ResponsesContent{
					Type: "image",
					// Image content handling
				})
			}
		}

		// Add reasoning as separate item
		if msg.Reasoning != nil && *msg.Reasoning != "" {
			oresp.Output = append(oresp.Output, ResponsesOutputItem{
				Type: "reasoning",
				Summary: []ResponsesSummary{{
					Type: "summary_text",
					Text: *msg.Reasoning,
				}},
			})
		}

		// Add tool calls as separate items
		for _, tc := range msg.ToolCalls {
			oresp.Output = append(oresp.Output, ResponsesOutputItem{
				Type:      "function_call",
				CallID:    tc.ID,
				Name:      tc.Name,
				Arguments: tc.Arguments,
			})
		}

		// Add message item if has content
		if len(item.Content) > 0 {
			oresp.Output = append(oresp.Output, item)
		}
	}

	// Convert usage
	if resp.Usage != nil {
		oresp.Usage = &ResponsesUsage{
			InputTokens:     resp.Usage.PromptTokens,
			OutputTokens:    resp.Usage.CompletionTokens,
			TotalTokens:     resp.Usage.TotalTokens,
		}
		if resp.Usage.PromptTokensDetails != nil {
			oresp.Usage.InputTokensDetails = &ResponsesInputTokensDetails{
				CachedTokens: resp.Usage.PromptTokensDetails.CachedTokens,
			}
		}
		if resp.Usage.CompletionTokensDetails != nil {
			oresp.Usage.OutputTokensDetails = &ResponsesOutputTokensDetails{
				ReasoningTokens: resp.Usage.CompletionTokensDetails.ReasoningTokens,
			}
		}
	}

	return oresp
}

// FormatStreamChunk converts canonical Chunk to Responses API SSE format.
// This is the stateful streaming logic (~900 lines in spec).
func (a *ResponsesClientAdapter) FormatStreamChunk(ctx context.Context, chunk *canonical.Chunk) ([]byte, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Handle done marker
	if chunk.Done {
		return []byte("data: [DONE]\n\n"), nil
	}

	var events []string

	// First chunk with role triggers response.created
	if !a.hasResponseCreated && (chunk.ID != "" || len(chunk.Deltas) > 0) {
		a.hasResponseCreated = true
		a.sequenceNumber = 0

		// Emit response.created
		createdEvent := a.createEvent(EventTypeResponseCreated, chunk)
		events = append(events, a.formatEvent(createdEvent))

		// Emit response.in_progress
		inProgressEvent := a.createEvent(EventTypeResponseInProgress, chunk)
		events = append(events, a.formatEvent(inProgressEvent))
	}

	// Process deltas
	for _, delta := range chunk.Deltas {
		// Handle reasoning content
		if delta.Delta.Reasoning != nil && *delta.Delta.Reasoning != "" {
			if !a.hasReasoningItemStarted {
				a.hasReasoningItemStarted = true
				// Emit reasoning item added
				itemEvent := a.createItemEvent(EventTypeOutputItemAdded, "reasoning", a.sequenceNumber)
				a.sequenceNumber++
				events = append(events, a.formatEvent(itemEvent))
			}
			// Emit reasoning delta
			deltaEvent := a.createDeltaEvent(EventTypeReasoningSummaryTextDelta, *delta.Delta.Reasoning, a.sequenceNumber)
			a.sequenceNumber++
			events = append(events, a.formatEvent(deltaEvent))
		}

		// Handle text content
		for _, cb := range delta.Delta.Content {
			if cb.Type == canonical.ContentText && cb.Text != "" {
				if !a.hasMessageItemStarted {
					a.hasMessageItemStarted = true
					// Emit message item added
					itemEvent := a.createItemEvent(EventTypeOutputItemAdded, "message", a.sequenceNumber)
					a.sequenceNumber++
					events = append(events, a.formatEvent(itemEvent))
				}

				if !a.hasContentPartStarted {
					a.hasContentPartStarted = true
					// Emit content part added
					partEvent := a.createContentPartEvent(EventTypeContentPartAdded, "output_text", a.sequenceNumber)
					a.sequenceNumber++
					events = append(events, a.formatEvent(partEvent))
				}

				// Emit text delta
				deltaEvent := a.createDeltaEvent(EventTypeOutputTextDelta, cb.Text, a.sequenceNumber)
				a.sequenceNumber++
				events = append(events, a.formatEvent(deltaEvent))
			}
		}

		// Handle tool calls
		for _, tc := range delta.Delta.ToolCalls {
			// Tool call start (has ID and Name)
			if tc.ID != "" && tc.Name != "" {
				// Emit function call item added
				itemEvent := a.createFunctionCallItemEvent(tc.ID, tc.Name, a.sequenceNumber)
				a.sequenceNumber++
				events = append(events, a.formatEvent(itemEvent))
				a.currentToolCallIndex = tc.Index
			}

			// Tool call arguments delta
			if tc.Arguments != "" {
				deltaEvent := a.createFunctionCallDeltaEvent(tc.Arguments, a.currentToolCallIndex, a.sequenceNumber)
				a.sequenceNumber++
				events = append(events, a.formatEvent(deltaEvent))
			}
		}

		// Handle finish reason
		if delta.FinishReason != nil {
			// Emit completed event
			completedEvent := a.createCompletedEvent(chunk, *delta.FinishReason)
			events = append(events, a.formatEvent(completedEvent))
		}
	}

	// Store chunk for aggregation
	a.aggregator.addChunk(chunk)

	// Join all events
	result := ""
	for _, e := range events {
		result += e
	}

	return []byte(result), nil
}

// createEvent creates a basic stream event.
func (a *ResponsesClientAdapter) createEvent(eventType string, chunk *canonical.Chunk) ResponsesStreamEvent {
	return ResponsesStreamEvent{
		Type: eventType,
		Response: &ResponsesResponse{
			ID:      chunk.ID,
			Model:   chunk.Model,
			Created: chunk.Created,
			Status:  "in_progress",
		},
		SequenceNumber: a.sequenceNumber,
	}
}

// createItemEvent creates an output_item.added event.
func (a *ResponsesClientAdapter) createItemEvent(eventType, itemType string, seqNum int) ResponsesStreamEvent {
	return ResponsesStreamEvent{
		Type: eventType,
		Item: &ResponsesOutputItem{
			Type: itemType,
		},
		SequenceNumber: seqNum,
	}
}

// createContentPartEvent creates a content_part.added event.
func (a *ResponsesClientAdapter) createContentPartEvent(eventType, contentType string, seqNum int) ResponsesStreamEvent {
	return ResponsesStreamEvent{
		Type: eventType,
		Content: &ResponsesContent{
			Type: contentType,
		},
		SequenceNumber: seqNum,
	}
}

// createDeltaEvent creates a delta event.
func (a *ResponsesClientAdapter) createDeltaEvent(eventType, delta string, seqNum int) ResponsesStreamEvent {
	return ResponsesStreamEvent{
		Type:           eventType,
		Delta:          delta,
		SequenceNumber: seqNum,
	}
}

// createFunctionCallItemEvent creates a function_call item event.
func (a *ResponsesClientAdapter) createFunctionCallItemEvent(callID, name string, seqNum int) ResponsesStreamEvent {
	return ResponsesStreamEvent{
		Type: EventTypeOutputItemAdded,
		Item: &ResponsesOutputItem{
			Type:   "function_call",
			CallID: callID,
			Name:   name,
		},
		SequenceNumber: seqNum,
	}
}

// createFunctionCallDeltaEvent creates a function_call_arguments.delta event.
func (a *ResponsesClientAdapter) createFunctionCallDeltaEvent(args string, outputIndex, seqNum int) ResponsesStreamEvent {
	return ResponsesStreamEvent{
		Type:         EventTypeFunctionCallArgumentsDelta,
		Delta:        args,
		OutputIndex:  outputIndex,
		SequenceNumber: seqNum,
	}
}

// createCompletedEvent creates a response.completed event.
func (a *ResponsesClientAdapter) createCompletedEvent(chunk *canonical.Chunk, finishReason string) ResponsesStreamEvent {
	status := "completed"
	switch finishReason {
	case "length":
		status = "incomplete"
	case "error":
		status = "failed"
	}

	resp := &ResponsesResponse{
		ID:      chunk.ID,
		Model:   chunk.Model,
		Created: chunk.Created,
		Status:  status,
	}

	if chunk.Usage != nil {
		resp.Usage = &ResponsesUsage{
			InputTokens:  chunk.Usage.PromptTokens,
			OutputTokens: chunk.Usage.CompletionTokens,
			TotalTokens:  chunk.Usage.TotalTokens,
		}
	}

	return ResponsesStreamEvent{
		Type:           EventTypeResponseCompleted,
		Response:       resp,
		SequenceNumber: a.sequenceNumber,
	}
}

// formatEvent formats a stream event to SSE format.
func (a *ResponsesClientAdapter) formatEvent(event ResponsesStreamEvent) string {
	data, err := json.Marshal(event)
	if err != nil {
		return ""
	}
	return "data: " + string(data) + "\n\n"
}

// AggregateStream aggregates all collected chunks into a full response.
func (a *ResponsesClientAdapter) AggregateStream(ctx context.Context) (*canonical.Response, error) {
	return a.aggregator.aggregate()
}

// FormatError converts canonical Error to Responses API error format.
func (a *ResponsesClientAdapter) FormatError(ctx context.Context, err *canonical.Error) ([]byte, error) {
	oerr := struct {
		Error ResponsesError `json:"error"`
	}{
		Error: ResponsesError{
			Code:    err.Code,
			Message: err.Message,
			Type:    err.Type,
		},
	}
	return json.Marshal(oerr)
}