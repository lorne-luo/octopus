package canonical

import (
	"encoding/json"
	"net/http"
	"net/url"
)

// RequestKind identifies the type of request.
type RequestKind int

const (
	KindChat       RequestKind = iota
	KindEmbedding
	KindCountTokens
)

// StopSequences represents stop sequences (can be single string or array).
type StopSequences struct {
	Single   string
	Multiple []string
}

// Request represents a canonical LLM request.
type Request struct {
	Kind  RequestKind
	Model string

	// Chat
	Messages   []Message
	Tools      []Tool
	ToolChoice *ToolChoice

	// Generation params
	Temperature         *float64
	TopP                *float64
	TopK                *int64
	MaxTokens           *int64 // Anthropic max_tokens, OpenAI legacy max_tokens
	MaxCompletionTokens *int64 // OpenAI max_completion_tokens
	Stop                *StopSequences
	FrequencyPenalty    *float64
	PresencePenalty     *float64
	Seed                *int64
	Logprobs            *bool
	TopLogprobs         *int64
	Store               *bool
	N                   *int // OpenAI number of completions

	// Modalities & Audio (OpenAI Chat)
	Modalities  []string // ["text"], ["text","audio"]
	AudioConfig *AudioConfig

	// Streaming
	Stream        bool
	StreamOptions *StreamOptions

	// Reasoning/Thinking (unified across providers)
	Reasoning *ReasoningConfig

	// Response format
	ResponseFormat *ResponseFormat
	// Anthropic output_config
	OutputConfig *OutputConfig

	// Embedding Specific
	EmbeddingInput          *EmbeddingInput
	EmbeddingDimensions     *int64
	EmbeddingEncodingFormat *string

	// Gemini: media_resolution
	MediaResolution *string

	// OpenAI-specific passthrough fields
	LogitBias       map[string]int64
	ServiceTier     *string
	PromptCacheKey  *bool
	SafetyIdentifier *string
	User            *string

	// Qwen specific
	EnableThinking *bool

	// Provider extension escape hatch
	ExtraBody json.RawMessage

	// Passthrough metadata
	SourceFormat APIFormat
	Metadata     map[string]string
	Query        url.Values

	// Provider-specific HTTP headers
	Headers http.Header

	// Raw inbound request body (for logging/debugging)
	RawRequest []byte

	// TransformHints for adapter-internal hints
	Hints TransformHints
}

// APIFormat identifies the source API format.
type APIFormat string

const (
	FormatOpenAIChat    APIFormat = "openai/chat"
	FormatOpenAIResponse APIFormat = "openai/responses"
	FormatOpenAIEmbed   APIFormat = "openai/embeddings"
	FormatAnthropic     APIFormat = "anthropic/messages"
	FormatGemini        APIFormat = "gemini"
)

// TransformHints carries adapter-internal metadata for round-trip fidelity.
type TransformHints struct {
	// ResponsesArrayInput: true when OpenAI Responses API input was array format.
	ResponsesArrayInput *bool

	// Include: additional output data requested by client.
	Include []string

	// AnthropicSystemArrayFormat: true when Anthropic system prompt was array.
	AnthropicSystemArrayFormat bool

	// GeminiTopK: preserved from Gemini request for round-trip.
	GeminiTopK *int

	// GeminiSafetySettings: preserved as raw JSON from Gemini request.
	GeminiSafetySettings json.RawMessage
}

// AudioConfig for OpenAI Chat audio output.
type AudioConfig struct {
	Format string // "wav","mp3","flac","opus","pcm16"
	Voice  string // "alloy","ash","coral", etc.
}

// OutputConfig for Anthropic output configuration.
type OutputConfig struct {
	Format *OutputFormat
}

// OutputFormat represents Anthropic output format.
type OutputFormat struct {
	Type   string          // "json"
	Schema json.RawMessage // JSON Schema
}

// ReasoningConfig unifies thinking/reasoning control across providers.
type ReasoningConfig struct {
	// Effort level — unified across providers.
	Effort *string

	// BudgetTokens — explicit token budget for thinking.
	BudgetTokens *int64

	// Enabled — thinking mode toggle.
	Enabled *bool
}

// StreamOptions for OpenAI streaming.
type StreamOptions struct {
	IncludeUsage bool
}

// ResponseFormat for OpenAI response format specification.
type ResponseFormat struct {
	Type       string          // "json_object", "json_schema", "text"
	JsonSchema *ResponseSchema // when Type == "json_schema"
}

// ResponseSchema for JSON schema response format.
type ResponseSchema struct {
	Name        string          // schema name
	Description string          // schema description
	Schema      json.RawMessage // JSON Schema definition
	Strict      bool            // strict schema validation
}

// EmbeddingInput represents embedding input (can be string or array).
type EmbeddingInput struct {
	Text   string
	Texts  []string
	Tokens []int64
}
