package anthropic

import (
	"encoding/json"

	"github.com/bestruirui/octopus/internal/transformer2/canonical"
)

// convertAnthropicMessageToCanonical converts an Anthropic MessageParam to canonical Message.
func convertAnthropicMessageToCanonical(msg MessageParam) canonical.Message {
	cmsg := canonical.Message{
		Role: canonical.Role(msg.Role),
	}

	// Handle content (string or array)
	if msg.Content.Text != "" {
		cmsg.Content = []canonical.ContentBlock{
			{Type: canonical.ContentText, Text: msg.Content.Text},
		}
	} else if len(msg.Content.Blocks) > 0 {
		cmsg.Content = convertAnthropicContentToCanonical(msg.Content.Blocks)
	}

	return cmsg
}

// convertCanonicalToAnthropicMessage converts a canonical Message to Anthropic MessageParam.
func convertCanonicalToAnthropicMessage(msg canonical.Message) MessageParam {
	aparam := MessageParam{
		Role: string(msg.Role),
	}

	// Handle content
	if len(msg.Content) > 0 {
		aparam.Content = convertCanonicalToAnthropicContent(msg.Content)
	}

	return aparam
}

// convertAnthropicContentToCanonical converts Anthropic content blocks to canonical ContentBlocks.
func convertAnthropicContentToCanonical(blocks []ContentBlock) []canonical.ContentBlock {
	result := make([]canonical.ContentBlock, 0, len(blocks))

	for _, block := range blocks {
		cb := canonical.ContentBlock{}

		switch block.Type {
		case ContentTypeText:
			cb.Type = canonical.ContentText
			cb.Text = derefStr(block.Text)
			if block.CacheControl != nil {
				cb.CacheControl = &canonical.CacheControl{Type: block.CacheControl.Type}
			}

		case ContentTypeImage:
			cb.Type = canonical.ContentImage
			if block.Source != nil {
				cb.Media = &canonical.MediaContent{
					MimeType: block.Source.MediaType,
					Base64:   block.Source.Data,
					URL:      block.Source.URL,
				}
			}

		case ContentTypeThinking:
			cb.Type = canonical.ContentThinking
			cb.Thinking = derefStr(block.Thinking)
			cb.Signature = derefStr(block.Signature)

		case ContentTypeToolUse:
			// Tool use blocks are handled via ToolCalls in the message
			// This is for assistant messages containing tool calls
			// Will be processed separately

		case ContentTypeToolResult:
			// Tool result is handled via ToolCallID in canonical message
			// Will be processed separately
		}

		result = append(result, cb)
	}

	return result
}

// convertCanonicalToAnthropicContent converts canonical ContentBlocks to Anthropic content blocks.
func convertCanonicalToAnthropicContent(blocks []canonical.ContentBlock) MessageContent {
	content := MessageContent{}

	if len(blocks) == 0 {
		return content
	}

	// Check if single text block -> use string format
	if len(blocks) == 1 && blocks[0].Type == canonical.ContentText && blocks[0].CacheControl == nil {
		content.Text = blocks[0].Text
		return content
	}

	// Otherwise use array format
	content.Blocks = make([]ContentBlock, 0, len(blocks))

	for _, cb := range blocks {
		block := ContentBlock{}

		switch cb.Type {
		case canonical.ContentText:
			block.Type = ContentTypeText
			block.Text = strPtr(cb.Text)
			if cb.CacheControl != nil {
				block.CacheControl = &CacheControl{Type: cb.CacheControl.Type}
			}

		case canonical.ContentImage:
			block.Type = ContentTypeImage
			if cb.Media != nil {
				block.Source = &ImageSource{
					MediaType: cb.Media.MimeType,
					Data:      cb.Media.Base64,
					URL:       cb.Media.URL,
				}
				if cb.Media.Base64 != "" {
					block.Source.Type = "base64"
				} else if cb.Media.URL != "" {
					block.Source.Type = "url"
				}
			}

		case canonical.ContentThinking:
			block.Type = ContentTypeThinking
			block.Thinking = strPtr(cb.Thinking)
			block.Signature = strPtr(cb.Signature)

		case canonical.ContentDocument:
			// Document content passthrough
			block.Type = "document"
			if cb.Media != nil {
				block.Source = &ImageSource{
					Type:      "base64",
					MediaType: cb.Media.MimeType,
					Data:      cb.Media.Base64,
				}
			}
		}

		content.Blocks = append(content.Blocks, block)
	}

	return content
}

