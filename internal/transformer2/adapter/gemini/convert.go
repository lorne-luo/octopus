package gemini

import (
	"encoding/json"
	"strings"

	"github.com/bestruirui/octopus/internal/transformer2/canonical"
)

// convertGeminiContentToCanonical converts a Gemini Content to canonical Message.
func convertGeminiContentToCanonical(c Content) canonical.Message {
	msg := canonical.Message{
		Role: convertGeminiRoleToCanonical(c.Role),
	}

	if len(c.Parts) > 0 {
		var contentBlocks []canonical.ContentBlock
		var toolCalls []canonical.ToolCall
		var reasoning *string
		var reasoningSignature *string
		var thoughtSummary *string

		for _, part := range c.Parts {
			// Handle thought/reasoning parts
			if part.Thought {
				if part.Text != "" {
					reasoning = &part.Text
				}
				if part.ThoughtSignature != "" {
					reasoningSignature = &part.ThoughtSignature
				}
				if part.ThoughtSummary != "" {
					thoughtSummary = &part.ThoughtSummary
				}
				continue
			}

			// Handle thoughtSignature without thought flag (standalone signature)
			if part.ThoughtSignature != "" {
				reasoningSignature = &part.ThoughtSignature
			}

			// Handle function call
			if part.FunctionCall != nil {
				tc := canonical.ToolCall{
					ID:        "", // Gemini doesn't have tool call IDs
					Type:      "function",
					Name:      part.FunctionCall.Name,
					Arguments: string(part.FunctionCall.Args),
				}
				// Preserve thought signature on first parallel function call
				if part.ThoughtSignature != "" && len(toolCalls) == 0 {
					tc.Signature = &part.ThoughtSignature
				}
				toolCalls = append(toolCalls, tc)
				continue
			}

			// Handle function response (tool result)
			if part.FunctionResponse != nil {
				msg.ToolCallID = &part.FunctionResponse.Name
				msg.Role = canonical.RoleTool
				// Marshal response to content
				if respBytes, err := json.Marshal(part.FunctionResponse.Response); err == nil {
					msg.Content = append(msg.Content, canonical.ContentBlock{
						Type: canonical.ContentText,
						Text: string(respBytes),
					})
				}
				continue
			}

			// Handle text and media content
			blocks := convertGeminiPartToCanonical(part)
			contentBlocks = append(contentBlocks, blocks...)
		}

		msg.Content = contentBlocks
		msg.ToolCalls = toolCalls
		msg.Reasoning = reasoning
		msg.ReasoningSignature = reasoningSignature
		msg.ThoughtSummary = thoughtSummary
	}

	return msg
}

// convertCanonicalToGeminiContent converts a canonical Message to Gemini Content.
func convertCanonicalToGeminiContent(msg canonical.Message) Content {
	c := Content{
		Role: convertCanonicalRoleToGemini(msg.Role),
	}

	// Handle tool result messages
	if msg.Role == canonical.RoleTool && msg.ToolCallID != nil {
		// Create function response part
		var response interface{}
		if len(msg.Content) > 0 && msg.Content[0].Type == canonical.ContentText {
			// Try to parse as JSON, otherwise use as string
			var parsed interface{}
			if err := json.Unmarshal([]byte(msg.Content[0].Text), &parsed); err == nil {
				response = parsed
			} else {
				response = map[string]interface{}{"result": msg.Content[0].Text}
			}
		}
		c.Parts = append(c.Parts, Part{
			FunctionResponse: &FunctionResponse{
				Name:     *msg.ToolCallID,
				Response: response,
			},
		})
		return c
	}

	// Handle assistant messages with tool calls
	if msg.Role == canonical.RoleAssistant && len(msg.ToolCalls) > 0 {
		for i, tc := range msg.ToolCalls {
			part := Part{
				FunctionCall: &FunctionCall{
					Name: tc.Name,
					Args: json.RawMessage(tc.Arguments),
				},
			}
			// Set thoughtSignature on first parallel function call
			if i == 0 && tc.Signature != nil {
				part.ThoughtSignature = *tc.Signature
			}
			c.Parts = append(c.Parts, part)
		}
	}

	// Convert content blocks
	if len(msg.Content) > 0 {
		parts := convertCanonicalToGeminiPart(msg.Content)
		c.Parts = append(c.Parts, parts...)
	}

	// Handle reasoning/thinking
	if msg.Reasoning != nil {
		c.Parts = append([]Part{{
			Thought: true,
			Text:    *msg.Reasoning,
		}}, c.Parts...)
	}

	// Handle reasoning signature
	if msg.ReasoningSignature != nil {
		// Add signature to the thought part if it exists
		for i, part := range c.Parts {
			if part.Thought {
				c.Parts[i].ThoughtSignature = *msg.ReasoningSignature
				break
			}
		}
	}

	return c
}

