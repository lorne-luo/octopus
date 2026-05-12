package canonical

// Usage unifies token counts from all providers.
type Usage struct {
	PromptTokens     int64
	CompletionTokens int64
	TotalTokens      int64

	PromptTokensDetails     *PromptTokensDetails
	CompletionTokensDetails *CompletionTokensDetails

	// Anthropic cache-specific (totals)
	CacheCreationInputTokens int64
	CacheReadInputTokens     int64

	// Anthropic cache creation breakdown by TTL
	CacheCreation *CacheCreationDetails

	// Anthropic-specific: original input_tokens from API response.
	InputTokensAfterBreakpoint int64

	// Anthropic server tool usage
	ServerToolUsage *ServerToolUsage

	// Internal flag for adapter-specific behavior
	IsAnthropicUsage bool
}

// CacheCreationDetails breaks down cache creation tokens by TTL duration.
type CacheCreationDetails struct {
	Ephemeral5mInputTokens int64
	Ephemeral1hInputTokens int64
}

// ServerToolUsage tracks Anthropic server-side tool invocations.
type ServerToolUsage struct {
	WebSearchRequests int64
	WebFetchRequests  int64
}

// PromptTokensDetails contains prompt token breakdown.
type PromptTokensDetails struct {
	CachedTokens int64
	AudioTokens  int64
}

// CompletionTokensDetails contains completion token breakdown.
type CompletionTokensDetails struct {
	ReasoningTokens         int64
	AudioTokens             int64
	AcceptedPredictionTokens int64
	RejectedPredictionTokens int64
}
