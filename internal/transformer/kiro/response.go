package kiro

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/bestruirui/octopus/internal/oauth/kiro"
	"github.com/bestruirui/octopus/internal/transformer/model"
	"github.com/bestruirui/octopus/internal/utils/log"
	"github.com/google/uuid"
)

// ResponseOutbound handles transforming Kiro responses to OpenAI format
type ResponseOutbound struct {
	parser          *kiro.AwsEventStreamParser
	accumulatedText strings.Builder
	toolCalls       []kiro.ToolCall
	usage           *model.Usage
	thinkingText    strings.Builder
	modelName       string
	responseID      string
	created         int64
}

// NewResponseOutbound creates a new response transformer
func NewResponseOutbound() *ResponseOutbound {
	return &ResponseOutbound{
		parser:     kiro.NewAwsEventStreamParser(),
		responseID: generateResponseID(),
		created:    time.Now().Unix(),
	}
}

// TransformResponse transforms a Kiro HTTP response to InternalLLMResponse
func (t *ResponseOutbound) TransformResponse(ctx context.Context, response *http.Response) (*model.InternalLLMResponse, error) {
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("kiro: read response body failed: %w", err)
	}

	// Check for error response
	if response.StatusCode >= 400 {
		return nil, fmt.Errorf("kiro: error response %d: %s", response.StatusCode, string(body))
	}

	// Parse all events from the body
	events := t.parser.Feed(body)

	// Process events
	for _, event := range events {
		t.processEvent(event)
	}

	// Get any remaining tool calls
	t.toolCalls = append(t.toolCalls, t.parser.GetToolCalls()...)

	// Build final response
	return t.buildFinalResponse(), nil
}

// TransformStream transforms a Kiro streaming event to InternalLLMResponse
func (t *ResponseOutbound) TransformStream(ctx context.Context, eventData []byte) (*model.InternalLLMResponse, error) {
	events := t.parser.Feed(eventData)

	if len(events) == 0 {
		return nil, nil
	}

	// Process events and build streaming response
	for _, event := range events {
		t.processEvent(event)
	}

	// Build streaming response for each event
	return t.buildStreamResponse(events)
}

// processEvent processes a single Kiro event
func (t *ResponseOutbound) processEvent(event kiro.Event) {
	switch event.Type {
	case kiro.EventTypeContent:
		if data, ok := event.Data.(kiro.ContentData); ok {
			t.accumulatedText.WriteString(data.Content)
		}
	case kiro.EventTypeThinking:
		if data, ok := event.Data.(kiro.ThinkingData); ok {
			t.thinkingText.WriteString(data.Content)
		}
	case kiro.EventTypeUsage:
		if data, ok := event.Data.(kiro.UsageData); ok {
			if t.usage == nil {
				t.usage = &model.Usage{}
			}
			// Kiro usage is in credits, approximate to tokens
			t.usage.CompletionTokens = int64(data.Credits)
		}
	}
}

// buildFinalResponse builds the final non-streaming response
func (t *ResponseOutbound) buildFinalResponse() *model.InternalLLMResponse {
	resp := &model.InternalLLMResponse{
		ID:      t.responseID,
		Object:  "chat.completion",
		Created: t.created,
		Model:   t.modelName,
		Choices: []model.Choice{{
			Index: 0,
			Message: &model.Message{
				Role: "assistant",
			},
		}},
	}

	// Set content
	content := t.accumulatedText.String()
	if content != "" {
		resp.Choices[0].Message.Content = model.MessageContent{Content: &content}
	}

	// Set reasoning content
	if t.thinkingText.Len() > 0 {
		thinking := t.thinkingText.String()
		resp.Choices[0].Message.ReasoningContent = &thinking
	}

	// Set tool calls
	toolCalls := t.parser.GetToolCalls()
	if len(toolCalls) > 0 {
		resp.Choices[0].Message.ToolCalls = convertKiroToolCalls(toolCalls)
	}

	// Set usage
	if t.usage != nil {
		resp.Usage = t.usage
	}

	// Set finish reason
	finishReason := "stop"
	if len(toolCalls) > 0 {
		finishReason = "tool_calls"
	}
	resp.Choices[0].FinishReason = &finishReason

	return resp
}