// convertGeminiPartToCanonical converts a Gemini Part to canonical ContentBlocks.
func convertGeminiPartToCanonical(p Part) []canonical.ContentBlock {
	var blocks []canonical.ContentBlock

	// Text content
	if p.Text != "" {
		blocks = append(blocks, canonical.ContentBlock{
			Type: canonical.ContentText,
			Text: p.Text,
		})
	}

	// Inline data (base64)
	if p.InlineData != nil {
		blocks = append(blocks, canonical.ContentBlock{
			Type: canonical.ContentImage,
			Media: &canonical.MediaContent{
				MimeType: p.InlineData.MimeType,
				Base64:   p.InlineData.Data,
			},
		})
	}

	// File data (URI)
	if p.FileData != nil {
		mimeType := p.FileData.MimeType
		var contentType canonical.ContentType
		if strings.HasPrefix(mimeType, "image/") {
			contentType = canonical.ContentImage
		} else if strings.HasPrefix(mimeType, "audio/") {
			contentType = canonical.ContentAudio
		} else {
			contentType = canonical.ContentFile
		}
		blocks = append(blocks, canonical.ContentBlock{
			Type: contentType,
			Media: &canonical.MediaContent{
				MimeType: mimeType,
				URL:      p.FileData.FileURI,
			},
		})
	}

	return blocks
}

// convertCanonicalToGeminiPart converts canonical ContentBlocks to Gemini Parts.
func convertCanonicalToGeminiPart(blocks []canonical.ContentBlock) []Part {
	var parts []Part

	for _, block := range blocks {
		switch block.Type {
		case canonical.ContentText:
			parts = append(parts, Part{
				Text: block.Text,
			})

		case canonical.ContentImage:
			if block.Media != nil {
				if block.Media.Base64 != "" {
					parts = append(parts, Part{
						InlineData: &InlineData{
							MimeType: block.Media.MimeType,
							Data:     block.Media.Base64,
						},
					})
				} else if block.Media.URL != "" {
					parts = append(parts, Part{
						FileData: &FileData{
							MimeType: block.Media.MimeType,
							FileURI:  block.Media.URL,
						},
					})
				}
			}

		case canonical.ContentAudio:
			if block.Media != nil {
				if block.Media.Base64 != "" {
					parts = append(parts, Part{
						InlineData: &InlineData{
							MimeType: block.Media.MimeType,
							Data:     block.Media.Base64,
						},
					})
				} else if block.Media.URL != "" {
					parts = append(parts, Part{
						FileData: &FileData{
							MimeType: block.Media.MimeType,
							FileURI:  block.Media.URL,
						},
					})
				}
			}

		case canonical.ContentFile:
			if block.Media != nil {
				if block.Media.Base64 != "" {
					parts = append(parts, Part{
						InlineData: &InlineData{
							MimeType: block.Media.MimeType,
							Data:     block.Media.Base64,
						},
					})
				} else if block.Media.URL != "" {
					parts = append(parts, Part{
						FileData: &FileData{
							MimeType: block.Media.MimeType,
							FileURI:  block.Media.URL,
						},
					})
				}
			}

		case canonical.ContentThinking:
			part := Part{
				Thought: true,
				Text:    block.Thinking,
			}
			if block.Signature != "" {
				part.ThoughtSignature = block.Signature
			}
			parts = append(parts, part)
		}
	}

	return parts
}

// convertGeminiToolToCanonical converts a Gemini FunctionDeclaration to canonical Tool.
func convertGeminiToolToCanonical(fd FunctionDeclaration) canonical.Tool {
	return canonical.Tool{
		Type:        "function",
		Name:        fd.Name,
		Description: fd.Description,
		Parameters:  fd.Parameters,
	}
}

// convertCanonicalToGeminiTool converts a canonical Tool to Gemini FunctionDeclaration.
func convertCanonicalToGeminiTool(tool canonical.Tool) FunctionDeclaration {
	return FunctionDeclaration{
		Name:        tool.Name,
		Description: tool.Description,
		Parameters:  tool.Parameters,
	}
}

// convertGeminiRoleToCanonical maps Gemini role to canonical role.
func convertGeminiRoleToCanonical(role string) canonical.Role {
	switch strings.ToLower(role) {
	case "model":
		return canonical.RoleAssistant
	case "user":
		return canonical.RoleUser
	default:
		return canonical.Role(role)
	}
}

// convertCanonicalRoleToGemini maps canonical role to Gemini role.
func convertCanonicalRoleToGemini(role canonical.Role) string {
	switch role {
	case canonical.RoleAssistant:
		return "model"
	case canonical.RoleUser:
		return "user"
	case canonical.RoleTool:
		return "user" // Tool results are in user messages
	default:
		return string(role)
	}
}

