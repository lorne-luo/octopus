package openai

import "encoding/json"

// ResponsesRequest represents OpenAI Responses API request.
// Covers /v1/responses (POST).
type ResponsesRequest struct {
	Model             string                 `json:"model"`
	Input             ResponsesInput         `json:"input"`
	Instructions      *string                `json:"instructions,omitempty"`
	Tools             []ResponsesTool        `json:"tools,omitempty"`
	ToolChoice        *ResponsesToolChoice   `json:"tool_choice,omitempty"`
	ParallelToolCalls *bool                  `json:"parallel_tool_calls,omitempty"`
	Reasoning         *ResponsesReasoning    `json:"reasoning,omitempty"`
	Text              *ResponsesText         `json:"text,omitempty"`
	Include           []string               `json:"include,omitempty"`
	MaxOutputTokens   *int64                 `json:"max_output_tokens,omitempty"`
	Temperature       *float64               `json:"temperature,omitempty"`
	TopP              *float64               `json:"top_p,omitempty"`
	Store             *bool                  `json:"store,omitempty"`
	ServiceTier       *string                `json:"service_tier,omitempty"`
	User              *string                `json:"user,omitempty"`
	Metadata          map[string]interface{} `json:"metadata,omitempty"`
	Stream            *bool                  `json:"stream,omitempty"`
}

// ResponsesInput represents input which can be string or array of items.
type ResponsesInput struct {
	Text  string               `json:"text,omitempty"`
	Items []ResponsesInputItem `json:"items,omitempty"`
}

// IsString returns true if input is a simple string.
func (ri ResponsesInput) IsString() bool {
	return ri.Text != "" && len(ri.Items) == 0
}

// MarshalJSON handles ResponsesInput serialization.
func (ri ResponsesInput) MarshalJSON() ([]byte, error) {
	if ri.Text != "" && len(ri.Items) == 0 {
		return json.Marshal(ri.Text)
	}
	return json.Marshal(ri.Items)
}

// UnmarshalJSON handles both string and array forms of ResponsesInput.
func (ri *ResponsesInput) UnmarshalJSON(data []byte) error {
	// Try string first
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		ri.Text = str
		return nil
	}

	// Try array of items
	var items []ResponsesInputItem
	if err := json.Unmarshal(data, &items); err != nil {
		return err
	}
	ri.Items = items
	return nil
}

// ResponsesInputItem represents an item in the input array.
type ResponsesInputItem struct {
	// For message items
	Type    string              `json:"type,omitempty"`
	Role    string              `json:"role,omitempty"`
	Content []ResponsesContent  `json:"content,omitempty"`

	// For function_call items
	CallID    string `json:"call_id,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`

	// For function_call_output items
	Output string `json:"output,omitempty"`

	// Status for partial responses
	Status string `json:"status,omitempty"`

	// Additional fields for partial assistant messages
	Partial *bool `json:"partial,omitempty"`
}

// ResponsesContent represents content in an input/output item.
type ResponsesContent struct {
	Type string `json:"type"`

	// For input_text/output_text
	Text string `json:"text,omitempty"`

	// For input_image
	ImageURL    string `json:"image_url,omitempty"`
	Detail      string `json:"detail,omitempty"`
	FileID      string `json:"file_id,omitempty"`
	FileData    string `json:"file_data,omitempty"`
	MimeType    string `json:"mime_type,omitempty"`

	// For refusal
	Refusal string `json:"refusal,omitempty"`
}

// ResponsesTool represents a tool definition in Responses API.
type ResponsesTool struct {
	Type string `json:"type"`

	// For function type
	Name        string          `json:"name,omitempty"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
	Strict      *bool           `json:"strict,omitempty"`

	// For image_generation type
	Background        string `json:"background,omitempty"`
	OutputFormat      string `json:"output_format,omitempty"`
	Quality           string `json:"quality,omitempty"`
	Size              string `json:"size,omitempty"`
	OutputCompression *int64 `json:"output_compression,omitempty"`
}

// ResponsesToolChoice represents tool choice in Responses API.
type ResponsesToolChoice struct {
	Mode string `json:"mode,omitempty"`
	Type string `json:"type,omitempty"`
	Name string `json:"name,omitempty"`
}

// MarshalJSON handles ResponsesToolChoice serialization.
func (tc ResponsesToolChoice) MarshalJSON() ([]byte, error) {
	if tc.Mode != "" && tc.Name == "" {
		return json.Marshal(tc.Mode)
	}
	// Object form for named tool
	return json.Marshal(struct {
		Type string `json:"type"`
		Name string `json:"name"`
	}{
		Type: tc.Type,
		Name: tc.Name,
	})
}

// UnmarshalJSON handles both string and object forms of ResponsesToolChoice.
func (tc *ResponsesToolChoice) UnmarshalJSON(data []byte) error {
	// Try string first
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		tc.Mode = str
		return nil
	}

	// Try object form
	var obj struct {
		Type string `json:"type"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(data, &obj); err != nil {
		return err
	}
	tc.Type = obj.Type
	tc.Name = obj.Name
	tc.Mode = "tool"
	return nil
}

