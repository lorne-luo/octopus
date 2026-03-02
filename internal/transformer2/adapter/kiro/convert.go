package kiro

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bestruirui/octopus/internal/transformer2/canonical"
	"github.com/bestruirui/octopus/internal/utils/log"
	"github.com/google/uuid"
)

// ── Canonical Request → Kiro Request ────────────────────────────────────

// buildKiroRequest converts a canonical Request into a KiroRequest.
func buildKiroRequest(req *canonical.Request) (*KiroRequest, error) {
	modelID := ResolveModel(req.Model)

	// Extract system prompt from messages
	systemPrompt := ""
	var chatMessages []canonical.Message
	for _, msg := range req.Messages {
		if msg.Role == canonical.RoleSystem || msg.Role == canonical.RoleDeveloper {
			for _, block := range msg.Content {
				if block.Type == canonical.ContentText {
					if systemPrompt != "" {
						systemPrompt += "\n"
					}
					systemPrompt += block.Text
				}
			}
		} else {
			chatMessages = append(chatMessages, msg)
		}
	}

	// Process tools — move long descriptions to system prompt
	var kiroTools []map[string]interface{}
	if len(req.Tools) > 0 {
		kiroTools, systemPrompt = processTools(req.Tools, systemPrompt)
	}

	// Process messages: merge, ensure alternating, build history
	chatMessages = ensureFirstMessageIsUser(chatMessages)
	chatMessages = mergeConsecutiveUserMessages(chatMessages)
	chatMessages = ensureAlternatingRoles(chatMessages)

	// Build current message (last user message) + history
	currentUserContent, images, toolResults := extractCurrentUserMessage(chatMessages, modelID)
	history := buildHistory(chatMessages[:maxInt(0, len(chatMessages)-1)], modelID)

	// Build context
	var ctx *MessageContext
	if len(kiroTools) > 0 || len(toolResults) > 0 {
		ctx = &MessageContext{}
		if len(kiroTools) > 0 {
			ctx.Tools = kiroTools
		}
		if len(toolResults) > 0 {
			ctx.ToolResults = toolResults
		}
	}

	// Prepend system prompt to content
	finalContent := currentUserContent
	if systemPrompt != "" {
		finalContent = systemPrompt + "\n\n" + currentUserContent
	}

	return &KiroRequest{
		ConversationState: ConversationState{
			ChatTriggerType: "MANUAL",
			ConversationID:  uuid.New().String(),
			CurrentMessage: CurrentMessage{
				UserInputMessage: UserInputMessage{
					Content:                 finalContent,
					ModelID:                 modelID,
					Origin:                  "AI_EDITOR",
					Images:                  images,
					UserInputMessageContext: ctx,
				},
			},
			History: history,
		},
	}, nil
}

// ── Message Processing Helpers ──────────────────────────────────────────

func ensureFirstMessageIsUser(messages []canonical.Message) []canonical.Message {
	if len(messages) == 0 {
		return messages
	}
	if messages[0].Role == canonical.RoleUser || messages[0].Role == canonical.RoleTool {
		return messages
	}
	log.Debugf("kiro: first message is not 'user', prepending synthetic user message")
	return append([]canonical.Message{{
		Role:    canonical.RoleUser,
		Content: []canonical.ContentBlock{{Type: canonical.ContentText, Text: "(empty)"}},
	}}, messages...)
}

func mergeConsecutiveUserMessages(messages []canonical.Message) []canonical.Message {
	if len(messages) < 2 {
		return messages
	}
	var merged []canonical.Message
	for _, msg := range messages {
		isUserLike := msg.Role == canonical.RoleUser || msg.Role == canonical.RoleTool
		if len(merged) > 0 {
			lastIsUserLike := merged[len(merged)-1].Role == canonical.RoleUser || merged[len(merged)-1].Role == canonical.RoleTool
			if isUserLike && lastIsUserLike {
				last := &merged[len(merged)-1]
				last.Content = append(last.Content, msg.Content...)
				if msg.Reasoning != nil {
					last.Reasoning = msg.Reasoning
				}
				continue
			}
		}
		merged = append(merged, msg)
	}
	return merged
}

func ensureAlternatingRoles(messages []canonical.Message) []canonical.Message {
	if len(messages) < 2 {
		return messages
	}
	var result []canonical.Message
	result = append(result, messages[0])

	for i := 1; i < len(messages); i++ {
		currentIsUserLike := messages[i].Role == canonical.RoleUser || messages[i].Role == canonical.RoleTool
		lastIsUserLike := result[len(result)-1].Role == canonical.RoleUser || result[len(result)-1].Role == canonical.RoleTool

		if currentIsUserLike && lastIsUserLike {
			result = append(result, canonical.Message{
				Role:    canonical.RoleAssistant,
				Content: []canonical.ContentBlock{{Type: canonical.ContentText, Text: "(empty)"}},
			})
		}
		result = append(result, messages[i])
	}
	return result
}

