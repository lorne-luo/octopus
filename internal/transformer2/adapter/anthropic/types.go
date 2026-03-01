package anthropic

import (
	"encoding/json"
)

// MessageRequest represents an Anthropic Messages API request.
type MessageRequest struct {
	Model         string            `json:"model"`
	MaxTokens     int64             `json:"max_tokens"`
	Messages      []MessageParam    `json:"messages"`
	System        SystemContent     `json:"system,omitempty"`
	Tools         []Tool            `json:"tools,omitempty"`
	ToolChoice    *ToolChoice       `json:"tool_choice,omitempty"`
	Thinking      *ThinkingConfig   `json:"thinking,omitempty"`
	Temperature   *float64          `json:"temperature,omitempty"`
	TopP          *float64          `json:"top_p,omitempty"`
	TopK          *int64            `json:"top_k,omitempty"`
	StopSequences []string          `json:"stop_sequences,omitempty"`
	Stream        bool              `json:"stream,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty"`

	// Anthropic output_config (effort control for adaptive thinking)
	OutputConfig *AnthropicOutputConfig `json:"output_config,omitempty"`

	// Anthropic-specific features
	MetadataUserID *string `json:"-"` // Extracted from metadata.user_id
}

// SystemContent represents system prompt (string or array).
type SystemContent struct {
	Text   string
	Blocks []SystemBlock
}

// UnmarshalJSON implements custom JSON unmarshaling for SystemContent.
func (s *SystemContent) UnmarshalJSON(data []byte) error {
	// Try string first
	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		s.Text = text
		return nil
	}

	// Try array
	var blocks []SystemBlock
	if err := json.Unmarshal(data, &blocks); err != nil {
		return err
	}
	s.Blocks = blocks
	return nil
}

// MarshalJSON implements custom JSON marshaling for SystemContent.
func (s SystemContent) MarshalJSON() ([]byte, error) {
	if s.Text != "" {
		return json.Marshal(s.Text)
	}
	if len(s.Blocks) > 0 {
		return json.Marshal(s.Blocks)
	}
	return json.Marshal(nil)
}

// SystemBlock represents a block in system prompt array.
type SystemBlock struct {
	Type         string        `json:"type"`
	Text         string        `json:"text"`
	CacheControl *CacheControl `json:"cache_control,omitempty"`
}

// MessageParam represents a message in Anthropic format.
type MessageParam struct {
	Role    string         `json:"role"`
	Content MessageContent `json:"content"`
}

// MessageContent represents message content (string or array).
type MessageContent struct {
	Text   string
	Blocks []ContentBlock
}

// UnmarshalJSON implements custom JSON unmarshaling for MessageContent.
func (m *MessageContent) UnmarshalJSON(data []byte) error {
	// Try string first
	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		m.Text = text
		return nil
	}

	// Try array
	var blocks []ContentBlock
	if err := json.Unmarshal(data, &blocks); err != nil {
		return err
	}
	m.Blocks = blocks
	return nil
}

// MarshalJSON implements custom JSON marshaling for MessageContent.
func (m MessageContent) MarshalJSON() ([]byte, error) {
	if m.Text != "" {
		return json.Marshal(m.Text)
	}
	if len(m.Blocks) > 0 {
		return json.Marshal(m.Blocks)
	}
	return json.Marshal(nil)
}

// String returns the text content if available.
func (m MessageContent) String() string {
	return m.Text
}

// ContentBlock represents a content block in Anthropic message.
type ContentBlock struct {
	Type string `json:"type"`

	// Text content
	Text string `json:"text,omitempty"`

	// Image content
	Source *ImageSource `json:"source,omitempty"`

	// Tool use
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`

	// Tool result
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`

	// Thinking content (extended thinking)
	Thinking  string `json:"thinking,omitempty"`
	Signature string `json:"signature,omitempty"`

	// Cache control
	CacheControl *CacheControl `json:"cache_control,omitempty"`
}

// ImageSource represents an image source in Anthropic format.
type ImageSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data,omitempty"`
	URL       string `json:"url,omitempty"`
}

// CacheControl represents Anthropic cache control.
type CacheControl struct {
	Type string `json:"type"` // "ephemeral"
}

// ThinkingConfig represents Anthropic thinking configuration.
type ThinkingConfig struct {
	Type         string `json:"type"` // "enabled" or "disabled"
	BudgetTokens int64  `json:"budget_tokens,omitempty"`
}

// Tool represents an Anthropic tool definition.
type Tool struct {
	Name         string          `json:"name"`
	Description  string          `json:"description,omitempty"`
	InputSchema  json.RawMessage `json:"input_schema"`
	CacheControl *CacheControl   `json:"cache_control,omitempty"`
}

// ToolChoice represents Anthropic tool choice.
type ToolChoice struct {
	Type                   string `json:"type"`           // "auto", "any", "tool"
	Name                   string `json:"name,omitempty"` // when Type == "tool"
	DisableParallelToolUse bool   `json:"disable_parallel_tool_use,omitempty"`
}

