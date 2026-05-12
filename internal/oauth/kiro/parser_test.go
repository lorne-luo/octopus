package kiro

import (
	"testing"
)

func TestFindMatchingBrace(t *testing.T) {
	tests := []struct {
		name      string
		text      string
		startPos  int
		expected  int
	}{
		{
			name:     "simple object",
			text:     `{"key":"value"}`,
			startPos: 0,
			expected: 14,
		},
		{
			name:     "nested object",
			text:     `{"outer":{"inner":"value"}}`,
			startPos: 0,
			expected: 26,
		},
		{
			name:     "object with string containing brace",
			text:     `{"key":"{value}"}`,
			startPos: 0,
			expected: 16,
		},
		{
			name:     "object with escaped quote",
			text:     `{"key":"value\"with\"quote"}`,
			startPos: 0,
			expected: 27,
		},
		{
			name:     "incomplete object",
			text:     `{"key":"value"`,
			startPos: 0,
			expected: -1,
		},
		{
			name:     "not starting with brace",
			text:     `"key":"value"}`,
			startPos: 0,
			expected: -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FindMatchingBrace(tt.text, tt.startPos)
			if result != tt.expected {
				t.Errorf("FindMatchingBrace(%q, %d) = %d, want %d", tt.text, tt.startPos, result, tt.expected)
			}
		})
	}
}

func TestAwsEventStreamParser_ContentEvent(t *testing.T) {
	parser := NewAwsEventStreamParser()

	// Feed a content event
	events := parser.Feed([]byte(`{"content":"Hello World"}`))

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	if events[0].Type != EventTypeContent {
		t.Errorf("expected EventTypeContent, got %s", events[0].Type)
	}

	data, ok := events[0].Data.(ContentData)
	if !ok {
		t.Fatalf("expected ContentData, got %T", events[0].Data)
	}

	if data.Content != "Hello World" {
		t.Errorf("expected content 'Hello World', got %q", data.Content)
	}
}

func TestAwsEventStreamParser_UsageEvent(t *testing.T) {
	parser := NewAwsEventStreamParser()

	events := parser.Feed([]byte(`{"usage":100}`))

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	if events[0].Type != EventTypeUsage {
		t.Errorf("expected EventTypeUsage, got %s", events[0].Type)
	}

	data, ok := events[0].Data.(UsageData)
	if !ok {
		t.Fatalf("expected UsageData, got %T", events[0].Data)
	}

	if data.Credits != 100 {
		t.Errorf("expected credits 100, got %d", data.Credits)
	}
}

func TestAwsEventStreamParser_ToolStartEvent(t *testing.T) {
	parser := NewAwsEventStreamParser()

	events := parser.Feed([]byte(`{"name":"test_func","toolUseId":"call_123","input":{"arg":"value"}}`))

	// Tool start events don't emit events directly
	if len(events) != 0 {
		t.Errorf("expected 0 events from tool_start, got %d", len(events))
	}

	// Check internal state
	toolCalls := parser.GetToolCalls()
	if len(toolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(toolCalls))
	}

	if toolCalls[0].Function.Name != "test_func" {
		t.Errorf("expected function name 'test_func', got %q", toolCalls[0].Function.Name)
	}

	if toolCalls[0].ID != "call_123" {
		t.Errorf("expected tool ID 'call_123', got %q", toolCalls[0].ID)
	}
}

func TestAwsEventStreamParser_MultipleEvents(t *testing.T) {
	parser := NewAwsEventStreamParser()

	// Feed multiple events in sequence
	events := parser.Feed([]byte(`{"content":"Hello"}{"content":" World"}{"usage":50}`))

	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}

	if events[0].Type != EventTypeContent {
		t.Errorf("expected first event to be content, got %s", events[0].Type)
	}

	if events[1].Type != EventTypeContent {
		t.Errorf("expected second event to be content, got %s", events[1].Type)
	}

	if events[2].Type != EventTypeUsage {
		t.Errorf("expected third event to be usage, got %s", events[2].Type)
	}
}

