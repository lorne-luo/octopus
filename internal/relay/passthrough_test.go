package relay

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	dbmodel "github.com/bestruirui/octopus/internal/model"
	transformerModel "github.com/bestruirui/octopus/internal/transformer/model"
	"github.com/gin-gonic/gin"
)

func TestReplaceModelInBody(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		oldModel string
		newModel string
		want     string
	}{
		{
			name:     "replace model name",
			body:     `{"model":"gpt-4","messages":[{"role":"user","content":"hi"}]}`,
			oldModel: "gpt-4",
			newModel: "gpt-4-turbo",
			want:     `{"messages":[{"role":"user","content":"hi"}],"model":"gpt-4-turbo"}`,
		},
		{
			name:     "same model no replacement",
			body:     `{"model":"claude-3","messages":[]}`,
			oldModel: "claude-3",
			newModel: "claude-3",
			want:     `{"model":"claude-3","messages":[]}`,
		},
		{
			name:     "empty new model no replacement",
			body:     `{"model":"gpt-4","messages":[]}`,
			oldModel: "gpt-4",
			newModel: "",
			want:     `{"model":"gpt-4","messages":[]}`,
		},
		{
			name:     "preserves other fields",
			body:     `{"model":"old","temperature":0.7,"stream":true,"messages":[{"role":"user","content":"hello"}]}`,
			oldModel: "old",
			newModel: "new",
			want:     `{"messages":[{"role":"user","content":"hello"}],"model":"new","stream":true,"temperature":0.7}`,
		},
		{
			name:     "invalid json returns original",
			body:     `not json`,
			oldModel: "old",
			newModel: "new",
			want:     `not json`,
		},
		{
			name:     "anthropic format",
			body:     `{"model":"claude-3-opus","max_tokens":1024,"messages":[{"role":"user","content":[{"type":"text","text":"hi"}]}],"system":"You are helpful","thinking":{"type":"enabled","budget_tokens":5000}}`,
			oldModel: "claude-3-opus",
			newModel: "claude-3-5-sonnet",
			want:     `{"max_tokens":1024,"messages":[{"role":"user","content":[{"type":"text","text":"hi"}]}],"model":"claude-3-5-sonnet","system":"You are helpful","thinking":{"type":"enabled","budget_tokens":5000}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := string(replaceModelInBody([]byte(tt.body), tt.oldModel, tt.newModel))
			if got != tt.want {
				t.Errorf("replaceModelInBody() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExtractAndSetUsage(t *testing.T) {
	tests := []struct {
		name               string
		body               string
		expectInputTokens  int64
		expectOutputTokens int64
	}{
		{
			name:               "openai usage",
			body:               `{"id":"chatcmpl-123","choices":[],"usage":{"prompt_tokens":100,"completion_tokens":50}}`,
			expectInputTokens:  100,
			expectOutputTokens: 50,
		},
		{
			name:               "anthropic usage",
			body:               `{"id":"msg_123","content":[],"usage":{"input_tokens":200,"output_tokens":75,"cache_read_input_tokens":50,"cache_creation_input_tokens":10}}`,
			expectInputTokens:  200,
			expectOutputTokens: 75,
		},
		{
			name:               "no usage field",
			body:               `{"id":"chatcmpl-123","choices":[]}`,
			expectInputTokens:  0,
			expectOutputTokens: 0,
		},
		{
			name:               "invalid json",
			body:               `not json`,
			expectInputTokens:  0,
			expectOutputTokens: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metrics := &RelayMetrics{
				Stats: dbmodel.StatsMetrics{},
			}
			extractAndSetUsage(metrics, []byte(tt.body), "test-model")

			if metrics.Stats.InputToken != tt.expectInputTokens {
				t.Errorf("InputToken = %d, want %d", metrics.Stats.InputToken, tt.expectInputTokens)
			}
			if metrics.Stats.OutputToken != tt.expectOutputTokens {
				t.Errorf("OutputToken = %d, want %d", metrics.Stats.OutputToken, tt.expectOutputTokens)
			}
		})
	}
}

// MockInbound implements transformerModel.Inbound
type MockInbound struct {
	CapturedChunks []*transformerModel.InternalLLMResponse
}

func (m *MockInbound) TransformRequest(ctx context.Context, body []byte) (*transformerModel.InternalLLMRequest, error) {
	return nil, nil
}

func (m *MockInbound) TransformResponse(ctx context.Context, response *transformerModel.InternalLLMResponse) ([]byte, error) {
	return json.Marshal(response)
}

func (m *MockInbound) TransformStream(ctx context.Context, stream *transformerModel.InternalLLMResponse) ([]byte, error) {
	if stream == nil {
		return nil, nil
	}
	m.CapturedChunks = append(m.CapturedChunks, stream)
	return []byte("data: mock\n\n"), nil
}

func (m *MockInbound) GetInternalResponse(ctx context.Context) (*transformerModel.InternalLLMResponse, error) {
	if len(m.CapturedChunks) == 0 {
		return nil, nil
	}
	// Simple aggregation for testing
	fullContent := ""
	for _, chunk := range m.CapturedChunks {
		if len(chunk.Choices) > 0 && chunk.Choices[0].Delta != nil && chunk.Choices[0].Delta.Content.Content != nil {
			fullContent += *chunk.Choices[0].Delta.Content.Content
		}
	}

	return &transformerModel.InternalLLMResponse{
		Choices: []transformerModel.Choice{
			{
				Message: &transformerModel.Message{
					Content: transformerModel.MessageContent{Content: &fullContent},
				},
			},
		},
	}, nil
}

// MockOutbound implements transformerModel.Outbound
type MockOutbound struct{}

func (m *MockOutbound) TransformRequest(ctx context.Context, request *transformerModel.InternalLLMRequest, baseUrl, key string) (*http.Request, error) {
	return nil, nil
}

func (m *MockOutbound) TransformResponse(ctx context.Context, response *http.Response) (*transformerModel.InternalLLMResponse, error) {
	return nil, nil
}

func (m *MockOutbound) TransformStream(ctx context.Context, eventData []byte) (*transformerModel.InternalLLMResponse, error) {
	str := string(eventData)
	if str == "[DONE]" {
		return nil, nil
	}
	var resp transformerModel.InternalLLMResponse
	if err := json.Unmarshal(eventData, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func TestHandlePassthroughStreamResponse_Aggregation(t *testing.T) {
	// Setup Gin context
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = &http.Request{}
	c.Request = c.Request.WithContext(context.Background())

	// Setup mocks
	mockIn := &MockInbound{}
	mockOut := &MockOutbound{}

	// Setup metrics
	internalReq := &transformerModel.InternalLLMRequest{
		Model: "test-model",
	}
	metrics := NewRelayMetrics(1, "test-model", internalReq)

	// Setup relayAttempt
	ra := &relayAttempt{
		relayRequest: &relayRequest{
			c:               c,
			inAdapter:       mockIn,
			internalRequest: internalReq,
			metrics:         metrics,
			requestModel:    "test-model",
		},
		outAdapter: mockOut,
		channel: &dbmodel.Channel{
			ID:   1,
			Type: 1, // OpenAI type
		},
		usedKey: dbmodel.ChannelKey{
			ID: 1,
		},
	}

	// Setup pipe to simulate streaming response from upstream
	pr, pw := io.Pipe()
	resp := &http.Response{
		StatusCode: 200,
		Body:       pr,
		Header:     make(http.Header),
	}
	resp.Header.Set("Content-Type", "text/event-stream")

	// Start writing to pipe in goroutine
	go func() {
		defer pw.Close()
		events := []string{
			`{"id":"1","choices":[{"index":0,"delta":{"content":"Hello"}}]}`,
			`{"id":"1","choices":[{"index":0,"delta":{"content":" World"}}]}`,
			`[DONE]`,
		}

		for _, event := range events {
			// Write SSE format
			_, _ = io.WriteString(pw, "data: "+event+"\n\n")
			time.Sleep(10 * time.Millisecond)
		}
	}()

	// Execute handler
	err := ra.handlePassthroughStreamResponse(c.Request.Context(), resp)
	if err != nil {
		t.Fatalf("handlePassthroughStreamResponse failed: %v", err)
	}

	// Verify metrics.FinalResponse
	// GetInternalResponse in mock aggregates to "Hello World"
	// TransformResponse in mock marshals it to JSON
	// So we expect JSON containing "Hello World"

	var finalResp transformerModel.InternalLLMResponse
	if err := json.Unmarshal([]byte(metrics.FinalResponse), &finalResp); err != nil {
		t.Fatalf("failed to unmarshal FinalResponse: %v, raw: %s", err, metrics.FinalResponse)
	}

	if len(finalResp.Choices) == 0 || finalResp.Choices[0].Message == nil || finalResp.Choices[0].Message.Content.Content == nil {
		t.Fatalf("unexpected FinalResponse structure: %+v", finalResp)
	}

	got := *finalResp.Choices[0].Message.Content.Content
	want := "Hello World"
	if got != want {
		t.Errorf("FinalResponse content = %q, want %q", got, want)
	}
}
