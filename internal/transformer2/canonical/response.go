package canonical

// Response represents a canonical LLM response.
type Response struct {
	ID      string
	Model   string
	Created int64
	Object  string // "chat.completion", "list", etc.

	Choices    []Choice
	Embeddings []EmbeddingObject
	Usage      *Usage
	Error      *Error

	// HTTP status code from provider response (0 if not applicable)
	StatusCode int

	// System fingerprint (OpenAI)
	SystemFingerprint string
	ServiceTier       string
}

// Error represents a provider-agnostic error.
type Error struct {
	Code       string // machine-readable
	Message    string // human-readable description
	Type       string // error category
	StatusCode int    // HTTP status to return to client
}

// Chunk represents one SSE streaming event from any provider.
type Chunk struct {
	ID      string
	Model   string
	Created int64
	Deltas  []ChoiceDelta
	Usage   *Usage // final chunk may carry usage

	// Done signals stream termination.
	Done bool
}

// Choice represents a single choice in a response.
type Choice struct {
	Index        int
	Message      Message
	FinishReason *string // "stop","length","tool_calls","content_filter"
	Logprobs     *LogprobsContent

	// StopSequence holds the matched custom stop sequence text.
	StopSequence *string
}

// ChoiceDelta represents a streaming delta for a choice.
type ChoiceDelta struct {
	Index        int
	Delta        Message
	FinishReason *string
}

// LogprobsContent represents log probability information.
type LogprobsContent struct {
	Content []Logprobs
}

// Logprobs represents log probability for a token.
type Logprobs struct {
	Token   string
	LogProb float64
	TopLogprobs []TopLogprob
}

// TopLogprob represents top log probability alternative.
type TopLogprob struct {
	Token   string
	LogProb float64
}

// EmbeddingObject represents a single embedding in a response.
type EmbeddingObject struct {
	Object    string    // "embedding"
	Index     int
	Embedding []float64
}
