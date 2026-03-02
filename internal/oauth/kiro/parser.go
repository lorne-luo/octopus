package kiro

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/bestruirui/octopus/internal/utils/log"
	"github.com/google/uuid"
)

// EventType represents the type of parsed event
type EventType string

const (
	EventTypeContent      EventType = "content"
	EventTypeToolStart    EventType = "tool_start"
	EventTypeToolInput    EventType = "tool_input"
	EventTypeToolStop     EventType = "tool_stop"
	EventTypeUsage        EventType = "usage"
	EventTypeContextUsage EventType = "context_usage"
	EventTypeThinking     EventType = "thinking"
)

// Event represents a parsed event
type Event struct {
	Type EventType
	Data interface{}
}

// ContentData represents content event data
type ContentData struct {
	Content string
}

// ThinkingData represents thinking event data
type ThinkingData struct {
	Content string
}

// ToolStartData represents tool start event data
type ToolStartData struct {
	Name      string
	ToolUseID string
	Input     interface{}
	Stop      bool
}

// ToolInputData represents tool input continuation data
type ToolInputData struct {
	Input interface{}
}

// UsageData represents usage event data
type UsageData struct {
	Credits int `json:"credits"`
}

// ContextUsageData represents context usage percentage
type ContextUsageData struct {
	Percentage float64
}

// ToolCall represents a completed tool call
type ToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function ToolCallFunction `json:"function"`
}

// ToolCallFunction represents function details
type ToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// AwsEventStreamParser parses AWS Event Stream format
type AwsEventStreamParser struct {
	buffer          string
	lastContent     *string
	currentToolCall *ToolCall
	toolCalls       []ToolCall
}

// NewAwsEventStreamParser creates a new parser
func NewAwsEventStreamParser() *AwsEventStreamParser {
	return &AwsEventStreamParser{
		toolCalls: make([]ToolCall, 0),
	}
}

// Feed adds a chunk to the buffer and returns parsed events
func (p *AwsEventStreamParser) Feed(chunk []byte) []Event {
	p.buffer += string(chunk)
	var events []Event

	for {
		// Find the earliest pattern
		earliestPos := -1
		var earliestType EventType

		patterns := []struct {
			pattern string
			t       EventType
		}{
			{`{"content":`, EventTypeContent},
			{`{"name":`, EventTypeToolStart},
			{`{"input":`, EventTypeToolInput},
			{`{"stop":`, EventTypeToolStop},
			{`{"usage":`, EventTypeUsage},
			{`{"contextUsagePercentage":`, EventTypeContextUsage},
			{`{"thinking":`, EventTypeThinking},
		}

		for _, pat := range patterns {
			pos := strings.Index(p.buffer, pat.pattern)
			if pos != -1 && (earliestPos == -1 || pos < earliestPos) {
				earliestPos = pos
				earliestType = pat.t
			}
		}

		if earliestPos == -1 {
			break
		}

		// Find JSON end
		jsonEnd := FindMatchingBrace(p.buffer, earliestPos)
		if jsonEnd == -1 {
			break // JSON not complete, wait for more data
		}

		jsonStr := p.buffer[earliestPos : jsonEnd+1]
		p.buffer = p.buffer[jsonEnd+1:]

		event, err := p.processEvent(jsonStr, earliestType)
		if err != nil {
			log.Warnf("kiro parser: failed to parse JSON: %v (data: %.100s...)", err, jsonStr)
			continue
		}

		if event != nil {
			events = append(events, *event)
		}
	}

	return events
}

// processEvent processes a parsed JSON event
func (p *AwsEventStreamParser) processEvent(jsonStr string, eventType EventType) (*Event, error) {
	switch eventType {
	case EventTypeContent:
		return p.processContentEvent(jsonStr)
	case EventTypeThinking:
		return p.processThinkingEvent(jsonStr)
	case EventTypeToolStart:
		return p.processToolStartEvent(jsonStr)
	case EventTypeToolInput:
		return p.processToolInputEvent(jsonStr)
	case EventTypeToolStop:
		return p.processToolStopEvent(jsonStr)
	case EventTypeUsage:
		return p.processUsageEvent(jsonStr)
	case EventTypeContextUsage:
		return p.processContextUsageEvent(jsonStr)
	}
	return nil, nil
}

