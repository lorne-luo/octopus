package kiro

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/bestruirui/octopus/internal/transformer/model"
	"github.com/bestruirui/octopus/internal/utils/log"
	"github.com/google/uuid"
)

// KiroPayload represents the Kiro API payload
type KiroPayload struct {
	ConversationState struct {
		ChatTriggerType string        `json:"chatTriggerType"`
		ConversationID  string        `json:"conversationId"`
		CurrentMessage  CurrentMessage `json:"currentMessage"`
		History         []interface{} `json:"history,omitempty"`
	} `json:"conversationState"`
	ProfileArn string `json:"profileArn,omitempty"`
}

// CurrentMessage represents the current message in Kiro format
type CurrentMessage struct {
	UserInputMessage UserInputMessage `json:"userInputMessage"`
}

// UserInputMessage represents user input in Kiro format
type UserInputMessage struct {
	Content                 string                   `json:"content"`
	ModelID                 string                   `json:"modelId"`
	Origin                  string                   `json:"origin"`
	Images                  []map[string]interface{} `json:"images,omitempty"`
	UserInputMessageContext *UserInputMessageContext `json:"userInputMessageContext,omitempty"`
}

// UserInputMessageContext contains tools and tool results
type UserInputMessageContext struct {
	Tools       []map[string]interface{} `json:"tools,omitempty"`
	ToolResults []map[string]interface{} `json:"toolResults,omitempty"`
}

// RequestOutbound handles transforming OpenAI requests to Kiro format
type RequestOutbound struct {
	profileArn string
}

// NewRequestOutbound creates a new request transformer
func NewRequestOutbound() *RequestOutbound {
	return &RequestOutbound{}
}

// SetProfileArn sets the profile ARN for Kiro requests
func (t *RequestOutbound) SetProfileArn(arn string) {
	t.profileArn = arn
}

// TransformRequest transforms an InternalLLMRequest to Kiro format HTTP request
func (t *RequestOutbound) TransformRequest(ctx context.Context, request *model.InternalLLMRequest, baseUrl, key string) (*http.Request, error) {
	// Resolve model name
	modelID := ResolveModel(request.Model)

	// Build Kiro payload
	payload := t.buildKiroPayload(request, modelID)

	// Marshal payload
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("kiro: marshal request failed: %w", err)
	}

	// Build request URL
	requestURL := baseUrl
	if !strings.HasSuffix(requestURL, "/") {
		requestURL += "/"
	}
	requestURL += "generateAssistantResponse"

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("kiro: create request failed: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Accept", "application/vnd.amazon.eventstream")

	return req, nil
}

// buildKiroPayload builds the Kiro API payload from an internal request
func (t *RequestOutbound) buildKiroPayload(request *model.InternalLLMRequest, modelID string) *KiroPayload {
	// Extract system prompt from first system message
	var systemPrompt string
	var messages []model.Message
	for _, msg := range request.Messages {
		if msg.Role == "system" {
			systemPrompt = msg.Content.GetFullContent()
			continue
		}
		messages = append(messages, msg)
	}

	// Process tools with long descriptions (max 4000 chars per Kiro API limit)
	// This follows the same logic as kiro-go-proxy
	const toolDescriptionMaxLength = 4000
	processedTools := ProcessToolsWithLongDescriptions(request.Tools, toolDescriptionMaxLength)
	ValidateToolNames(processedTools.Tools)

	// Build full system prompt with tool docs if needed
	fullSystemPrompt := systemPrompt
	if processedTools.ToolDocs != "" {
		if fullSystemPrompt != "" {
			fullSystemPrompt += processedTools.ToolDocs
		} else {
			fullSystemPrompt = strings.TrimSpace(processedTools.ToolDocs)
		}
	}

	// Process messages
	if len(request.Tools) == 0 {
		messages, _ = StripAllToolContent(messages)
	}

	messages = MergeAdjacentMessages(messages)
	messages = EnsureFirstMessageIsUser(messages)
	messages = NormalizeMessageRoles(messages)
	messages = EnsureAlternatingRoles(messages)

	if len(messages) == 0 {
		log.Warnf("kiro: no messages to send")
		return nil
	}

	// Build payload
	payload := &KiroPayload{}
	payload.ConversationState.ChatTriggerType = "MANUAL"
	payload.ConversationState.ConversationID = generateConversationID()

	// Build history (all except last)
	var history []interface{}
	historyMessages := messages[:len(messages)-1]
	if len(historyMessages) > 0 {
		if fullSystemPrompt != "" {
			// Add system prompt to first user message
			for i, msg := range historyMessages {
				if msg.Role == "user" {
					content := msg.Content.GetFullContent()
					systemContent := fullSystemPrompt + "\n\n" + content
					messages[i].Content = model.MessageContent{Content: &systemContent}
					break
				}
			}
		}
		history = buildKiroHistory(historyMessages, modelID)
	}

	// Current message
	currentMessage := messages[len(messages)-1]
	currentContent := currentMessage.Content.GetFullContent()

	// Add system prompt if no history
	if fullSystemPrompt != "" && len(history) == 0 {
		currentContent = fullSystemPrompt + "\n\n" + currentContent
	}

	// Handle assistant as current message
	if currentMessage.Role == "assistant" {
		history = append(history, map[string]interface{}{
			"assistantResponseMessage": map[string]interface{}{
				"content": currentContent,
			},
		})
		currentContent = "Continue"
	}

	// Handle tool role as current message (convert to user message with tool result)
	var toolResultForCurrent *ToolResult
	if currentMessage.Role == "tool" {
		toolResultForCurrent = &ToolResult{
			ToolUseID: "",
			Content:   currentContent,
		}
		if currentMessage.ToolCallID != nil {
			toolResultForCurrent.ToolUseID = *currentMessage.ToolCallID
		}
	}

	// Handle empty content
	if currentContent == "" {
		currentContent = "Continue"
	}

	// Build user input message
	userInput := UserInputMessage{
		Content: currentContent,
		ModelID: modelID,
		Origin:  "AI_EDITOR",
	}

	// Process images
	images := ExtractImagesFromContent(currentMessage.Content)
	if len(images) > 0 {
		userInput.Images = convertImagesToKiroFormat(images)
	}

	// Build context for tools and tool results
	var context *UserInputMessageContext
	if len(processedTools.Tools) > 0 || len(currentMessage.ToolCalls) > 0 || toolResultForCurrent != nil {
		context = &UserInputMessageContext{}
		if len(processedTools.Tools) > 0 {
			context.Tools = convertToolsToKiroFormat(processedTools.Tools)
		}
		if toolResultForCurrent != nil {
			context.ToolResults = ConvertToolResultsToKiroFormat([]ToolResult{*toolResultForCurrent})
		}
	}

	userInput.UserInputMessageContext = context
	payload.ConversationState.CurrentMessage.UserInputMessage = userInput

	if len(history) > 0 {
		payload.ConversationState.History = history
	}

	if t.profileArn != "" {
		payload.ProfileArn = t.profileArn
	}

	return payload
}