// mapGeminiFinishReasonToCanonical maps Gemini finish reason to canonical finish reason.
func mapGeminiFinishReasonToCanonical(reason string) *string {
	if reason == "" {
		return nil
	}

	var result string
	switch reason {
	case "STOP":
		result = "stop"
	case "MAX_TOKENS":
		result = "length"
	case "SAFETY", "BLOCKLIST", "PROHIBITED_CONTENT", "SPII", "RECITATION":
		result = "content_filter"
	case "OTHER":
		result = "other"
	default:
		result = strings.ToLower(reason)
	}

	return &result
}

// mapCanonicalFinishReasonToGemini maps canonical finish reason to Gemini finish reason.
func mapCanonicalFinishReasonToGemini(reason string) string {
	switch reason {
	case "stop":
		return "STOP"
	case "length":
		return "MAX_TOKENS"
	case "content_filter":
		return "SAFETY"
	case "tool_calls":
		return "STOP"
	default:
		return strings.ToUpper(reason)
	}
}

// mapCanonicalToolChoiceToGemini maps canonical ToolChoice to Gemini FunctionCallingConfig.
func mapCanonicalToolChoiceToGemini(tc *canonical.ToolChoice) *FunctionCallingConfig {
	if tc == nil {
		return nil
	}

	config := &FunctionCallingConfig{}

	switch tc.Mode {
	case "auto":
		config.Mode = "AUTO"
	case "none":
		config.Mode = "NONE"
	case "required":
		config.Mode = "ANY"
	case "tool":
		config.Mode = "ANY"
		if tc.Function != nil {
			config.AllowedFunctionNames = []string{*tc.Function}
		}
	default:
		config.Mode = "AUTO"
	}

	return config
}

// mapGeminiToolChoiceToCanonical maps Gemini FunctionCallingConfig to canonical ToolChoice.
func mapGeminiToolChoiceToCanonical(config *FunctionCallingConfig) *canonical.ToolChoice {
	if config == nil {
		return nil
	}

	tc := &canonical.ToolChoice{}

	switch config.Mode {
	case "AUTO":
		tc.Mode = "auto"
	case "NONE":
		tc.Mode = "none"
	case "ANY":
		if len(config.AllowedFunctionNames) == 1 {
			tc.Mode = "tool"
			tc.Function = &config.AllowedFunctionNames[0]
		} else {
			tc.Mode = "required"
		}
	default:
		tc.Mode = "auto"
	}

	return tc
}

// convertGeminiUsageToCanonical converts Gemini UsageMetadata to canonical Usage.
func convertGeminiUsageToCanonical(usage *UsageMetadata) *canonical.Usage {
	if usage == nil {
		return nil
	}

	cusage := &canonical.Usage{
		PromptTokens:     int64(usage.PromptTokenCount),
		CompletionTokens: int64(usage.CandidatesTokenCount),
		TotalTokens:      int64(usage.TotalTokenCount),
	}

	if usage.ThoughtsTokenCount != nil {
		cusage.CompletionTokensDetails = &canonical.CompletionTokensDetails{
			ReasoningTokens: int64(*usage.ThoughtsTokenCount),
		}
	}

	if usage.CachedContentTokenCount != nil {
		cusage.PromptTokensDetails = &canonical.PromptTokensDetails{
			CachedTokens: int64(*usage.CachedContentTokenCount),
		}
	}

	return cusage
}

// mapCanonicalEffortToGeminiThinkingLevel maps canonical reasoning effort to Gemini thinking level.
func mapCanonicalEffortToGeminiThinkingLevel(effort *string) string {
	if effort == nil {
		return "" // Use default
	}

	switch *effort {
	case "none":
		return "minimal"
	case "low", "medium", "high":
		return *effort
	default:
		return strings.ToLower(*effort)
	}
}

// mapGeminiThinkingLevelToCanonicalEffort maps Gemini thinking level to canonical reasoning effort.
func mapGeminiThinkingLevelToCanonicalEffort(level string) *string {
	if level == "" {
		return nil
	}

	var effort string
	switch level {
	case "minimal":
		effort = "none"
	case "low", "medium", "high":
		effort = level
	default:
		effort = strings.ToLower(level)
	}

	return &effort
}

// buildSystemInstruction extracts system and developer messages into a single Gemini system_instruction.
func buildSystemInstruction(messages []canonical.Message) *Content {
	var textParts []string

	for _, msg := range messages {
		if msg.Role == canonical.RoleSystem || msg.Role == canonical.RoleDeveloper {
			for _, cb := range msg.Content {
				if cb.Type == canonical.ContentText {
					textParts = append(textParts, cb.Text)
				}
			}
		}
	}

	if len(textParts) == 0 {
		return nil
	}

	return &Content{
		Parts: []Part{{
			Text: strings.Join(textParts, "\n"),
		}},
	}
}

// filterNonSystemMessages filters out system and developer messages for the contents array.
func filterNonSystemMessages(messages []canonical.Message) []canonical.Message {
	var result []canonical.Message
	for _, msg := range messages {
		if msg.Role != canonical.RoleSystem && msg.Role != canonical.RoleDeveloper {
			result = append(result, msg)
		}
	}
	return result
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
