package tokenizer

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCountTokens(t *testing.T) {
	tests := []struct {
		name    string
		content string
		model   string
		wantMin int // Minimum expected tokens (to allow for slight varations if implementations change)
	}{
		{
			name:    "Empty content",
			content: "",
			model:   "gpt-4",
			wantMin: 0,
		},
		{
			name:    "OpenAI GPT-4",
			content: "Hello, world!",
			model:   "gpt-4",
			wantMin: 3, // "Hello", ",", " world", "!" -> likely 4, but > 0
		},
		{
			name:    "OpenAI GPT-3.5",
			content: "Hello, world!",
			model:   "gpt-3.5-turbo",
			wantMin: 3,
		},
		{
			name:    "Anthropic Claude 3",
			content: "Hello, world!",
			model:   "claude-3-opus-20240229",
			wantMin: 3,
		},
		{
			name:    "Anthropic Generic",
			content: "Hello, world!",
			model:   "claude-2",
			wantMin: 3,
		},
		{
			name:    "Gemini (Fallback)",
			content: "Hello, world!",
			model:   "gemini-pro",
			wantMin: 3,
		},
		{
			name:    "Chinese Content",
			content: "你好，世界！",
			model:   "gpt-4",
			wantMin: 3, // specific counting might vary, but should be > 0
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CountTokens(tt.content, tt.model)
			if tt.content == "" {
				assert.Equal(t, 0, got)
			} else {
				assert.GreaterOrEqual(t, got, tt.wantMin, "Token count should be at least %d for model %s", tt.wantMin, tt.model)
			}
		})
	}
}

func TestIsAnthropicModel(t *testing.T) {
	tests := []struct {
		model string
		want  bool
	}{
		{"claude-3-opus", true},
		{"claude-2", true},
		{"CLAUDE-INSTANT", true},
		{"gpt-4", false},
		{"gemini-pro", false},
		{"random-model", false},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.want, isAnthropicModel(tt.model), "isAnthropicModel(%s)", tt.model)
	}
}