// buildKiroHistory builds Kiro history from messages
func buildKiroHistory(messages []model.Message, modelID string) []interface{} {
	var history []interface{}

	for _, msg := range messages {
		if msg.Role == "user" {
			content := msg.Content.GetFullContent()
			if content == "" {
				content = "(empty)"
			}

			userInput := map[string]interface{}{
				"content": content,
				"modelId": modelID,
				"origin":  "AI_EDITOR",
			}

			// Process images
			images := ExtractImagesFromContent(msg.Content)
			if len(images) > 0 {
				userInput["images"] = convertImagesToKiroFormat(images)
			}

			// Process tool results attached to user message (from ToolCallID field)
			if msg.ToolCallID != nil {
				context := map[string]interface{}{
					"toolResults": []map[string]interface{}{{
						"content": []map[string]interface{}{
							{"text": content},
						},
						"status":    "success",
						"toolUseId": *msg.ToolCallID,
					}},
				}
				userInput["userInputMessageContext"] = context
			}

			history = append(history, map[string]interface{}{
				"userInputMessage": userInput,
			})
		} else if msg.Role == "assistant" {
			content := msg.Content.GetFullContent()
			if content == "" {
				content = "(empty)"
			}

			assistant := map[string]interface{}{
				"content": content,
			}

			// Process tool uses
			if len(msg.ToolCalls) > 0 {
				var toolUses []map[string]interface{}
				for _, tc := range msg.ToolCalls {
					var input interface{}
					json.Unmarshal([]byte(tc.Function.Arguments), &input)
					toolUses = append(toolUses, map[string]interface{}{
						"name":      tc.Function.Name,
						"input":     input,
						"toolUseId": tc.ID,
					})
				}
				assistant["toolUses"] = toolUses
			}

			history = append(history, map[string]interface{}{
				"assistantResponseMessage": assistant,
			})
		} else if msg.Role == "tool" {
			// Handle tool role messages (from Anthropic tool_result blocks)
			// In Kiro format, tool results are attached to a user message
			content := msg.Content.GetFullContent()
			if content == "" {
				content = "(empty)"
			}

			toolUseID := ""
			if msg.ToolCallID != nil {
				toolUseID = *msg.ToolCallID
			}

			userInput := map[string]interface{}{
				"content": content,
				"modelId": modelID,
				"origin":  "AI_EDITOR",
			}

			// Build tool result context
			context := map[string]interface{}{
				"toolResults": []map[string]interface{}{{
					"content": []map[string]interface{}{
						{"text": content},
					},
					"status":    "success",
					"toolUseId": toolUseID,
				}},
			}
			userInput["userInputMessageContext"] = context

			history = append(history, map[string]interface{}{
				"userInputMessage": userInput,
			})
		}
	}

	return history
}

// ProcessedTools contains the result of processing tools with long descriptions
type ProcessedTools struct {
	Tools    []model.Tool
	ToolDocs string
}

