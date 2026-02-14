package relay

import (
	"testing"

	dbmodel "github.com/bestruirui/octopus/internal/model"
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
