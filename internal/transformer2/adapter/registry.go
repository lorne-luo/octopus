package adapter

import (
	"github.com/bestruirui/octopus/internal/transformer2/adapter/openai"
)

// ClientType identifies the inbound client API format.
type ClientType int

const (
	ClientOpenAIChat ClientType = iota
	ClientOpenAIResponse // OpenAI Responses API (/v1/responses)
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
		return openai.NewChatClientAdapter() // TODO: Add ResponsesClientAdapter
	case ClientOpenAIEmbedding:
		return openai.NewChatClientAdapter() // Embedding uses similar format
	case ClientAnthropic:
		// TODO: return anthropic.NewClientAdapter()
		return nil
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
		return openai.NewChatProviderAdapter() // TODO: Add ResponsesProviderAdapter
	case ProviderAnthropic:
		// TODO: return anthropic.NewProviderAdapter()
		return nil
	case ProviderGemini:
		// TODO: return gemini.NewProviderAdapter()
		return nil
	case ProviderVolcengine:
		// TODO: return volcengine.NewProviderAdapter()
		return nil
	case ProviderOpenAIEmbedding:
		return openai.NewChatProviderAdapter() // Embedding uses similar format
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
