package kiro

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	kiroparser "github.com/bestruirui/octopus/internal/oauth/kiro"
	"github.com/bestruirui/octopus/internal/transformer2/canonical"
	"github.com/bestruirui/octopus/internal/utils/log"
	"github.com/google/uuid"
)

// ProviderAdapter implements adapter.ProviderAdapter for the Kiro API.
// It also implements adapter.RawStreamProvider since Kiro uses AWS Event Stream
// format rather than standard SSE.
type ProviderAdapter struct {
	profileArn string
	modelName  string

	// Streaming state
	parser          *kiroparser.AwsEventStreamParser
	accumulatedText strings.Builder
	thinkingText    strings.Builder
	usage           *canonical.Usage
	responseID      string
	created         int64
}

// NewProviderAdapter creates a new Kiro provider adapter.
func NewProviderAdapter() *ProviderAdapter {
	return &ProviderAdapter{
		parser:     kiroparser.NewAwsEventStreamParser(),
		responseID: "chatcmpl-" + uuid.New().String()[:29],
		created:    time.Now().Unix(),
	}
}

// SetProfileArn sets the profile ARN for Kiro requests.
func (p *ProviderAdapter) SetProfileArn(arn string) {
	p.profileArn = arn
}

// SetModelName sets the model name for responses.
func (p *ProviderAdapter) SetModelName(name string) {
	p.modelName = name
}

// IsRawStream implements adapter.RawStreamProvider.
// Kiro uses AWS Event Stream binary format, not SSE.
func (p *ProviderAdapter) IsRawStream() bool {
	return true
}

// ── adapter.ProviderAdapter implementation ──────────────────────────────