// MessageResponse represents an Anthropic Messages API response.
type MessageResponse struct {
	ID           string         `json:"id"`
	Type         string         `json:"type"` // "message"
	Role         string         `json:"role"` // "assistant"
	Model        string         `json:"model"`
	Content      []ContentBlock `json:"content"`
	StopReason   string         `json:"stop_reason"`
	StopSequence string         `json:"stop_sequence,omitempty"`
	Usage        Usage          `json:"usage"`
}

// Usage represents Anthropic token usage.
type Usage struct {
	InputTokens              int64 `json:"input_tokens"`
	OutputTokens             int64 `json:"output_tokens"`
	CacheCreationInputTokens int64 `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int64 `json:"cache_read_input_tokens,omitempty"`
}

// ErrorResponse represents an Anthropic error response.
type ErrorResponse struct {
	Type  string      `json:"type"` // "error"
	Error ErrorDetail `json:"error"`
}

// ErrorDetail represents Anthropic error details.
type ErrorDetail struct {
	Type    string `json:"type"` // "invalid_request_error", "authentication_error", etc.
	Message string `json:"message"`
}

// StreamEvent represents an Anthropic SSE event.
type StreamEvent struct {
	Type string `json:"type"`

	// message_start
	Message *MessageResponse `json:"message,omitempty"`

	// content_block_start
	Index        int           `json:"index,omitempty"`
	ContentBlock *ContentBlock `json:"content_block,omitempty"`

	// For content_block_delta and message_delta
	// We use RawDelta to capture the delta field, then parse based on event type
	DeltaRaw json.RawMessage `json:"delta,omitempty"`

	// Parsed deltas - populated after unmarshaling based on event type
	Delta        *ContentDelta    `json:"-"`
	DeltaMessage *MessageDeltaRaw `json:"-"`

	// message_delta usage field
	Usage *Usage `json:"usage,omitempty"`
}

// UnmarshalJSON implements custom JSON unmarshaling for StreamEvent.
func (e *StreamEvent) UnmarshalJSON(data []byte) error {
	// Use an alias to avoid infinite recursion
	type Alias StreamEvent
	aux := &struct {
		*Alias
	}{
		Alias: (*Alias)(e),
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	// Parse delta based on event type
	if len(e.DeltaRaw) > 0 {
		switch e.Type {
		case EventTypeContentBlockDelta:
			var delta ContentDelta
			if err := json.Unmarshal(e.DeltaRaw, &delta); err == nil {
				e.Delta = &delta
			}
		case EventTypeMessageDelta:
			var delta MessageDeltaRaw
			if err := json.Unmarshal(e.DeltaRaw, &delta); err == nil {
				e.DeltaMessage = &delta
			}
		}
	}

	return nil
}

// MessageDeltaRaw captures the raw delta from message_delta events.
// Note: this uses a different struct to avoid collision with the `delta` field
// which is already used by ContentDelta.
type MessageDeltaRaw struct {
	StopReason   string `json:"stop_reason,omitempty"`
	StopSequence string `json:"stop_sequence,omitempty"`
}

// ContentDelta represents a content delta in streaming.
type ContentDelta struct {
	Type string `json:"type"` // "text_delta", "input_json_delta", "thinking_delta", "signature_delta"

	Text        string `json:"text,omitempty"`
	PartialJSON string `json:"partial_json,omitempty"`
	Thinking    string `json:"thinking,omitempty"`
	Signature   string `json:"signature,omitempty"`
}

// MessageDelta represents message-level delta.
type MessageDelta struct {
	StopReason   string `json:"stop_reason,omitempty"`
	StopSequence string `json:"stop_sequence,omitempty"`
}

// Streaming event types as constants.
const (
	EventTypeMessageStart      = "message_start"
	EventTypeContentBlockStart = "content_block_start"
	EventTypeContentBlockDelta = "content_block_delta"
	EventTypeContentBlockStop  = "content_block_stop"
	EventTypeMessageDelta      = "message_delta"
	EventTypeMessageStop       = "message_stop"
	EventTypePing              = "ping"
	EventTypeError             = "error"
)

// Content block types as constants.
const (
	ContentTypeText       = "text"
	ContentTypeImage      = "image"
	ContentTypeToolUse    = "tool_use"
	ContentTypeToolResult = "tool_result"
	ContentTypeThinking   = "thinking"
)

// Finish reasons as constants.
const (
	StopReasonEndTurn      = "end_turn"
	StopReasonMaxTokens    = "max_tokens"
	StopReasonToolUse      = "tool_use"
	StopReasonStopSequence = "stop_sequence"
)

// AnthropicOutputConfig represents Anthropic's output_config field.
// Used for effort control in adaptive thinking mode.
// Maps to: output_config: {effort: "low"|"medium"|"high"|"max"}
type AnthropicOutputConfig struct {
	Effort string `json:"effort,omitempty"`
}

// CountTokensResponse represents Anthropic's /v1/messages/count_tokens response.
type CountTokensResponse struct {
	InputTokens int64 `json:"input_tokens"`
}
