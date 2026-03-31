package openai

// ChatCompletionResponse represents OpenAI Chat Completion API response.
type ChatCompletionResponse struct {
	ID                string         `json:"id"`
	Object            string         `json:"object"`
	Created           int64          `json:"created"`
	Model             string         `json:"model"`
	Choices           []ChatChoice   `json:"choices"`
	Usage             *Usage         `json:"usage,omitempty"`
	SystemFingerprint string         `json:"system_fingerprint,omitempty"`
	ServiceTier       string         `json:"service_tier,omitempty"`
}

// ChatChoice represents a choice in a chat completion response.
type ChatChoice struct {
	Index        int           `json:"index"`
	Message      *ChatMessage  `json:"message,omitempty"`
	Delta        *ChatMessage  `json:"delta,omitempty"`
	FinishReason *string       `json:"finish_reason,omitempty"`
	Logprobs     *Logprobs     `json:"logprobs,omitempty"`
}

// Logprobs represents log probability information.
type Logprobs struct {
	Content []TokenLogprob `json:"content"`
}

// TokenLogprob represents log probability for a token.
type TokenLogprob struct {
	Token       string        `json:"token"`
	Logprob     float64       `json:"logprob"`
	Bytes       []int         `json:"bytes,omitempty"`
	TopLogprobs []TopLogprob  `json:"top_logprobs,omitempty"`
}

// TopLogprob represents top alternative tokens.
type TopLogprob struct {
	Token   string  `json:"token"`
	Logprob float64 `json:"logprob"`
	Bytes   []int   `json:"bytes,omitempty"`
}

// Usage represents token usage information.
type Usage struct {
	PromptTokens           int64               `json:"prompt_tokens"`
	CompletionTokens       int64               `json:"completion_tokens"`
	TotalTokens            int64               `json:"total_tokens"`
	PromptTokensDetails    *PromptTokensDetails `json:"prompt_tokens_details,omitempty"`
	CompletionTokensDetails *CompletionTokensDetails `json:"completion_tokens_details,omitempty"`
}

// PromptTokensDetails represents prompt token breakdown.
type PromptTokensDetails struct {
	CachedTokens int64 `json:"cached_tokens,omitempty"`
	AudioTokens  int64 `json:"audio_tokens,omitempty"`
}

// CompletionTokensDetails represents completion token breakdown.
type CompletionTokensDetails struct {
	ReasoningTokens          int64 `json:"reasoning_tokens,omitempty"`
	AudioTokens              int64 `json:"audio_tokens,omitempty"`
	AcceptedPredictionTokens int64 `json:"accepted_prediction_tokens,omitempty"`
	RejectedPredictionTokens int64 `json:"rejected_prediction_tokens,omitempty"`
}

// ErrorResponse represents OpenAI error response.
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail represents error details.
type ErrorDetail struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message"`
	Type    string `json:"type"`
	Param   string `json:"param,omitempty"`
}