// ResponsesReasoning represents reasoning configuration.
type ResponsesReasoning struct {
	Effort string `json:"effort,omitempty"` // "none"|"minimal"|"low"|"medium"|"high"
}

// ResponsesText represents text format configuration.
type ResponsesText struct {
	Format *ResponsesTextFormat `json:"format,omitempty"`
}

// ResponsesTextFormat represents text format specification.
type ResponsesTextFormat struct {
	Type string `json:"type"` // "text"|"json_object"|"json_schema"

	// For json_schema
	Name        string          `json:"name,omitempty"`
	Description string          `json:"description,omitempty"`
	Schema      json.RawMessage `json:"schema,omitempty"`
	Strict      *bool           `json:"strict,omitempty"`
}

// ResponsesResponse represents OpenAI Responses API response.
type ResponsesResponse struct {
	ID      string               `json:"id"`
	Object  string               `json:"object"`
	Created int64                `json:"created"`
	Model   string               `json:"model"`
	Status  string               `json:"status"` // "completed"|"incomplete"|"failed"
	Output  []ResponsesOutputItem `json:"output"`
	Usage   *ResponsesUsage      `json:"usage,omitempty"`
	Error   *ResponsesError      `json:"error,omitempty"`
}

// ResponsesOutputItem represents an item in the output array.
type ResponsesOutputItem struct {
	Type string `json:"type"`

	// For message type
	ID       string              `json:"id,omitempty"`
	Role     string              `json:"role,omitempty"`
	Content  []ResponsesContent  `json:"content,omitempty"`
	Status   string              `json:"status,omitempty"`

	// For function_call type
	CallID   string `json:"call_id,omitempty"`
	Name     string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`

	// For reasoning type
	Summary []ResponsesSummary `json:"summary,omitempty"`

	// For image_generation_call type
	Result string `json:"result,omitempty"`
}

// ResponsesSummary represents reasoning summary.
type ResponsesSummary struct {
	Type string `json:"type"` // "summary_text"
	Text string `json:"text"`
}

// ResponsesUsage represents token usage in Responses API.
type ResponsesUsage struct {
	InputTokens           int64                       `json:"input_tokens"`
	OutputTokens          int64                       `json:"output_tokens"`
	TotalTokens           int64                       `json:"total_tokens"`
	InputTokensDetails    *ResponsesInputTokensDetails  `json:"input_tokens_details,omitempty"`
	OutputTokensDetails   *ResponsesOutputTokensDetails `json:"output_tokens_details,omitempty"`
}

// ResponsesInputTokensDetails represents input token breakdown.
type ResponsesInputTokensDetails struct {
	CachedTokens int64 `json:"cached_tokens"`
}

// ResponsesOutputTokensDetails represents output token breakdown.
type ResponsesOutputTokensDetails struct {
	ReasoningTokens int64 `json:"reasoning_tokens"`
}

// ResponsesError represents an error in Responses API.
type ResponsesError struct {
	Type    string `json:"type"`
	Code    string `json:"code,omitempty"`
	Message string `json:"message"`
}

// ResponsesStreamEvent represents a streaming event from Responses API.
type ResponsesStreamEvent struct {
	Type string `json:"type"`

	// For response.created, response.in_progress, response.completed, etc.
	Response  *ResponsesResponse   `json:"response,omitempty"`
	Item      *ResponsesOutputItem `json:"item,omitempty"`
	Content   *ResponsesContent    `json:"content,omitempty"`
	Delta     string               `json:"delta,omitempty"`
	OutputIndex int                `json:"output_index,omitempty"`
	ContentIndex int               `json:"content_index,omitempty"`
	SequenceNumber int             `json:"sequence_number,omitempty"`
}

// Event type constants for Responses API streaming.
const (
	EventTypeResponseCreated                = "response.created"
	EventTypeResponseInProgress             = "response.in_progress"
	EventTypeResponseCompleted              = "response.completed"
	EventTypeResponseFailed                 = "response.failed"
	EventTypeResponseIncomplete             = "response.incomplete"
	EventTypeOutputItemAdded                = "response.output_item.added"
	EventTypeOutputItemDone                 = "response.output_item.done"
	EventTypeContentPartAdded               = "response.content_part.added"
	EventTypeContentPartDone                = "response.content_part.done"
	EventTypeOutputTextDelta                = "response.output_text.delta"
	EventTypeOutputTextDone                 = "response.output_text.done"
	EventTypeFunctionCallArgumentsDelta     = "response.function_call_arguments.delta"
	EventTypeFunctionCallArgumentsDone      = "response.function_call_arguments.done"
	EventTypeReasoningSummaryTextDelta      = "response.reasoning_summary_text.delta"
	EventTypeReasoningSummaryTextDone       = "response.reasoning_summary_text.done"
	EventTypeImageGenerationCallInProgress  = "response.image_generation_call.in_progress"
	EventTypeImageGenerationCallCompleted   = "response.image_generation_call.completed"
)