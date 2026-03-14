package canonical

import "encoding/json"

// Tool unifies tool definitions across providers.
type Tool struct {
	Type        string          // "function", "image_generation"
	Name        string
	Description string
	Parameters  json.RawMessage // JSON Schema
	Strict      *bool           // OpenAI & Anthropic strict mode (schema validation)

	ImageGeneration *ImageGenerationConfig
	CacheControl    *CacheControl

	// Server/built-in tools (passthrough as opaque JSON)
	RawJSON json.RawMessage
}

// ToolCall represents a tool call in a message.
type ToolCall struct {
	ID        string
	Type      string // "function"
	Name      string
	Arguments string // JSON string (Gemini: marshal map[string]any to JSON string)
	Index     int
	CacheControl *CacheControl

	// Gemini thoughtSignature for function call round-trip.
	Signature *string
}

// ImageGenerationConfig carries parameters for image_generation tool type.
type ImageGenerationConfig struct {
	Background        string // "opaque", "transparent"
	OutputFormat      string // "png", "jpeg", "webp"
	Quality           string // "standard", "hd"
	Size              string // "1024x1024", "1792x1024", etc.
	OutputCompression *int64 // compression level
}

// ToolChoice unifies tool selection across providers.
type ToolChoice struct {
	Mode     string  // "auto", "none", "required", "tool"
	Function *string // specific function name (when Mode == "tool")

	// Unified parallel tool call control.
	DisableParallelToolUse *bool
}
