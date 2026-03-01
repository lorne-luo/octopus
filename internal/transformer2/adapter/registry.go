package adapter

import (
	"github.com/bestruirui/octopus/internal/transformer2/adapter/anthropic"
	"github.com/bestruirui/octopus/internal/transformer2/adapter/gemini"
	"github.com/bestruirui/octopus/internal/transformer2/adapter/openai"
	"github.com/bestruirui/octopus/internal/transformer2/adapter/volcengine"
)

// ClientType identifies the inbound client API format.
type ClientType int

const (
	ClientOpenAIChat     ClientType = iota
	ClientOpenAIResponse            // OpenAI Responses API (/v1/responses)
	ClientOpenAIEmbedding
	ClientAnthropic
)

// ProviderType identifies the outbound provider API format.
// CRITICAL: values MUST match existing outbound.OutboundType iota values.
type ProviderType int

const (
	ProviderOpenAIChat      ProviderType = 0
	ProviderOpenAIResponse  ProviderType = 1
	ProviderAnthropic       ProviderType = 2
	ProviderGemini          ProviderType = 3
	ProviderVolcengine      ProviderType = 4
	ProviderOpenAIEmbedding ProviderType = 5
)

// GetClient returns a ClientAdapter for the given client type.
func GetClient(t ClientType) ClientAdapter {
	switch t {
	case ClientOpenAIChat:
		return openai.NewChatClientAdapter()
	case ClientOpenAIResponse:
		return openai.NewResponsesClientAdapter()
	case ClientOpenAIEmbedding:
		return openai.NewEmbeddingClientAdapter()
	case ClientAnthropic:
		return anthropic.NewClientAdapter()
	default:
		return nil
	}
}

// GetProvider returns a ProviderAdapter for the given provider type.
func GetProvider(t ProviderType) ProviderAdapter {
	switch t {
	case ProviderOpenAIChat:
		return openai.NewChatProviderAdapter()
	case ProviderOpenAIResponse:
		return openai.NewResponsesProviderAdapter()
	case ProviderAnthropic:
		return anthropic.NewProviderAdapter()
	case ProviderGemini:
		return gemini.NewProviderAdapter()
	case ProviderVolcengine:
		return volcengine.NewProviderAdapter()
	case ProviderOpenAIEmbedding:
		return openai.NewEmbeddingProviderAdapter()
	default:
		return nil
	}
}

// IsChatProvider returns true if the provider type is a chat provider.
func IsChatProvider(t ProviderType) bool {
	return t == ProviderOpenAIChat ||
		t == ProviderOpenAIResponse ||
		t == ProviderAnthropic ||
		t == ProviderGemini ||
		t == ProviderVolcengine
}

// IsEmbeddingProvider returns true if the provider type is an embedding provider.
func IsEmbeddingProvider(t ProviderType) bool {
	return t == ProviderOpenAIEmbedding
}