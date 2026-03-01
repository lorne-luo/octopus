package openai

import "encoding/json"

// ChatCompletionRequest represents OpenAI Chat Completion API request.
type ChatCompletionRequest struct {
	Model               string           `json:"model"`
	Messages            []ChatMessage    `json:"messages"`
	Temperature         *float64         `json:"temperature,omitempty"`
	TopP                *float64         `json:"top_p,omitempty"`
	TopLogprobs         *int64           `json:"top_logprobs,omitempty"`
	MaxTokens           *int64           `json:"max_tokens,omitempty"`
	MaxCompletionTokens *int64           `json:"max_completion_tokens,omitempty"`
	FrequencyPenalty    *float64         `json:"frequency_penalty,omitempty"`
	PresencePenalty     *float64         `json:"presence_penalty,omitempty"`
	Seed                *int64           `json:"seed,omitempty"`
	Logprobs            *bool            `json:"logprobs,omitempty"`
	Store               *bool            `json:"store,omitempty"`
	LogitBias           map[string]int64 `json:"logit_bias,omitempty"`
	User                *string          `json:"user,omitempty"`
	ServiceTier         *string          `json:"service_tier,omitempty"`
	Stop                *StopSequences   `json:"stop,omitempty"`
	Stream              *bool            `json:"stream,omitempty"`
	StreamOptions       *StreamOptions   `json:"stream_options,omitempty"`
	Tools               []Tool           `json:"tools,omitempty"`
	ToolChoice          *ToolChoice      `json:"tool_choice,omitempty"`
	ParallelToolCalls   *bool            `json:"parallel_tool_calls,omitempty"`
	ResponseFormat      *ResponseFormat  `json:"response_format,omitempty"`
	Modalities          []string         `json:"modalities,omitempty"`
	Audio               *AudioConfig     `json:"audio,omitempty"`
	ReasoningEffort     *string          `json:"reasoning_effort,omitempty"`
	EnableThinking      *bool            `json:"enable_thinking,omitempty"`
}

// ChatMessage represents a message in a chat completion request.
type ChatMessage struct {
	Role             string         `json:"role"`
	Content          MessageContent `json:"content"`
	Name             *string        `json:"name,omitempty"`
	Refusal          string         `json:"refusal,omitempty"`
	ToolCalls        []ToolCall     `json:"tool_calls,omitempty"`
	ToolCallID       *string        `json:"tool_call_id,omitempty"`
	ReasoningContent *string        `json:"reasoning_content,omitempty"`
}

// MessageContent represents message content (string or array).
type MessageContent struct {
	Text  *string       `json:"text,omitempty"`
	Parts []ContentPart `json:"parts,omitempty"`
}

// ContentPart represents a content part in a message.
type ContentPart struct {
	Type     string    `json:"type"`
	Text     *string   `json:"text,omitempty"`
	ImageURL *ImageURL `json:"image_url,omitempty"`
	Audio    *Audio    `json:"input_audio,omitempty"`
	File     *File     `json:"file,omitempty"`
}

// ImageURL represents an image URL content part.
type ImageURL struct {
	URL    string  `json:"url"`
	Detail *string `json:"detail,omitempty"`
}

// Audio represents an audio content part.
type Audio struct {
	Format string `json:"format"`
	Data   string `json:"data"`
}

// File represents a file content part.
type File struct {
	Filename string `json:"filename"`
	FileData string `json:"file_data"`
}

// StopSequences represents stop sequences (string or array).
type StopSequences struct {
	Single   string
	Multiple []string
}

// StreamOptions represents stream options.
type StreamOptions struct {
	IncludeUsage bool `json:"include_usage,omitempty"`
}

// Tool represents a tool definition.
type Tool struct {
	Type            string           `json:"type"`
	Function        *FunctionDef     `json:"function,omitempty"`
	ImageGeneration *ImageGeneration `json:"image_generation,omitempty"`
}

// FunctionDef represents a function definition.
type FunctionDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
	Strict      *bool           `json:"strict,omitempty"`
}

// ImageGeneration represents image generation tool parameters.
type ImageGeneration struct {
	Background        string `json:"background,omitempty"`
	OutputFormat      string `json:"output_format,omitempty"`
	Quality           string `json:"quality,omitempty"`
	Size              string `json:"size,omitempty"`
	OutputCompression *int64 `json:"output_compression,omitempty"`
}

// ToolChoice represents tool choice (string or object).
type ToolChoice struct {
	Mode     string
	Function *string
}

// ToolCall represents a tool call in a message.
type ToolCall struct {
	ID       string       `json:"id,omitempty"`
	Type     string       `json:"type,omitempty"`
	Function FunctionCall `json:"function"`
	Index    int          `json:"index"`
}

// FunctionCall represents a function call.
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// JsonSchemaFormat represents the structured json_schema response format.
type JsonSchemaFormat struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Schema      json.RawMessage `json:"schema"`
	Strict      *bool           `json:"strict,omitempty"`
}

// ResponseFormat represents response format specification.
type ResponseFormat struct {
	Type       string            `json:"type"`
	JsonSchema *JsonSchemaFormat `json:"json_schema,omitempty"`
}

// AudioConfig represents audio output configuration.
type AudioConfig struct {
	Format string `json:"format,omitempty"`
	Voice  string `json:"voice,omitempty"`
}