// BuildRequest builds an HTTP request for the Kiro API from a canonical Request.
func (p *ProviderAdapter) BuildRequest(ctx context.Context, req *canonical.Request, baseURL, key string) (*http.Request, error) {
	p.modelName = req.Model

	kiroReq, err := buildKiroRequest(req)
	if err != nil {
		return nil, fmt.Errorf("kiro: failed to build request: %w", err)
	}

	body, err := json.Marshal(kiroReq)
	if err != nil {
		return nil, fmt.Errorf("kiro: failed to marshal request: %w", err)
	}

	endpoint := strings.TrimSuffix(baseURL, "/") + "/invoke"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("kiro: failed to create HTTP request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "*/*")
	httpReq.Header.Set("Authorization", "Bearer "+key)

	if p.profileArn != "" {
		httpReq.Header.Set("x-amz-profile-arn", p.profileArn)
	}

	log.Debugf("kiro: request to %s, model=%s", endpoint, kiroReq.ConversationState.CurrentMessage.UserInputMessage.ModelID)
	return httpReq, nil
}

// ParseResponse parses a non-streaming Kiro HTTP response to canonical Response.
func (p *ProviderAdapter) ParseResponse(ctx context.Context, resp *http.Response) (*canonical.Response, error) {
	if resp == nil {
		return nil, fmt.Errorf("kiro: response is nil")
	}

	// Read entire body as AWS Event Stream
	buf, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, fmt.Errorf("kiro: failed to read response body: %w", readErr)
	}

	// Parse all events
	events := p.parser.Feed(buf)

	// Accumulate text and tool calls
	for _, event := range events {
		p.processEvent(event)
	}

	return p.buildFinalResponse(), nil
}

// ParseStreamChunk parses AWS Event Stream data to canonical Chunk.
func (p *ProviderAdapter) ParseStreamChunk(ctx context.Context, data []byte) (*canonical.Chunk, error) {
	events := p.parser.Feed(data)
	if len(events) == 0 {
		return nil, nil
	}

	chunk := &canonical.Chunk{
		ID:      p.responseID,
		Model:   p.modelName,
		Created: p.created,
	}

	for _, event := range events {
		p.processEvent(event)

		switch event.Type {
		case kiroparser.EventTypeContent:
			if d, ok := event.Data.(kiroparser.ContentData); ok {
				chunk.Deltas = append(chunk.Deltas, canonical.ChoiceDelta{
					Index: 0,
					Delta: canonical.Message{
						Role:    canonical.RoleAssistant,
						Content: []canonical.ContentBlock{{Type: canonical.ContentText, Text: d.Content}},
					},
				})
			}

		case kiroparser.EventTypeThinking:
			if d, ok := event.Data.(kiroparser.ThinkingData); ok {
				reasoning := d.Content
				chunk.Deltas = append(chunk.Deltas, canonical.ChoiceDelta{
					Index: 0,
					Delta: canonical.Message{
						Role:      canonical.RoleAssistant,
						Reasoning: &reasoning,
					},
				})
			}

		case kiroparser.EventTypeUsage:
			if d, ok := event.Data.(kiroparser.UsageData); ok {
				chunk.Usage = &canonical.Usage{
					CompletionTokens: int64(d.Credits),
				}
			}

		case kiroparser.EventTypeToolStart:
			// Tool start events are accumulated in processEvent
			if d, ok := event.Data.(kiroparser.ToolStartData); ok {
				log.Debugf("kiro stream: tool_start, name=%s", d.Name)
			}
		}
	}

	// If we have accumulated tool calls and received a non-tool event, emit them
	toolCalls := p.parser.GetToolCalls()
	if len(toolCalls) > 0 {
		var ktcs []kiroToolCall
		for _, tc := range toolCalls {
			ktcs = append(ktcs, kiroToolCall{
				ID:        tc.ID,
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			})
		}
		if len(chunk.Deltas) == 0 {
			chunk.Deltas = append(chunk.Deltas, canonical.ChoiceDelta{
				Index: 0,
				Delta: canonical.Message{
					Role: canonical.RoleAssistant,
				},
			})
		}
		chunk.Deltas[0].Delta.ToolCalls = kiroToolCallsToCanonical(ktcs)
	}

	if len(chunk.Deltas) == 0 {
		return nil, nil // No meaningful events
	}

	return chunk, nil
}

// ── Internal helpers ────────────────────────────────────────────────────

func (p *ProviderAdapter) processEvent(event kiroparser.Event) {
	switch event.Type {
	case kiroparser.EventTypeContent:
		if d, ok := event.Data.(kiroparser.ContentData); ok {
			p.accumulatedText.WriteString(d.Content)
		}
	case kiroparser.EventTypeThinking:
		if d, ok := event.Data.(kiroparser.ThinkingData); ok {
			p.thinkingText.WriteString(d.Content)
		}
	case kiroparser.EventTypeUsage:
		if d, ok := event.Data.(kiroparser.UsageData); ok {
			if p.usage == nil {
				p.usage = &canonical.Usage{}
			}
			p.usage.CompletionTokens = int64(d.Credits)
		}
	}
}

func (p *ProviderAdapter) buildFinalResponse() *canonical.Response {
	resp := &canonical.Response{
		ID:      p.responseID,
		Object:  "chat.completion",
		Created: p.created,
		Model:   p.modelName,
		Choices: []canonical.Choice{{
			Index: 0,
			Message: canonical.Message{
				Role: canonical.RoleAssistant,
			},
		}},
	}

	// Set content
	content := p.accumulatedText.String()
	if content != "" {
		resp.Choices[0].Message.Content = []canonical.ContentBlock{
			{Type: canonical.ContentText, Text: content},
		}
	}

	// Set reasoning
	if p.thinkingText.Len() > 0 {
		thinking := p.thinkingText.String()
		resp.Choices[0].Message.Reasoning = &thinking
	}

	// Set tool calls
	toolCalls := p.parser.GetToolCalls()
	if len(toolCalls) > 0 {
		var ktcs []kiroToolCall
		for _, tc := range toolCalls {
			ktcs = append(ktcs, kiroToolCall{
				ID:        tc.ID,
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			})
		}
		resp.Choices[0].Message.ToolCalls = kiroToolCallsToCanonical(ktcs)
	}

	// Set usage
	if p.usage != nil {
		resp.Usage = p.usage
	}

	// Set finish reason
	finishReason := "stop"
	if len(toolCalls) > 0 {
		finishReason = "tool_calls"
	}
	resp.Choices[0].FinishReason = &finishReason

	return resp
}
