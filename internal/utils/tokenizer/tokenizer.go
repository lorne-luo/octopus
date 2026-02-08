package tokenizer

import (
	"strings"
	"sync"

	anthropic "github.com/qhenkart/anthropic-tokenizer-go"
	tiktoken "github.com/tiktoken-go/tokenizer"
)

var (
	anthropicTokenizer *anthropic.Tokenizer
	anthropicInitOnce  sync.Once
)

func getAnthropicTokenizer() *anthropic.Tokenizer {
	anthropicInitOnce.Do(func() {
		t, err := anthropic.New()
		if err != nil {
			// If initialization fails, we'll leave anthropicTokenizer as nil
			// and fall back to OpenAI tokenizer.
			return
		}
		anthropicTokenizer = t
	})
	return anthropicTokenizer
}

// CountTokens counts the number of tokens in the content for the given model.
// It supports OpenAI and Anthropic models directly.
// For other models (e.g., Gemini), it falls back to OpenAI's cl100k_base encoding as an approximation.
func CountTokens(content, model string) int {
	if content == "" {
		return 0
	}

	// Anthropic models
	if isAnthropicModel(model) {
		t := getAnthropicTokenizer()
		if t != nil {
			return t.Tokens(content)
		}
		// Fallback to OpenAI tokenizer if Anthropic tokenizer implementation is not available
		return countOpenAITokens(content, model)
	}

	// OpenAI models and fallback for others
	return countOpenAITokens(content, model)
}

func isAnthropicModel(model string) bool {
	return strings.Contains(strings.ToLower(model), "claude")
}

func countOpenAITokens(content, model string) int {
	// Try to get encoding for the specific model
	enc, err := tiktoken.ForModel(tiktoken.Model(model))
	if err != nil {
		// Fallback to cl100k_base (GPT-4) as a reasonable approximation for modern models
		enc, err = tiktoken.Get(tiktoken.O200kBase)
		if err != nil {
			return 0
		}
	}

	ids, _, err := enc.Encode(content)
	if err != nil {
		return 0
	}
	return len(ids)
}
