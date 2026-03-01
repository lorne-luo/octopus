package kiro

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bestruirui/octopus/internal/transformer/model"
	"github.com/bestruirui/octopus/internal/utils/log"
)

// StripAllToolContent strips all tool-related content from messages
// This is used when no tools are provided to simplify the message content
func StripAllToolContent(messages []model.Message) ([]model.Message, bool) {
	var result []model.Message
	var hadToolContent bool

	for _, msg := range messages {
		// Check for tool-related content
		if len(msg.ToolCalls) > 0 || msg.ToolCallID != nil || msg.Role == "tool" {
			hadToolContent = true

			content := msg.Content.GetFullContent()
			parts := []string{content}

			if len(msg.ToolCalls) > 0 {
				parts = append(parts, ToolCallsToText(msg.ToolCalls))
			}
			if msg.ToolCallID != nil || msg.Role == "tool" {
				// Tool result message - convert to text representation
				toolUseID := ""
				if msg.ToolCallID != nil {
					toolUseID = *msg.ToolCallID
				}
				toolResultText := ToolResultToText(&toolUseID, content)
				parts = append(parts, toolResultText)
			}

			// Create new message with merged content
			// Convert "tool" role to "user" since Kiro doesn't have tool role
			newRole := msg.Role
			if msg.Role == "tool" {
				newRole = "user"
			}
			newMsg := model.Message{
				Role: newRole,
			}
			mergedContent := strings.Join(parts, "\n\n")
			newMsg.Content = model.MessageContent{Content: &mergedContent}

			// Preserve images if any
			if len(msg.Content.MultipleContent) > 0 {
				var images []model.MessageContentPart
				for _, part := range msg.Content.MultipleContent {
					if part.Type == "image_url" && part.ImageURL != nil {
						images = append(images, part)
					}
				}
				if len(images) > 0 {
					newMsg.Content.MultipleContent = images
				}
			}

			result = append(result, newMsg)
		} else {
			result = append(result, msg)
		}
	}

	return result, hadToolContent
}

// ToolCallsToText converts tool calls to text representation
func ToolCallsToText(calls []model.ToolCall) string {
	var parts []string
	for _, tc := range calls {
		if tc.ID != "" {
			parts = append(parts, fmt.Sprintf("[Tool: %s (%s)]\n%s", tc.Function.Name, tc.ID, tc.Function.Arguments))
		} else {
			parts = append(parts, fmt.Sprintf("[Tool: %s]\n%s", tc.Function.Name, tc.Function.Arguments))
		}
	}
	return strings.Join(parts, "\n\n")
}

// ToolResultToText converts a tool result to text representation
func ToolResultToText(toolUseID *string, content string) string {
	if content == "" {
		content = "(empty result)"
	}
	if toolUseID != nil && *toolUseID != "" {
		return fmt.Sprintf("[Tool Result (%s)]\n%s", *toolUseID, content)
	}
	return fmt.Sprintf("[Tool Result]\n%s", content)
}

// MergeAdjacentMessages merges adjacent messages with the same role
// Note: "tool" role is treated as user-equivalent for merging purposes
func MergeAdjacentMessages(messages []model.Message) []model.Message {
	if len(messages) == 0 {
		return nil
	}

	var merged []model.Message
	for _, msg := range messages {
		if len(merged) == 0 {
			merged = append(merged, msg)
			continue
		}

		last := &merged[len(merged)-1]

		// Check if roles are the same (treating "tool" as equivalent to "user")
		sameRole := msg.Role == last.Role
		if !sameRole {
			// Also merge if both are user-like (user or tool)
			currentIsUserLike := msg.Role == "user" || msg.Role == "tool"
			lastIsUserLike := last.Role == "user" || last.Role == "tool"
			if currentIsUserLike && lastIsUserLike {
				sameRole = true
			}
		}

		if sameRole {
			// Merge content
			lastContent := last.Content.GetFullContent()
			currentContent := msg.Content.GetFullContent()
			mergedContent := lastContent + "\n" + currentContent
			last.Content = model.MessageContent{Content: &mergedContent}

			// Merge tool calls
			if len(msg.ToolCalls) > 0 {
				last.ToolCalls = append(last.ToolCalls, msg.ToolCalls...)
			}

			// Merge reasoning content
			if msg.ReasoningContent != nil {
				if last.ReasoningContent != nil {
					merged := *last.ReasoningContent + "\n" + *msg.ReasoningContent
					last.ReasoningContent = &merged
				} else {
					last.ReasoningContent = msg.ReasoningContent
				}
			}
		} else {
			merged = append(merged, msg)
		}
	}

	return merged
}

// EnsureFirstMessageIsUser ensures the first message is from user
// Prepends a synthetic user message if needed
// Note: "tool" role is treated as user-equivalent for this check
func EnsureFirstMessageIsUser(messages []model.Message) []model.Message {
	if len(messages) == 0 {
		return messages
	}

	firstRole := messages[0].Role
	if firstRole == "user" || firstRole == "tool" {
		return messages
	}

	log.Debugf("kiro: first message is not 'user', prepending synthetic user message")
	emptyContent := "(empty)"
	return append([]model.Message{{
		Role:    "user",
		Content: model.MessageContent{Content: &emptyContent},
	}}, messages...)
}

