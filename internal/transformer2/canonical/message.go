package canonical

import "encoding/json"

// Role represents the role of a message sender.
type Role string

const (
	RoleSystem    Role = "system"
	RoleDeveloper Role = "developer" // OpenAI developer role
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// ContentType represents the type of content in a message.
type ContentType string

const (
	ContentText     ContentType = "text"
	ContentImage    ContentType = "image"
	ContentAudio    ContentType = "audio"
	ContentFile     ContentType = "file"
	ContentDocument ContentType = "document" // Anthropic document block (PDF)
	ContentThinking ContentType = "thinking" // Anthropic thinking block (interleaved thinking)
)

// CacheControl represents cache control settings for Anthropic prompt caching.
type CacheControl struct {
	Type string `json:"type"` // "ephemeral"
}

// Message represents a single message in a conversation.
type Message struct {
	Role    Role
	Content []ContentBlock
	Name    *string
	Refusal string

	// Tool call (assistant requesting tool execution)
	ToolCalls []ToolCall
	// Tool result (tool responding)
	ToolCallID *string

	// Tool call helper fields (not serialized, used by adapters)
	MessageIndex    *int
	ToolCallName    *string
	ToolCallIsError *bool

	// Reasoning/Thinking output
	Reasoning          *string
	ReasoningSignature *string // Gemini thoughtSignature, Anthropic signature
	ThoughtSummary     *string // Gemini thought_summary
	RedactedThinking   *string // Anthropic redacted_thinking.data (opaque passthrough)

	// Provider-specific passthrough
	CacheControl *CacheControl

	// Server tool content blocks (Anthropic: web_search_tool_result, etc.)
	ServerToolBlocks []json.RawMessage

	// Image generation results (merged into Content during processing)
	Images []ContentBlock
}

// ContentBlock represents a single content block in a message.
type ContentBlock struct {
	Type  ContentType
	Text  string        // when Type == ContentText
	Media *MediaContent // when Type == ContentImage/Audio/File/Document

	// Thinking content (Anthropic extended thinking, OpenAI reasoning)
	// Present when Type == ContentThinking
	Thinking  string // the thinking/reasoning text
	Signature string // Anthropic thinking block signature (required for multi-turn)

	// Provider-specific passthrough
	CacheControl *CacheControl

	// Raw JSON for unsupported/passthrough content types
	RawJSON []byte
}

// MediaContent unifies multimodal content across providers.
type MediaContent struct {
	// Image
	URL      string  // URL-based image (OpenAI image_url.url, Anthropic url source, Gemini fileUri)
	Base64   string  // base64-encoded image data
	MimeType string  // MIME type (image/jpeg, image/png, etc.)
	Detail   *string // OpenAI detail level: "auto"|"low"|"high"

	// Audio
	AudioFormat string // "wav"|"mp3"
	AudioData   string // base64-encoded audio

	// File / Document
	FileName string
	FileData string // base64-encoded file content
	FileID   string // provider-specific file ID reference
	FileURL  string // URL to file
}