func (p *AwsEventStreamParser) processContentEvent(jsonStr string) (*Event, error) {
	var data struct {
		Content        string `json:"content"`
		FollowupPrompt string `json:"followupPrompt"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		return nil, err
	}

	// Skip followupPrompt
	if data.FollowupPrompt != "" {
		return nil, nil
	}

	// Deduplicate repeating content
	if p.lastContent != nil && data.Content == *p.lastContent {
		return nil, nil
	}

	p.lastContent = &data.Content

	return &Event{
		Type: EventTypeContent,
		Data: ContentData{Content: data.Content},
	}, nil
}

func (p *AwsEventStreamParser) processThinkingEvent(jsonStr string) (*Event, error) {
	var data struct {
		Thinking string `json:"thinking"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		return nil, err
	}

	if data.Thinking == "" {
		return nil, nil
	}

	return &Event{
		Type: EventTypeThinking,
		Data: ThinkingData{Content: data.Thinking},
	}, nil
}

func (p *AwsEventStreamParser) processToolStartEvent(jsonStr string) (*Event, error) {
	// Finalize previous tool call if exists
	if p.currentToolCall != nil {
		p.finalizeToolCall()
	}

	var data struct {
		Name      string      `json:"name"`
		ToolUseID string      `json:"toolUseId"`
		Input     interface{} `json:"input"`
		Stop      bool        `json:"stop"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		return nil, err
	}

	inputStr := ""
	switch v := data.Input.(type) {
	case string:
		inputStr = v
	case map[string]interface{}:
		b, _ := json.Marshal(v)
		inputStr = string(b)
	}

	p.currentToolCall = &ToolCall{
		ID:   data.ToolUseID,
		Type: "function",
		Function: ToolCallFunction{
			Name:      data.Name,
			Arguments: inputStr,
		},
	}

	if data.Stop {
		p.finalizeToolCall()
	}

	return nil, nil
}

func (p *AwsEventStreamParser) processToolInputEvent(jsonStr string) (*Event, error) {
	if p.currentToolCall == nil {
		return nil, nil
	}

	var data struct {
		Input interface{} `json:"input"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		return nil, err
	}

	var inputStr string
	switch v := data.Input.(type) {
	case string:
		inputStr = v
	case map[string]interface{}:
		b, _ := json.Marshal(v)
		inputStr = string(b)
	}

	p.currentToolCall.Function.Arguments += inputStr
	return nil, nil
}

func (p *AwsEventStreamParser) processToolStopEvent(jsonStr string) (*Event, error) {
	var data struct {
		Stop bool `json:"stop"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		return nil, err
	}

	if p.currentToolCall != nil && data.Stop {
		p.finalizeToolCall()
	}

	return nil, nil
}

func (p *AwsEventStreamParser) processUsageEvent(jsonStr string) (*Event, error) {
	var data struct {
		Usage int `json:"usage"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		return nil, err
	}

	return &Event{
		Type: EventTypeUsage,
		Data: UsageData{Credits: data.Usage},
	}, nil
}