// convertAnthropicToolToCanonical converts an Anthropic Tool to canonical Tool.
func convertAnthropicToolToCanonical(tool Tool) canonical.Tool {
	ctool := canonical.Tool{
		Name:        tool.Name,
		Description: tool.Description,
		Parameters:  tool.InputSchema,
	}

	// Anthropic tools don't have explicit type, default to "function"
	ctool.Type = "function"

	if tool.CacheControl != nil {
		ctool.CacheControl = &canonical.CacheControl{Type: tool.CacheControl.Type}
	}

	return ctool
}

// convertCanonicalToAnthropicTool converts a canonical Tool to Anthropic Tool.
func convertCanonicalToAnthropicTool(tool canonical.Tool) Tool {
	atool := Tool{
		Name:        tool.Name,
		Description: tool.Description,
		InputSchema: tool.Parameters,
	}

	if tool.CacheControl != nil {
		atool.CacheControl = &CacheControl{Type: tool.CacheControl.Type}
	}

	return atool
}

// convertAnthropicUsageToCanonical converts Anthropic Usage to canonical Usage.
func convertAnthropicUsageToCanonical(usage Usage) *canonical.Usage {
	// CRITICAL CACHING SEMANTICS: When caching is enabled, input_tokens only
	// represents tokens AFTER the last cache breakpoint, NOT total input tokens.
	// Correct formula: PromptTokens = cache_read + cache_creation + input_tokens
	totalPromptTokens := usage.InputTokens + usage.CacheReadInputTokens + usage.CacheCreationInputTokens
	cusage := &canonical.Usage{
		PromptTokens:               totalPromptTokens,
		CompletionTokens:           usage.OutputTokens,
		TotalTokens:                totalPromptTokens + usage.OutputTokens,
		CacheCreationInputTokens:   usage.CacheCreationInputTokens,
		CacheReadInputTokens:       usage.CacheReadInputTokens,
		InputTokensAfterBreakpoint: usage.InputTokens,
		IsAnthropicUsage:           true,
	}

	return cusage
}

// mapAnthropicStopReasonToCanonical maps Anthropic stop_reason to canonical finish_reason.
// Returns nil if stop_reason is empty.
func mapAnthropicStopReasonToCanonical(reason string) *string {
	if reason == "" {
		return nil
	}

	var result string
	switch reason {
	case StopReasonEndTurn:
		result = "stop"
	case StopReasonMaxTokens:
		result = "length"
	case StopReasonToolUse:
		result = "tool_calls"
	case StopReasonStopSequence:
		result = "stop"
	case "pause_turn":
		result = "stop"
	case "refusal":
		result = "content_filter"
	default:
		result = reason
	}

	return &result
}

// mapCanonicalFinishReasonToAnthropic maps canonical finish_reason to Anthropic stop_reason.
func mapCanonicalFinishReasonToAnthropic(reason string) string {
	switch reason {
	case "stop":
		return StopReasonEndTurn
	case "length":
		return StopReasonMaxTokens
	case "tool_calls":
		return StopReasonToolUse
	case "content_filter":
		return "refusal"
	default:
		return reason
	}
}

// convertAnthropicToolChoiceToCanonical converts Anthropic ToolChoice to canonical ToolChoice.
func convertAnthropicToolChoiceToCanonical(tc *ToolChoice) *canonical.ToolChoice {
	if tc == nil {
		return nil
	}

	ctc := &canonical.ToolChoice{}

	switch tc.Type {
	case "auto":
		ctc.Mode = "auto"
	case "none":
		ctc.Mode = "none"
	case "any":
		ctc.Mode = "required"
	case "tool":
		ctc.Mode = "tool"
		ctc.Function = &tc.Name
	}

	if tc.DisableParallelToolUse {
		ctc.DisableParallelToolUse = &tc.DisableParallelToolUse
	}

	return ctc
}

// convertCanonicalToAnthropicToolChoice converts canonical ToolChoice to Anthropic ToolChoice.
func convertCanonicalToAnthropicToolChoice(tc *canonical.ToolChoice) *ToolChoice {
	if tc == nil {
		return nil
	}

	atc := &ToolChoice{}

	switch tc.Mode {
	case "auto":
		atc.Type = "auto"
	case "none":
		atc.Type = "none"
	case "required":
		atc.Type = "any"
	case "tool":
		atc.Type = "tool"
		if tc.Function != nil {
			atc.Name = *tc.Function
		}
	}

	if tc.DisableParallelToolUse != nil {
		atc.DisableParallelToolUse = *tc.DisableParallelToolUse
	}

	return atc
}