// buildStreamResponse builds a streaming response from events
func (t *ResponseOutbound) buildStreamResponse(events []kiro.Event) (*model.InternalLLMResponse, error) {
	if len(events) == 0 {
		return nil, nil
	}

	resp := &model.InternalLLMResponse{
		ID:      t.responseID,
		Object:  "chat.completion.chunk",
		Created: t.created,
		Model:   t.modelName,
		Choices: []model.Choice{{
			Index: 0,
			Delta: &model.Message{
				Role: "assistant",
			},
		}},
	}

	// Process each event type
	for _, event := range events {
		switch event.Type {
		case kiro.EventTypeContent:
			if data, ok := event.Data.(kiro.ContentData); ok {
				resp.Choices[0].Delta.Content = model.MessageContent{Content: &data.Content}
			}
		case kiro.EventTypeThinking:
			if data, ok := event.Data.(kiro.ThinkingData); ok {
				resp.Choices[0].Delta.ReasoningContent = &data.Content
			}
		case kiro.EventTypeUsage:
			if data, ok := event.Data.(kiro.UsageData); ok {
				if t.usage == nil {
					t.usage = &model.Usage{}
				}
				t.usage.CompletionTokens = int64(data.Credits)
				resp.Usage = &model.Usage{
					CompletionTokens: int64(data.Credits),
				}
			}
		}
	}

	// Handle tool calls in streaming
	toolCalls := t.parser.GetToolCalls()
	if len(toolCalls) > 0 {
		// For streaming, we add tool calls incrementally
		resp.Choices[0].Delta.ToolCalls = convertKiroToolCalls(toolCalls)
	}

	return resp, nil
}

// convertKiroToolCalls converts Kiro tool calls to OpenAI format
func convertKiroToolCalls(calls []kiro.ToolCall) []model.ToolCall {
	if len(calls) == 0 {
		return nil
	}

	result := make([]model.ToolCall, len(calls))
	for i, tc := range calls {
		result[i] = model.ToolCall{
			ID:    tc.ID,
			Type:  tc.Type,
			Index: i,
			Function: model.FunctionCall{
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			},
		}
	}
	return result
}

// SetModelName sets the model name for the response
func (t *ResponseOutbound) SetModelName(name string) {
	t.modelName = name
}

// GetParser returns the underlying parser for advanced usage
func (t *ResponseOutbound) GetParser() *kiro.AwsEventStreamParser {
	return t.parser
}

// Reset resets the transformer state for reuse
func (t *ResponseOutbound) Reset() {
	t.parser.Reset()
	t.accumulatedText.Reset()
	t.thinkingText.Reset()
	t.toolCalls = nil
	t.usage = nil
	t.responseID = generateResponseID()
	t.created = time.Now().Unix()
}

func generateResponseID() string {
	return "chatcmpl-" + uuid.New().String()[:29]
}

// ParseKiroStreamResponse parses raw Kiro streaming response data
// This is useful for converting raw AWS Event Stream data to OpenAI SSE format
func ParseKiroStreamResponse(data []byte, parser *kiro.AwsEventStreamParser, modelName string) ([]byte, error) {
	events := parser.Feed(data)

	var results []byte
	for _, event := range events {
		chunk := buildSSEChunk(event, modelName)
		if chunk != nil {
			results = append(results, chunk...)
		}
	}

	return results, nil
}

func buildSSEChunk(event kiro.Event, modelName string) []byte {
	resp := &model.InternalLLMResponse{
		ID:      generateResponseID(),
		Object:  "chat.completion.chunk",
		Created: time.Now().Unix(),
		Model:   modelName,
		Choices: []model.Choice{{
			Index: 0,
			Delta: &model.Message{},
		}},
	}

	switch event.Type {
	case kiro.EventTypeContent:
		if data, ok := event.Data.(kiro.ContentData); ok {
			resp.Choices[0].Delta.Content = model.MessageContent{Content: &data.Content}
		}
	case kiro.EventTypeThinking:
		if data, ok := event.Data.(kiro.ThinkingData); ok {
			resp.Choices[0].Delta.ReasoningContent = &data.Content
		}
	case kiro.EventTypeUsage:
		if data, ok := event.Data.(kiro.UsageData); ok {
			resp.Usage = &model.Usage{CompletionTokens: int64(data.Credits)}
		}
	default:
		return nil
	}

	b, err := json.Marshal(resp)
	if err != nil {
		return nil
	}

	return []byte(fmt.Sprintf("data: %s\n\n", string(b)))
}

// IsKiroStreamError checks if the response indicates a Kiro error
func IsKiroStreamError(data []byte) bool {
	dataStr := strings.ToLower(string(data))
	return strings.Contains(dataStr, "unauthorizedexception") ||
		strings.Contains(dataStr, "accessdenied") ||
		strings.Contains(dataStr, "throttlingexception")
}

// logStreamingEvent logs streaming events for debugging
func logStreamingEvent(event kiro.Event) {
	switch event.Type {
	case kiro.EventTypeContent:
		if data, ok := event.Data.(kiro.ContentData); ok {
			log.Debugf("kiro stream: content event, len=%d", len(data.Content))
		}
	case kiro.EventTypeToolStart:
		if data, ok := event.Data.(kiro.ToolStartData); ok {
			log.Debugf("kiro stream: tool_start event, name=%s", data.Name)
		}
	case kiro.EventTypeUsage:
		if data, ok := event.Data.(kiro.UsageData); ok {
			log.Debugf("kiro stream: usage event, credits=%d", data.Credits)
		}
	}
}
