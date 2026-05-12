package gemini

import (
	"encoding/json"
)

// GenerateContentRequest represents a Gemini generateContent API request.
type GenerateContentRequest struct {
	Contents         []Content          `json:"contents,omitempty"`
	SystemInstruction *Content          `json:"systemInstruction,omitempty"`
	Tools            []Tool             `json:"tools,omitempty"`
	ToolConfig       *ToolConfig        `json:"toolConfig,omitempty"`
	GenerationConfig *GenerationConfig  `json:"generationConfig,omitempty"`
	SafetySettings   []SafetySetting    `json:"safetySettings,omitempty"`
}

// Content represents a content message in Gemini format.
type Content struct {
	Role  string `json:"role,omitempty"` // "user" or "model"
	Parts []Part `json:"parts,omitempty"`
}

// Part represents a part of content in Gemini format.
type Part struct {
	// Text content
	Text string `json:"text,omitempty"`

	// Inline data (base64)
	InlineData *InlineData `json:"inlineData,omitempty"`

	// File data (URI reference)
	FileData *FileData `json:"fileData,omitempty"`

	// Function call (in model response)
	FunctionCall *FunctionCall `json:"functionCall,omitempty"`

	// Function response (in user message for tool results)
	FunctionResponse *FunctionResponse `json:"functionResponse,omitempty"`

	// Thought/reasoning content
	Thought bool `json:"thought,omitempty"`

	// Thought signature for reasoning continuity (Gemini 3)
	ThoughtSignature string `json:"thoughtSignature,omitempty"`

	// Thought summary (Gemini 3)
	ThoughtSummary string `json:"thoughtSummary,omitempty"`
}

// InlineData represents inline base64 data.
type InlineData struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"`
}

// FileData represents a file reference by URI.
type FileData struct {
	MimeType string `json:"mimeType"`
	FileURI  string `json:"fileUri"`
}

// FunctionCall represents a function call in Gemini format.
type FunctionCall struct {
	Name string          `json:"name"`
	Args json.RawMessage `json:"args,omitempty"`
}

// FunctionResponse represents a function response in Gemini format.
type FunctionResponse struct {
	Name     string      `json:"name"`
	Response interface{} `json:"response,omitempty"`
}

// Tool represents a tool definition in Gemini format.
type Tool struct {
	FunctionDeclarations []FunctionDeclaration `json:"functionDeclarations,omitempty"`
}

// FunctionDeclaration represents a function declaration in Gemini format.
type FunctionDeclaration struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

// ToolConfig represents tool configuration in Gemini format.
type ToolConfig struct {
	FunctionCallingConfig *FunctionCallingConfig `json:"functionCallingConfig,omitempty"`
}

// FunctionCallingConfig represents function calling configuration.
type FunctionCallingConfig struct {
	Mode                 string   `json:"mode,omitempty"` // "AUTO", "ANY", "NONE"
	AllowedFunctionNames []string `json:"allowedFunctionNames,omitempty"`
}

// GenerationConfig represents generation configuration in Gemini format.
type GenerationConfig struct {
	Temperature     *float64         `json:"temperature,omitempty"`
	TopP            *float64         `json:"topP,omitempty"`
	TopK            *int64           `json:"topK,omitempty"`
	MaxOutputTokens *int64           `json:"maxOutputTokens,omitempty"`
	StopSequences   []string         `json:"stopSequences,omitempty"`
	ResponseMimeType string          `json:"responseMimeType,omitempty"`
	ResponseSchema  json.RawMessage  `json:"responseSchema,omitempty"`

	// Thinking/Reasoning configuration (Gemini 3)
	ThinkingConfig *ThinkingConfig `json:"thinkingConfig,omitempty"`

	// Media resolution for images/videos
	MediaResolution string `json:"mediaResolution,omitempty"`
}

// ThinkingConfig represents thinking/reasoning configuration in Gemini format.
type ThinkingConfig struct {
	ThinkingLevel  string `json:"thinkingLevel,omitempty"`  // "minimal", "low", "medium", "high"
	ThinkingBudget *int64 `json:"thinkingBudget,omitempty"` // token budget for thinking
}

// SafetySetting represents a safety setting in Gemini format.
type SafetySetting struct {
	Category  string `json:"category"`
	Threshold string `json:"threshold"`
}

// GenerateContentResponse represents a Gemini generateContent API response.
type GenerateContentResponse struct {
	Candidates    []Candidate   `json:"candidates,omitempty"`
	UsageMetadata *UsageMetadata `json:"usageMetadata,omitempty"`
	ModelVersion  string        `json:"modelVersion,omitempty"`
}

// Candidate represents a candidate in a Gemini response.
type Candidate struct {
	Index         int           `json:"index,omitempty"`
	Content       *Content      `json:"content,omitempty"`
	FinishReason  string        `json:"finishReason,omitempty"`
	SafetyRatings []SafetyRating `json:"safetyRatings,omitempty"`
}

// SafetyRating represents a safety rating in Gemini format.
type SafetyRating struct {
	Category    string `json:"category"`
	Probability string `json:"probability"`
}

// UsageMetadata represents token usage in Gemini format.
type UsageMetadata struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`

	// Detailed token counts
	ThoughtsTokenCount     *int `json:"thoughtsTokenCount,omitempty"`
	CachedContentTokenCount *int `json:"cachedContentTokenCount,omitempty"`
}

// StreamChunk represents a streaming response chunk (same structure as GenerateContentResponse).
type StreamChunk struct {
	Candidates    []Candidate   `json:"candidates,omitempty"`
	UsageMetadata *UsageMetadata `json:"usageMetadata,omitempty"`
	ModelVersion  string        `json:"modelVersion,omitempty"`
}

// ErrorResponse represents a Gemini error response.
type ErrorResponse struct {
	Error *ErrorDetail `json:"error,omitempty"`
}

// ErrorDetail represents Gemini error details.
type ErrorDetail struct {
	Code    int    `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
	Status  string `json:"status,omitempty"`
}

// CountTokensRequest represents a Gemini countTokens API request.
type CountTokensRequest struct {
	Contents []Content `json:"contents,omitempty"`
}

// CountTokensResponse represents a Gemini countTokens API response.
type CountTokensResponse struct {
	TotalTokens int `json:"totalTokens"`
}