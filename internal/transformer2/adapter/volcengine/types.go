package volcengine

import "encoding/json"

// ThinkingType represents the thinking mode for Volcengine API.
type ThinkingType string

const (
	ThinkingTypeAuto     ThinkingType = "auto"
	ThinkingTypeDisabled ThinkingType = "disabled"
	ThinkingTypeEnabled  ThinkingType = "enabled"
)

// Thinking represents Volcengine's thinking configuration.
type Thinking struct {
	Type ThinkingType `json:"type"`
}

// ResponsesInput represents input which can be string or array of items.
type ResponsesInput struct {
	Text  string            `json:"-"`
	Items []ResponsesInputItem `json:"-"`
}

// MarshalJSON handles ResponsesInput serialization.
func (ri ResponsesInput) MarshalJSON() ([]byte, error) {
	if ri.Text != "" && len(ri.Items) == 0 {
		return json.Marshal(ri.Text)
	}
	return json.Marshal(ri.Items)
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

	// Partial flag for assistant messages in continuation
	Partial bool `json:"partial,omitempty"`
}

// ResponsesContent represents content in an input/output item.
type ResponsesContent struct {
	Type string `json:"type"`

	// For input_text/output_text
	Text string `json:"text,omitempty"`

	// For input_image
	ImageURL string `json:"image_url,omitempty"`
	Detail   string `json:"detail,omitempty"`
	FileID   string `json:"file_id,omitempty"`
	FileData string `json:"file_data,omitempty"`
	MimeType string `json:"mime_type,omitempty"`
}

// ResponsesRequest represents Volcengine Responses API request.
type ResponsesRequest struct {
	Model           string                 `json:"model"`
	Input           ResponsesInput         `json:"input"`
	Instructions    *string                `json:"instructions,omitempty"`
	Tools           []ResponsesTool        `json:"tools,omitempty"`
	ToolChoice      *ResponsesToolChoice   `json:"tool_choice,omitempty"`
	Reasoning       *ResponsesReasoning    `json:"reasoning,omitempty"`
	Thinking        *Thinking              `json:"thinking,omitzero"`
	Text            *ResponsesText         `json:"text,omitempty"`
	Include         []string               `json:"include,omitempty"`
	MaxOutputTokens *int64                 `json:"max_output_tokens,omitempty"`
	Temperature     *float64               `json:"temperature,omitempty"`
	TopP            *float64               `json:"top_p,omitempty"`
	Store           *bool                  `json:"store,omitempty"`
	ServiceTier     *string                `json:"service_tier,omitempty"`
	User            *string                `json:"user,omitempty"`
	Stream          *bool                  `json:"stream,omitempty"`
}

// ResponsesTool represents a tool definition.
type ResponsesTool struct {
	Type string `json:"type"`

	// For function type
	Name        string          `json:"name,omitempty"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
	Strict      *bool           `json:"strict,omitempty"`
}

// ResponsesToolChoice represents tool choice.
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
	return json.Marshal(struct {
		Type string `json:"type"`
		Name string `json:"name"`
	}{
		Type: tc.Type,
		Name: tc.Name,
	})
}

// ResponsesReasoning represents reasoning configuration.
type ResponsesReasoning struct {
	Effort string `json:"effort,omitempty"`
}

// ResponsesText represents text format configuration.
type ResponsesText struct {
	Format *ResponsesTextFormat `json:"format,omitempty"`
}

// ResponsesTextFormat represents text format specification.
type ResponsesTextFormat struct {
	Type string `json:"type"`

	// For json_schema
	Name        string          `json:"name,omitempty"`
	Description string          `json:"description,omitempty"`
	Schema      json.RawMessage `json:"schema,omitempty"`
	Strict      *bool           `json:"strict,omitempty"`
}