func extractCurrentUserMessage(messages []canonical.Message, modelID string) (string, []map[string]interface{}, []map[string]interface{}) {
	if len(messages) == 0 {
		return "(empty)", nil, nil
	}

	lastMsg := messages[len(messages)-1]
	var textParts []string
	var images []map[string]interface{}
	var toolResults []map[string]interface{}

	// Handle tool result messages
	if lastMsg.Role == canonical.RoleTool {
		content := getTextContent(lastMsg)
		if content == "" {
			content = "(empty)"
		}
		toolCallID := ""
		if lastMsg.ToolCallID != nil {
			toolCallID = *lastMsg.ToolCallID
		}
		toolResults = append(toolResults, map[string]interface{}{
			"content":   []map[string]interface{}{{"text": content}},
			"status":    "success",
			"toolUseId": toolCallID,
		})
		return content, nil, toolResults
	}

	for _, block := range lastMsg.Content {
		switch block.Type {
		case canonical.ContentText:
			textParts = append(textParts, block.Text)
		case canonical.ContentImage:
			if block.Media != nil && block.Media.Base64 != "" {
				format := block.Media.MimeType
				if strings.Contains(format, "/") {
					format = format[strings.Index(format, "/")+1:]
				}
				if format == "" {
					format = "jpeg"
				}
				images = append(images, map[string]interface{}{
					"format": format,
					"source": map[string]interface{}{
						"bytes": block.Media.Base64,
					},
				})
			}
		}
	}

	text := strings.Join(textParts, "\n")
	if text == "" {
		text = "(empty)"
	}
	return text, images, toolResults
}

func buildHistory(messages []canonical.Message, modelID string) []interface{} {
	var history []interface{}

	for _, msg := range messages {
		switch msg.Role {
		case canonical.RoleUser:
			content := getTextContent(msg)
			if content == "" {
				content = "(empty)"
			}
			// Extract images from user message
			var images []map[string]interface{}
			for _, block := range msg.Content {
				if block.Type == canonical.ContentImage && block.Media != nil && block.Media.Base64 != "" {
					format := block.Media.MimeType
					if strings.Contains(format, "/") {
						format = format[strings.Index(format, "/")+1:]
					}
					if format == "" {
						format = "jpeg"
					}
					images = append(images, map[string]interface{}{
						"format": format,
						"source": map[string]interface{}{"bytes": block.Media.Base64},
					})
				}
			}
			userInput := map[string]interface{}{
				"content": content,
				"modelId": modelID,
				"origin":  "AI_EDITOR",
			}
			if len(images) > 0 {
				userInput["images"] = images
			}
			history = append(history, map[string]interface{}{
				"userInputMessage": userInput,
			})

		case canonical.RoleAssistant:
			content := getTextContent(msg)
			if content == "" {
				content = "(empty)"
			}
			assistantMsg := map[string]interface{}{
				"body": content,
			}
			// Include tool calls if present
			if len(msg.ToolCalls) > 0 {
				var kiroToolUse []map[string]interface{}
				for _, tc := range msg.ToolCalls {
					kiroToolUse = append(kiroToolUse, map[string]interface{}{
						"toolUseId": tc.ID,
						"name":      tc.Name,
						"input":     json.RawMessage(tc.Arguments),
					})
				}
				assistantMsg["toolUse"] = kiroToolUse
			}
			history = append(history, map[string]interface{}{
				"assistantResponseMessage": assistantMsg,
			})

		case canonical.RoleTool:
			content := getTextContent(msg)
			if content == "" {
				content = "(empty)"
			}
			toolCallID := ""
			if msg.ToolCallID != nil {
				toolCallID = *msg.ToolCallID
			}
			userInput := map[string]interface{}{
				"content": content,
				"modelId": modelID,
				"origin":  "AI_EDITOR",
			}
			userInput["userInputMessageContext"] = map[string]interface{}{
				"toolResults": []map[string]interface{}{{
					"content":   []map[string]interface{}{{"text": content}},
					"status":    "success",
					"toolUseId": toolCallID,
				}},
			}
			history = append(history, map[string]interface{}{
				"userInputMessage": userInput,
			})
		}
	}
	return history
}

func getTextContent(msg canonical.Message) string {
	var parts []string
	for _, block := range msg.Content {
		if block.Type == canonical.ContentText && block.Text != "" {
			parts = append(parts, block.Text)
		}
	}
	return strings.Join(parts, "\n")
}

func processTools(tools []canonical.Tool, systemPrompt string) ([]map[string]interface{}, string) {
	const maxDescLen = 200

	var kiroTools []map[string]interface{}
	var docParts []string

	for _, tool := range tools {
		desc := tool.Description
		if desc == "" {
			desc = "Tool: " + tool.Name
		}

		// Move long descriptions to system prompt
		if len(desc) > maxDescLen {
			docParts = append(docParts, fmt.Sprintf("## Tool: %s\n\n%s", tool.Name, desc))
			desc = fmt.Sprintf("[Full documentation in system prompt under '## Tool: %s']", tool.Name)
		}

		var params interface{}
		if len(tool.Parameters) > 0 {
			json.Unmarshal(tool.Parameters, &params)
			if pm, ok := params.(map[string]interface{}); ok {
				params = SanitizeJSONSchema(pm)
			}
		} else {
			params = map[string]interface{}{}
		}

		kiroTools = append(kiroTools, map[string]interface{}{
			"toolSpecification": map[string]interface{}{
				"name":        tool.Name,
				"description": desc,
				"inputSchema": map[string]interface{}{
					"json": params,
				},
			},
		})
	}

	if len(docParts) > 0 {
		systemPrompt += "\n\n---\n# Tool Documentation\n" + strings.Join(docParts, "\n\n---\n\n")
	}

	return kiroTools, systemPrompt
}

// ── Kiro Response → Canonical ───────────────────────────────────────────

func kiroToolCallsToCanonical(calls []kiroToolCall) []canonical.ToolCall {
	if len(calls) == 0 {
		return nil
	}
	result := make([]canonical.ToolCall, len(calls))
	for i, tc := range calls {
		result[i] = canonical.ToolCall{
			ID:        tc.ID,
			Type:      "function",
			Name:      tc.Name,
			Arguments: tc.Arguments,
		}
	}
	return result
}

// kiroToolCall holds accumulated tool call data from stream events.
type kiroToolCall struct {
	ID        string
	Name      string
	Arguments string
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