// NormalizeMessageRoles normalizes unknown roles to user
// Note: "tool" role is preserved as it's handled separately in buildKiroHistory
func NormalizeMessageRoles(messages []model.Message) []model.Message {
	for i, msg := range messages {
		// Keep known roles: user, assistant, system, tool
		if msg.Role != "user" && msg.Role != "assistant" && msg.Role != "system" && msg.Role != "tool" {
			log.Debugf("kiro: normalizing role '%s' to 'user'", msg.Role)
			messages[i].Role = "user"
		}
	}
	return messages
}

// EnsureAlternatingRoles ensures alternating user/assistant roles
// Inserts empty assistant messages between consecutive user messages
// Note: "tool" role is treated as user-equivalent for alternation purposes
func EnsureAlternatingRoles(messages []model.Message) []model.Message {
	if len(messages) < 2 {
		return messages
	}

	var result []model.Message
	result = append(result, messages[0])

	for i := 1; i < len(messages); i++ {
		currentRole := messages[i].Role
		lastRole := result[len(result)-1].Role

		// Treat "tool" as user-equivalent
		currentIsUserLike := currentRole == "user" || currentRole == "tool"
		lastIsUserLike := lastRole == "user" || lastRole == "tool"

		if currentIsUserLike && lastIsUserLike {
			emptyContent := "(empty)"
			result = append(result, model.Message{
				Role:    "assistant",
				Content: model.MessageContent{Content: &emptyContent},
			})
		}
		result = append(result, messages[i])
	}

	return result
}

// ExtractTextContent extracts text content from a message content interface
func ExtractTextContent(content interface{}) string {
	if content == nil {
		return ""
	}

	switch v := content.(type) {
	case string:
		return v
	case *string:
		if v != nil {
			return *v
		}
		return ""
	case model.MessageContent:
		return v.GetFullContent()
	case []model.MessageContentPart:
		var texts []string
		for _, part := range v {
			if part.Type == "text" && part.Text != nil {
				texts = append(texts, *part.Text)
			}
		}
		return strings.Join(texts, "\n")
	default:
		// Try to marshal and extract
		b, err := json.Marshal(content)
		if err != nil {
			return ""
		}
		var str string
		if json.Unmarshal(b, &str) == nil {
			return str
		}
		return string(b)
	}
}

// ExtractImagesFromContent extracts images from OpenAI-style content
func ExtractImagesFromContent(content model.MessageContent) []map[string]interface{} {
	var images []map[string]interface{}

	for _, part := range content.MultipleContent {
		if part.Type != "image_url" || part.ImageURL == nil {
			continue
		}

		url := part.ImageURL.URL
		if url == "" {
			continue
		}

		// Parse data URL
		if len(url) > 5 && url[:5] == "data:" {
			mediaType, data := parseDataURL(url)
			if data != "" {
				images = append(images, map[string]interface{}{
					"media_type": mediaType,
					"data":       data,
				})
			}
		}
	}

	return images
}

func parseDataURL(url string) (string, string) {
	idx := strings.Index(url, ",")
	if idx == -1 {
		return "", ""
	}

	header := url[:idx]
	data := url[idx+1:]

	// Extract media type from header (data:image/jpeg;base64)
	mediaType := "image/jpeg"
	if len(header) > 5 {
		header = header[5:] // Remove "data:"
		if semiIdx := strings.Index(header, ";"); semiIdx != -1 {
			mediaType = header[:semiIdx]
		} else {
			mediaType = header
		}
	}

	return mediaType, data
}

// SanitizeJSONSchema removes fields that Kiro API doesn't accept
// This follows the same logic as kiro-go-proxy
func SanitizeJSONSchema(schema map[string]interface{}) map[string]interface{} {
	if schema == nil {
		return make(map[string]interface{})
	}

	result := make(map[string]interface{})

	for key, value := range schema {
		// Skip empty required arrays
		if key == "required" {
			if arr, ok := value.([]interface{}); ok && len(arr) == 0 {
				continue
			}
		}

		// Skip additionalProperties
		if key == "additionalProperties" {
			continue
		}

		// Recursively process nested objects
		switch v := value.(type) {
		case map[string]interface{}:
			if key == "properties" {
				props := make(map[string]interface{})
				for propKey, propValue := range v {
					if propMap, ok := propValue.(map[string]interface{}); ok {
						props[propKey] = SanitizeJSONSchema(propMap)
					} else {
						props[propKey] = propValue
					}
				}
				result[key] = props
			} else {
				result[key] = SanitizeJSONSchema(v)
			}
		case []interface{}:
			var newArr []interface{}
			for _, item := range v {
				if itemMap, ok := item.(map[string]interface{}); ok {
					newArr = append(newArr, SanitizeJSONSchema(itemMap))
				} else {
					newArr = append(newArr, item)
				}
			}
			result[key] = newArr
		default:
			result[key] = value
		}
	}

	return result
}