// convertAnthropicThinkingToCanonical converts Anthropic ThinkingConfig to canonical ReasoningConfig.
// Returns nil if thinking is disabled or not specified.
func convertAnthropicThinkingToCanonical(thinking *ThinkingConfig) *canonical.ReasoningConfig {
	if thinking == nil {
		return nil
	}

	switch thinking.Type {
	case "adaptive":
		// Adaptive mode: Enabled = nil
		return &canonical.ReasoningConfig{}
	case "enabled":
		// Legacy mode: Enabled = true
		enabled := true
		rc := &canonical.ReasoningConfig{
			Enabled: &enabled,
		}
		if thinking.BudgetTokens > 0 {
			rc.BudgetTokens = &thinking.BudgetTokens
			// Map budget_tokens to effort level (per cross_cutting.md)
			// minimal: ≤1024, low: ≤5000, medium: ≤15000, high: >15000
			var effort string
			if thinking.BudgetTokens <= 1024 {
				effort = "minimal"
			} else if thinking.BudgetTokens <= 5000 {
				effort = "low"
			} else if thinking.BudgetTokens <= 15000 {
				effort = "medium"
			} else {
				effort = "high"
			}
			rc.Effort = &effort
		}
		return rc
	case "disabled":
		// Disabled: Enabled = false
		enabled := false
		return &canonical.ReasoningConfig{
			Enabled: &enabled,
		}
	default:
		return nil
	}
}

// convertCanonicalToAnthropicThinking converts canonical ReasoningConfig to Anthropic ThinkingConfig.
// Returns nil if reasoning is disabled or not specified.
func convertCanonicalToAnthropicThinking(rc *canonical.ReasoningConfig) *ThinkingConfig {
	if rc == nil {
		return nil
	}

	// Check for explicit disabled
	if rc.Enabled != nil && !*rc.Enabled {
		return &ThinkingConfig{Type: "disabled"}
	}

	// If Enabled is nil (adaptive mode), return adaptive
	if rc.Enabled == nil {
		return &ThinkingConfig{Type: "adaptive"}
	}

	// Build thinking config (enabled mode)
	tc := &ThinkingConfig{Type: "enabled"}

	// Handle budget_tokens
	if rc.BudgetTokens != nil && *rc.BudgetTokens > 0 {
		tc.BudgetTokens = *rc.BudgetTokens
	} else if rc.Effort != nil {
		// Map reasoning_effort to budget_tokens (per cross_cutting.md)
		// minimal→1024, low→5000, medium→15000, high→30000, xhigh→60000
		switch *rc.Effort {
		case "minimal":
			tc.BudgetTokens = 1024
		case "low":
			tc.BudgetTokens = 5000
		case "medium":
			tc.BudgetTokens = 15000
		case "high":
			tc.BudgetTokens = 30000
		case "xhigh", "max":
			tc.BudgetTokens = 60000
		default:
			tc.BudgetTokens = 15000
		}
	}

	return tc
}

// extractToolResultContent extracts the content from a tool result message.
func extractToolResultContent(msg canonical.Message) (string, []ContentBlock) {
	if len(msg.Content) == 1 && msg.Content[0].Type == canonical.ContentText {
		return msg.Content[0].Text, nil
	}

	// Convert content blocks
	blocks := make([]ContentBlock, 0, len(msg.Content))
	for _, cb := range msg.Content {
		block := ContentBlock{Type: string(cb.Type)}
		switch cb.Type {
		case canonical.ContentText:
			block.Text = strPtr(cb.Text)
		case canonical.ContentImage:
			if cb.Media != nil {
				block.Source = &ImageSource{
					Type:      "base64",
					MediaType: cb.Media.MimeType,
					Data:      cb.Media.Base64,
				}
			}
		}
		blocks = append(blocks, block)
	}

	return "", blocks
}

// marshalJSON marshals a value to JSON, returning empty bytes on error.
func marshalJSON(v interface{}) json.RawMessage {
	data, _ := json.Marshal(v)
	return data
}

// unmarshalJSON safely unmarshals JSON bytes.
func unmarshalJSON(data json.RawMessage, v interface{}) error {
	if len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, v)
}

// mergeExtraBody merges opaque ExtraBody JSON into the serialized request body.
// ExtraBody keys take precedence over existing keys (user intent).
func mergeExtraBody(body []byte, extraBody json.RawMessage) ([]byte, error) {
	if len(extraBody) == 0 {
		return body, nil
	}

	var base map[string]interface{}
	if err := json.Unmarshal(body, &base); err != nil {
		return body, nil
	}

	var extra map[string]interface{}
	if err := json.Unmarshal(extraBody, &extra); err != nil {
		return body, nil
	}

	for k, v := range extra {
		base[k] = v
	}

	return json.Marshal(base)
}