func (p *AwsEventStreamParser) processContextUsageEvent(jsonStr string) (*Event, error) {
	var data struct {
		ContextUsagePercentage float64 `json:"contextUsagePercentage"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		return nil, err
	}

	return &Event{
		Type: EventTypeContextUsage,
		Data: ContextUsageData{Percentage: data.ContextUsagePercentage},
	}, nil
}

func (p *AwsEventStreamParser) finalizeToolCall() {
	if p.currentToolCall == nil {
		return
	}

	toolName := p.currentToolCall.Function.Name
	args := p.currentToolCall.Function.Arguments

	log.Debugf("kiro parser: finalizing tool call '%s' with arguments: %.200s...", toolName, args)

	// Try to parse and normalize arguments as JSON
	if args != "" {
		var parsed interface{}
		if err := json.Unmarshal([]byte(args), &parsed); err == nil {
			// Re-serialize to ensure valid JSON
			b, _ := json.Marshal(parsed)
			p.currentToolCall.Function.Arguments = string(b)
		} else {
			log.Warnf("kiro parser: failed to parse tool '%s' arguments: %v", toolName, err)
			p.currentToolCall.Function.Arguments = "{}"
		}
	} else {
		p.currentToolCall.Function.Arguments = "{}"
	}

	// Generate ID if missing
	if p.currentToolCall.ID == "" {
		p.currentToolCall.ID = generateToolCallID()
	}

	p.toolCalls = append(p.toolCalls, *p.currentToolCall)
	p.currentToolCall = nil
}

// GetToolCalls returns all collected tool calls
func (p *AwsEventStreamParser) GetToolCalls() []ToolCall {
	if p.currentToolCall != nil {
		p.finalizeToolCall()
	}
	return DeduplicateToolCalls(p.toolCalls)
}

// Reset resets the parser state
func (p *AwsEventStreamParser) Reset() {
	p.buffer = ""
	p.lastContent = nil
	p.currentToolCall = nil
	p.toolCalls = make([]ToolCall, 0)
}

// FindMatchingBrace finds the position of the closing brace
func FindMatchingBrace(text string, startPos int) int {
	if startPos >= len(text) || text[startPos] != '{' {
		return -1
	}

	braceCount := 0
	inString := false
	escapeNext := false

	for i := startPos; i < len(text); i++ {
		char := text[i]

		if escapeNext {
			escapeNext = false
			continue
		}

		if char == '\\' && inString {
			escapeNext = true
			continue
		}

		if char == '"' && !escapeNext {
			inString = !inString
			continue
		}

		if !inString {
			if char == '{' {
				braceCount++
			} else if char == '}' {
				braceCount--
				if braceCount == 0 {
					return i
				}
			}
		}
	}

	return -1
}

// ParseBracketToolCalls parses tool calls in [Called func_name with args: {...}] format
func ParseBracketToolCalls(responseText string) []ToolCall {
	if responseText == "" {
		return nil
	}
	// Check for tool call pattern (case-insensitive)
	if !strings.Contains(strings.ToLower(responseText), "[called") {
		return nil
	}

	var toolCalls []ToolCall
	pattern := regexp.MustCompile(`(?i)\[Called\s+(\w+)\s+with\s+args:\s*`)

	matches := pattern.FindAllStringSubmatchIndex(responseText, -1)
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}

		funcName := responseText[match[2]:match[3]]
		argsStart := match[1]

		// Find JSON start
		jsonStart := strings.Index(responseText[argsStart:], "{")
		if jsonStart == -1 {
			continue
		}
		jsonStart += argsStart

		// Find JSON end
		jsonEnd := FindMatchingBrace(responseText, jsonStart)
		if jsonEnd == -1 {
			continue
		}

		jsonStr := responseText[jsonStart : jsonEnd+1]

		var args interface{}
		if err := json.Unmarshal([]byte(jsonStr), &args); err == nil {
			argsJSON, _ := json.Marshal(args)
			toolCalls = append(toolCalls, ToolCall{
				ID:   generateToolCallID(),
				Type: "function",
				Function: ToolCallFunction{
					Name:      funcName,
					Arguments: string(argsJSON),
				},
			})
		} else {
			log.Warnf("kiro parser: failed to parse tool call arguments: %.100s...", jsonStr)
		}
	}

	return toolCalls
}

// DeduplicateToolCalls removes duplicate tool calls
func DeduplicateToolCalls(toolCalls []ToolCall) []ToolCall {
	if len(toolCalls) == 0 {
		return nil
	}

	// Deduplicate by ID - keep tool call with non-empty arguments
	byID := make(map[string]ToolCall)
	for _, tc := range toolCalls {
		if tc.ID == "" {
			continue
		}

		existing, ok := byID[tc.ID]
		if !ok {
			byID[tc.ID] = tc
		} else {
			// Prefer non-empty arguments
			if tc.Function.Arguments != "{}" && (existing.Function.Arguments == "{}" ||
				len(tc.Function.Arguments) > len(existing.Function.Arguments)) {
				byID[tc.ID] = tc
			}
		}
	}

	// Collect tool calls with ID
	var result []ToolCall
	for _, tc := range byID {
		result = append(result, tc)
	}

	// Add tool calls without ID
	for _, tc := range toolCalls {
		if tc.ID == "" {
			result = append(result, tc)
		}
	}

	// Deduplicate by name+arguments
	seen := make(map[string]bool)
	var unique []ToolCall
	for _, tc := range result {
		key := fmt.Sprintf("%s-%s", tc.Function.Name, tc.Function.Arguments)
		if !seen[key] {
			seen[key] = true
			unique = append(unique, tc)
		}
	}

	if len(toolCalls) != len(unique) {
		log.Debugf("kiro parser: deduplicated tool calls: %d -> %d", len(toolCalls), len(unique))
	}

	return unique
}

func generateToolCallID() string {
	return "call_" + uuid.New().String()[:24]
}
