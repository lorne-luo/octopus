package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/bestruirui/octopus/internal/transformer2/canonical"
)

// ResponsesProviderAdapter implements ProviderAdapter for OpenAI Responses API.
type ResponsesProviderAdapter struct{}

// NewResponsesProviderAdapter creates a new ResponsesProviderAdapter.
func NewResponsesProviderAdapter() *ResponsesProviderAdapter {
	return &ResponsesProviderAdapter{}
}

// BuildRequest builds an HTTP request for OpenAI Responses API from canonical Request.
func (a *ResponsesProviderAdapter) BuildRequest(ctx context.Context, req *canonical.Request, baseURL, key string) (*http.Request, error) {
	oreq := convertCanonicalToResponsesRequest(req)

	body, err := json.Marshal(oreq)
	if err != nil {
		return nil, err
	}

	url := strings.TrimSuffix(baseURL, "/") + "/v1/responses"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+key)

	return httpReq, nil
}

// ParseResponse parses OpenAI Responses HTTP response to canonical Response.
func (a *ResponsesProviderAdapter) ParseResponse(ctx context.Context, resp *http.Response) (*canonical.Response, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		var errResp struct {
			Error *ResponsesError `json:"error,omitempty"`
		}
		if json.Unmarshal(body, &errResp) == nil && errResp.Error != nil {
			return &canonical.Response{
				StatusCode: resp.StatusCode,
				Error: &canonical.Error{
					Code:       errResp.Error.Code,
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

	var oresp ResponsesResponse
	if err := json.Unmarshal(body, &oresp); err != nil {
		return nil, err
	}

	return convertResponsesResponseToCanonical(&oresp, resp.StatusCode), nil
}

// ParseStreamChunk parses SSE data to canonical Chunk for Responses API.
func (a *ResponsesProviderAdapter) ParseStreamChunk(ctx context.Context, data []byte) (*canonical.Chunk, error) {
	dataStr := string(data)

	// Handle [DONE]
	if dataStr == "[DONE]" {
		return &canonical.Chunk{Done: true}, nil
	}

	var event ResponsesStreamEvent
	if err := json.Unmarshal(data, &event); err != nil {
		return nil, err
	}

	return convertResponsesEventToChunk(&event), nil
}

// convertCanonicalToResponsesRequest converts canonical Request to Responses API format.
func convertCanonicalToResponsesRequest(req *canonical.Request) *ResponsesRequest {
	oreq := &ResponsesRequest{
		Model:           req.Model,
		MaxOutputTokens: req.MaxCompletionTokens,
		Temperature:     req.Temperature,
		TopP:            req.TopP,
		Store:           req.Store,
		ServiceTier:     req.ServiceTier,
		User:            req.User,
	}

	// Handle stream
	if req.Stream {
		oreq.Stream = &req.Stream
	}

	// Extract instructions and convert messages to input
	instructions, input := extractInstructionsAndInput(req.Messages, req.Hints)
	if instructions != nil {
		oreq.Instructions = instructions
	}
	oreq.Input = input

	// Convert tools
	if len(req.Tools) > 0 {
		oreq.Tools = make([]ResponsesTool, len(req.Tools))
		for i, tool := range req.Tools {
			oreq.Tools[i] = convertCanonicalToolToResponsesTool(tool)
		}
	}

	// Convert tool_choice
	if req.ToolChoice != nil {
		oreq.ToolChoice = convertCanonicalToolChoiceToResponses(req.ToolChoice)
		if req.ToolChoice.DisableParallelToolUse != nil {
			parallel := !*req.ToolChoice.DisableParallelToolUse
			oreq.ParallelToolCalls = &parallel
		}
	}

	// Convert reasoning
	if req.Reasoning != nil && req.Reasoning.Effort != nil {
		oreq.Reasoning = &ResponsesReasoning{
			Effort: *req.Reasoning.Effort,
		}
	}

	// Convert text format
	if req.ResponseFormat != nil {
		oreq.Text = convertCanonicalResponseFormatToResponsesText(req.ResponseFormat)
	}

	// Handle include hints
	if len(req.Hints.Include) > 0 {
		oreq.Include = req.Hints.Include
	}

	return oreq
}

// extractInstructionsAndInput extracts instructions and converts messages to Responses input.
func extractInstructionsAndInput(messages []canonical.Message, hints canonical.TransformHints) (*string, ResponsesInput) {
	var instructions *string
	var inputItems []ResponsesInputItem
	useArrayFormat := hints.ResponsesArrayInput != nil && *hints.ResponsesArrayInput

	// Collect non-system messages
	var nonSystemMessages []canonical.Message
	for _, msg := range messages {
		if msg.Role == canonical.RoleSystem || msg.Role == canonical.RoleDeveloper {
			// Use first system/developer message as instructions
			if instructions == nil && len(msg.Content) > 0 && msg.Content[0].Type == canonical.ContentText {
				instructions = &msg.Content[0].Text
			}
			continue
		}
		nonSystemMessages = append(nonSystemMessages, msg)
	}

	// If no non-system messages, return empty input
	if len(nonSystemMessages) == 0 {
		return instructions, ResponsesInput{Text: ""}
	}

	// Check if we can use simple string input
	// Only if: single user message with text-only content and not explicitly using array format
	if !useArrayFormat && len(nonSystemMessages) == 1 &&
		nonSystemMessages[0].Role == canonical.RoleUser &&
		isTextOnlyContent(nonSystemMessages[0].Content) {
		text := extractTextFromContent(nonSystemMessages[0].Content)
		return instructions, ResponsesInput{Text: text}
	}

	// Convert to array format
	inputItems = make([]ResponsesInputItem, 0, len(nonSystemMessages))
	for _, msg := range nonSystemMessages {
		item := convertCanonicalMessageToResponsesInputItem(msg)
		inputItems = append(inputItems, item)
	}

	return instructions, ResponsesInput{Items: inputItems}
}

// isTextOnlyContent checks if content is a single text block.
func isTextOnlyContent(content []canonical.ContentBlock) bool {
	if len(content) != 1 {
		return false
	}
	return content[0].Type == canonical.ContentText && content[0].Media == nil
}

// extractTextFromContent extracts text from content blocks.
func extractTextFromContent(content []canonical.ContentBlock) string {
	if len(content) == 0 {
		return ""
	}
	var texts []string
	for _, cb := range content {
		if cb.Type == canonical.ContentText {
			texts = append(texts, cb.Text)
		}
	}
	return strings.Join(texts, "")
}

// convertCanonicalMessageToResponsesInputItem converts canonical Message to ResponsesInputItem.
func convertCanonicalMessageToResponsesInputItem(msg canonical.Message) ResponsesInputItem {
	item := ResponsesInputItem{}

	switch msg.Role {
	case canonical.RoleUser:
		item.Role = "user"
		item.Type = "message"
		item.Content = convertCanonicalContentToResponsesContent(msg.Content)
	case canonical.RoleAssistant:
		// Check for tool calls
		if len(msg.ToolCalls) > 0 {
			// For each tool call, create a function_call item
			// Note: This handles single tool call; multiple would need different handling
			tc := msg.ToolCalls[0]
			item.Type = "function_call"
			item.CallID = tc.ID
			item.Name = tc.Name
			item.Arguments = tc.Arguments
		} else {
			item.Type = "message"
			item.Role = "assistant"
			item.Content = convertCanonicalContentToResponsesContent(msg.Content)
		}
	case canonical.RoleTool:
		item.Type = "function_call_output"
		item.CallID = *msg.ToolCallID
		if len(msg.Content) > 0 && msg.Content[0].Type == canonical.ContentText {
			item.Output = msg.Content[0].Text
		}
	}

	return item
}

// convertCanonicalContentToResponsesContent converts canonical ContentBlock to ResponsesContent.
func convertCanonicalContentToResponsesContent(content []canonical.ContentBlock) []ResponsesContent {
	result := make([]ResponsesContent, 0, len(content))
	for _, cb := range content {
		rc := ResponsesContent{}
		switch cb.Type {
		case canonical.ContentText:
			rc.Type = "output_text"
			if len(result) == 0 {
				rc.Type = "input_text"
			}
			rc.Text = cb.Text
		case canonical.ContentImage:
			rc.Type = "input_image"
			if cb.Media != nil {
				rc.ImageURL = cb.Media.URL
				rc.Detail = detailStr(cb.Media.Detail)
				rc.FileID = cb.Media.FileID
				rc.FileData = cb.Media.Base64
				if cb.Media.MimeType != "" {
					rc.MimeType = cb.Media.MimeType
				}
			}
		}
		result = append(result, rc)
	}
	return result
}

// convertCanonicalToolToResponsesTool converts canonical Tool to ResponsesTool.
func convertCanonicalToolToResponsesTool(tool canonical.Tool) ResponsesTool {
	rt := ResponsesTool{
		Type: tool.Type,
	}

	if tool.Type == "function" {
		rt.Name = tool.Name
		rt.Description = tool.Description
		rt.Parameters = tool.Parameters
		rt.Strict = tool.Strict
	}

	if tool.Type == "image_generation" && tool.ImageGeneration != nil {
		rt.Background = tool.ImageGeneration.Background
		rt.OutputFormat = tool.ImageGeneration.OutputFormat
		rt.Quality = tool.ImageGeneration.Quality
		rt.Size = tool.ImageGeneration.Size
		rt.OutputCompression = tool.ImageGeneration.OutputCompression
	}

	return rt
}

// convertCanonicalToolChoiceToResponses converts canonical ToolChoice to ResponsesToolChoice.
func convertCanonicalToolChoiceToResponses(tc *canonical.ToolChoice) *ResponsesToolChoice {
	if tc == nil {
		return nil
	}

	rtc := &ResponsesToolChoice{}

	if tc.Function != nil {
		rtc.Type = "function"
		rtc.Name = *tc.Function
		rtc.Mode = "tool"
	} else {
		rtc.Mode = tc.Mode
	}

	return rtc
}

// convertCanonicalResponseFormatToResponsesText converts canonical ResponseFormat to ResponsesText.
func convertCanonicalResponseFormatToResponsesText(rf *canonical.ResponseFormat) *ResponsesText {
	if rf == nil {
		return nil
	}

	rt := &ResponsesText{
		Format: &ResponsesTextFormat{
			Type: rf.Type,
		},
	}

	if rf.Type == "json_schema" && rf.JsonSchema != nil {
		rt.Format.Name = rf.JsonSchema.Name
		rt.Format.Description = rf.JsonSchema.Description
		rt.Format.Schema = rf.JsonSchema.Schema
		rt.Format.Strict = &rf.JsonSchema.Strict
	}

	return rt
}

// convertResponsesResponseToCanonical converts ResponsesResponse to canonical Response.
func convertResponsesResponseToCanonical(oresp *ResponsesResponse, statusCode int) *canonical.Response {
	creq := &canonical.Response{
		ID:         oresp.ID,
		Model:      oresp.Model,
		Created:    oresp.Created,
		Object:     oresp.Object,
		StatusCode: statusCode,
	}

	// Convert status to finish reason
	if oresp.Status == "completed" {
		finish := "stop"
		creq.Choices = []canonical.Choice{{
			Index:        0,
			FinishReason: &finish,
		}}
	} else if oresp.Status == "incomplete" {
		finish := "length"
		creq.Choices = []canonical.Choice{{
			Index:        0,
			FinishReason: &finish,
		}}
	} else if oresp.Status == "failed" {
		finish := "error"
		creq.Choices = []canonical.Choice{{
			Index:        0,
			FinishReason: &finish,
		}}
	}

	// Convert usage
	if oresp.Usage != nil {
		creq.Usage = convertResponsesUsageToCanonical(oresp.Usage)
	}

	// Convert output items to message content
	if len(oresp.Output) > 0 {
		msg := canonical.Message{
			Role:    canonical.RoleAssistant,
			Content: make([]canonical.ContentBlock, 0),
		}

		for _, item := range oresp.Output {
			switch item.Type {
			case "message":
				// Extract content from message item
				for _, c := range item.Content {
					if c.Type == "output_text" {
						msg.Content = append(msg.Content, canonical.ContentBlock{
							Type: canonical.ContentText,
							Text: c.Text,
						})
					}
				}
			case "function_call":
				msg.ToolCalls = append(msg.ToolCalls, canonical.ToolCall{
					ID:        item.CallID,
					Type:      "function",
					Name:      item.Name,
					Arguments: item.Arguments,
				})
				// Update finish reason for tool calls
				finish := "tool_calls"
				creq.Choices = []canonical.Choice{{
					Index:        0,
					FinishReason: &finish,
				}}
			case "reasoning":
				if len(item.Summary) > 0 {
					var reasoningText string
					for _, s := range item.Summary {
						reasoningText += s.Text
					}
					msg.Reasoning = &reasoningText
				}
			case "image_generation_call":
				msg.Content = append(msg.Content, canonical.ContentBlock{
					Type: canonical.ContentImage,
					Media: &canonical.MediaContent{
						Base64: item.Result,
					},
				})
			}
		}

		if len(creq.Choices) > 0 {
			creq.Choices[0].Message = msg
		} else {
			creq.Choices = []canonical.Choice{{Index: 0, Message: msg}}
		}
	}

	// Handle error
	if oresp.Error != nil {
		creq.Error = &canonical.Error{
			Code:    oresp.Error.Code,
			Message: oresp.Error.Message,
			Type:    oresp.Error.Type,
		}
	}

	return creq
}

// convertResponsesUsageToCanonical converts ResponsesUsage to canonical Usage.
func convertResponsesUsageToCanonical(usage *ResponsesUsage) *canonical.Usage {
	if usage == nil {
		return nil
	}

	cusage := &canonical.Usage{
		PromptTokens:     usage.InputTokens,
		CompletionTokens: usage.OutputTokens,
		TotalTokens:      usage.TotalTokens,
	}

	if usage.InputTokensDetails != nil {
		cusage.PromptTokensDetails = &canonical.PromptTokensDetails{
			CachedTokens: usage.InputTokensDetails.CachedTokens,
		}
	}

	if usage.OutputTokensDetails != nil {
		cusage.CompletionTokensDetails = &canonical.CompletionTokensDetails{
			ReasoningTokens: usage.OutputTokensDetails.ReasoningTokens,
		}
	}

	return cusage
}

// convertResponsesEventToChunk converts ResponsesStreamEvent to canonical Chunk.
func convertResponsesEventToChunk(event *ResponsesStreamEvent) *canonical.Chunk {
	chunk := &canonical.Chunk{}

	// Set ID and model from response if present
	if event.Response != nil {
		chunk.ID = event.Response.ID
		chunk.Model = event.Response.Model
		chunk.Created = event.Response.Created
	}

	switch event.Type {
	case EventTypeResponseCreated, EventTypeResponseInProgress:
		// Initial event, may carry response metadata
		if event.Response != nil {
			chunk.ID = event.Response.ID
			chunk.Model = event.Response.Model
		}

	case EventTypeOutputTextDelta:
		// Text content delta
		chunk.Deltas = []canonical.ChoiceDelta{{
			Index: 0,
			Delta: canonical.Message{
				Role: canonical.RoleAssistant,
				Content: []canonical.ContentBlock{{
					Type: canonical.ContentText,
					Text: event.Delta,
				}},
			},
		}}

	case EventTypeFunctionCallArgumentsDelta:
		// Tool call arguments delta
		var idx int
		if event.OutputIndex > 0 {
			idx = event.OutputIndex
		}
		chunk.Deltas = []canonical.ChoiceDelta{{
			Index: idx,
			Delta: canonical.Message{
				Role: canonical.RoleAssistant,
				ToolCalls: []canonical.ToolCall{{
					Index:     idx,
					Arguments: event.Delta,
				}},
			},
		}}

	case EventTypeOutputItemAdded:
		// New output item (function_call start, etc.)
		if event.Item != nil && event.Item.Type == "function_call" {
			chunk.Deltas = []canonical.ChoiceDelta{{
				Index: 0,
				Delta: canonical.Message{
					Role: canonical.RoleAssistant,
					ToolCalls: []canonical.ToolCall{{
						ID:    event.Item.CallID,
						Type:  "function",
						Name:  event.Item.Name,
						Index: 0,
					}},
				},
			}}
		}

	case EventTypeReasoningSummaryTextDelta:
		// Reasoning content delta
		chunk.Deltas = []canonical.ChoiceDelta{{
			Index: 0,
			Delta: canonical.Message{
				Role:      canonical.RoleAssistant,
				Reasoning: &event.Delta,
			},
		}}

	case EventTypeResponseCompleted:
		// Final event with complete response
		if event.Response != nil {
			// Map status to finish reason
			var finish string
			switch event.Response.Status {
			case "completed":
				finish = "stop"
			case "incomplete":
				finish = "length"
			case "failed":
				finish = "error"
			}
			chunk.Deltas = []canonical.ChoiceDelta{{
				Index:        0,
				FinishReason: &finish,
			}}
			// Include usage if present
			if event.Response.Usage != nil {
				chunk.Usage = convertResponsesUsageToCanonical(event.Response.Usage)
			}
		}

	case EventTypeResponseFailed:
		// Error event
		errFinish := "error"
		chunk.Deltas = []canonical.ChoiceDelta{{
			Index:        0,
			FinishReason: &errFinish,
		}}
	}

	return chunk
}

// detailStr converts *string to string, returning empty string if nil.
func detailStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}