// ProcessToolsWithLongDescriptions processes tools with long descriptions
// If a tool description exceeds maxLen, it moves the full description to the system prompt
// and replaces it with a reference. This follows the same logic as kiro-go-proxy.
func ProcessToolsWithLongDescriptions(tools []model.Tool, maxLen int) ProcessedTools {
	if len(tools) == 0 || maxLen <= 0 {
		return ProcessedTools{Tools: tools}
	}

	var processed []model.Tool
	var docParts []string

	for _, tool := range tools {
		if tool.Type != "function" {
			processed = append(processed, tool)
			continue
		}

		if len(tool.Function.Description) <= maxLen {
			processed = append(processed, tool)
		} else {
			log.Debugf("kiro: tool '%s' has long description (%d chars > %d), moving to system prompt",
				tool.Function.Name, len(tool.Function.Description), maxLen)

			docParts = append(docParts, fmt.Sprintf("## Tool: %s\n\n%s", tool.Function.Name, tool.Function.Description))

			// Create tool with shortened description
			newTool := tool
			newTool.Function.Description = fmt.Sprintf("[Full documentation in system prompt under '## Tool: %s']", tool.Function.Name)
			processed = append(processed, newTool)
		}
	}

	var toolDocs string
	if len(docParts) > 0 {
		toolDocs = "\n\n---\n# Tool Documentation\nThe following tools have detailed documentation that couldn't fit in the tool definition.\n\n" +
			strings.Join(docParts, "\n\n---\n\n")
	}

	return ProcessedTools{
		Tools:    processed,
		ToolDocs: toolDocs,
	}
}

// ValidateToolNames validates tool names against Kiro API limit
// Kiro has a 64 character limit on tool names
func ValidateToolNames(tools []model.Tool) {
	for _, tool := range tools {
		if tool.Type == "function" && len(tool.Function.Name) > 64 {
			log.Warnf("kiro: tool name '%s' exceeds 64 character limit (%d chars)", tool.Function.Name, len(tool.Function.Name))
		}
	}
}

// convertToolsToKiroFormat converts tools to Kiro format
func convertToolsToKiroFormat(tools []model.Tool) []map[string]interface{} {
	var result []map[string]interface{}

	for _, tool := range tools {
		if tool.Type != "function" {
			continue
		}

		desc := tool.Function.Description
		if desc == "" {
			desc = "Tool: " + tool.Function.Name
		}

		var params interface{}
		if len(tool.Function.Parameters) > 0 {
			json.Unmarshal(tool.Function.Parameters, &params)
			// Sanitize the JSON schema for Kiro API compatibility
			if paramsMap, ok := params.(map[string]interface{}); ok {
				params = SanitizeJSONSchema(paramsMap)
			}
		} else {
			params = map[string]interface{}{}
		}

		result = append(result, map[string]interface{}{
			"toolSpecification": map[string]interface{}{
				"name":        tool.Function.Name,
				"description": desc,
				"inputSchema": map[string]interface{}{
					"json": params,
				},
			},
		})
	}

	return result
}

// convertImagesToKiroFormat converts images to Kiro format
func convertImagesToKiroFormat(images []map[string]interface{}) []map[string]interface{} {
	var result []map[string]interface{}

	for _, img := range images {
		mediaType, _ := img["media_type"].(string)
		if mediaType == "" {
			mediaType = "image/jpeg"
		}

		data, _ := img["data"].(string)
		if data == "" {
			continue
		}

		// Extract format from media type
		format := mediaType
		if strings.Contains(mediaType, "/") {
			format = mediaType[strings.Index(mediaType, "/")+1:]
		}

		result = append(result, map[string]interface{}{
			"format": format,
			"source": map[string]interface{}{
				"bytes": data,
			},
		})
	}

	return result
}

// BuildPassthroughRequest implements PassthroughOutbound interface
// Note: Kiro doesn't support true passthrough as it requires request transformation
func (t *RequestOutbound) BuildPassthroughRequest(ctx context.Context, rawBody []byte, stream bool, baseUrl, key string, query url.Values) (*http.Request, error) {
	// Kiro doesn't support passthrough - use TransformRequest instead
	return nil, fmt.Errorf("kiro: passthrough not supported, use TransformRequest")
}

func generateConversationID() string {
	return uuid.New().String()
}

// ToolResult represents a tool result in unified format
type ToolResult struct {
	ToolUseID string
	Content   interface{}
}

// ConvertToolResultsToKiroFormat converts tool results to Kiro format
func ConvertToolResultsToKiroFormat(results []ToolResult) []map[string]interface{} {
	var kiroResults []map[string]interface{}

	for _, tr := range results {
		content := ExtractTextContent(tr.Content)
		if content == "" {
			content = "(empty result)"
		}

		kiroResults = append(kiroResults, map[string]interface{}{
			"content": []map[string]interface{}{
				{"text": content},
			},
			"status":    "success",
			"toolUseId": tr.ToolUseID,
		})
	}

	return kiroResults
}