func TestAwsEventStreamParser_IncompleteJSON(t *testing.T) {
	parser := NewAwsEventStreamParser()

	// Feed incomplete JSON
	events := parser.Feed([]byte(`{"content":"Hello"`))

	// Should not emit any events
	if len(events) != 0 {
		t.Errorf("expected 0 events for incomplete JSON, got %d", len(events))
	}

	// Buffer should contain the incomplete JSON
	if parser.buffer != `{"content":"Hello"` {
		t.Errorf("expected buffer to contain incomplete JSON, got %q", parser.buffer)
	}

	// Complete the JSON
	events = parser.Feed([]byte(`}`))

	if len(events) != 1 {
		t.Errorf("expected 1 event after completing JSON, got %d", len(events))
	}
}

func TestAwsEventStreamParser_Deduplication(t *testing.T) {
	parser := NewAwsEventStreamParser()

	// Feed same content twice
	events := parser.Feed([]byte(`{"content":"Hello"}{"content":"Hello"}`))

	// Second identical content should be deduplicated
	if len(events) != 1 {
		t.Errorf("expected 1 event after deduplication, got %d", len(events))
	}
}

func TestAwsEventStreamParser_Reset(t *testing.T) {
	parser := NewAwsEventStreamParser()

	// Feed some events
	_ = parser.Feed([]byte(`{"content":"Hello"}`))

	// Reset
	parser.Reset()

	// Check state is cleared
	if parser.buffer != "" {
		t.Errorf("expected empty buffer after reset, got %q", parser.buffer)
	}

	if len(parser.toolCalls) != 0 {
		t.Errorf("expected empty tool calls after reset, got %d", len(parser.toolCalls))
	}
}

func TestParseBracketToolCalls(t *testing.T) {
	tests := []struct {
		name          string
		text          string
		expectedCount int
		expectedName  string
	}{
		{
			name:          "single tool call",
			text:          `Some text [Called my_func with args: {"arg": "value"}] more text`,
			expectedCount: 1,
			expectedName:  "my_func",
		},
		{
			name:          "no tool calls",
			text:          `Regular text without tool calls`,
			expectedCount: 0,
			expectedName:  "",
		},
		{
			name:          "case insensitive",
			text:          `[called test_func with args: {"arg":"value"}]`,
			expectedCount: 1,
			expectedName:  "test_func",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseBracketToolCalls(tt.text)

			if len(result) != tt.expectedCount {
				t.Errorf("expected %d tool calls, got %d", tt.expectedCount, len(result))
				return
			}

			if tt.expectedCount > 0 && result[0].Function.Name != tt.expectedName {
				t.Errorf("expected function name %q, got %q", tt.expectedName, result[0].Function.Name)
			}
		})
	}
}

func TestDeduplicateToolCalls(t *testing.T) {
	calls := []ToolCall{
		{ID: "call_1", Function: ToolCallFunction{Name: "func_a", Arguments: `{"arg":"value"}`}},
		{ID: "call_1", Function: ToolCallFunction{Name: "func_a", Arguments: `{}`}},
		{ID: "call_2", Function: ToolCallFunction{Name: "func_b", Arguments: `{}`}},
	}

	result := DeduplicateToolCalls(calls)

	// Should have 2 unique IDs
	if len(result) != 2 {
		t.Errorf("expected 2 deduplicated calls, got %d", len(result))
	}

	// Find call_1 and verify it has the non-empty arguments
	for _, call := range result {
		if call.ID == "call_1" {
			if call.Function.Arguments == `{}` {
				t.Error("expected call_1 to have non-empty arguments")
			}
		}
	}
}

func TestGenerateToolCallID(t *testing.T) {
	id1 := generateToolCallID()
	id2 := generateToolCallID()

	// Should have correct prefix
	if len(id1) < 5 || id1[:5] != "call_" {
		t.Errorf("expected ID to start with 'call_', got %q", id1)
	}

	// Should be unique
	if id1 == id2 {
		t.Error("expected unique IDs, got duplicates")
	}
}
