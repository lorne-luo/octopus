package openai

import (
	"encoding/json"
)

// UnmarshalJSON handles both string and array forms of MessageContent.
func (mc *MessageContent) UnmarshalJSON(data []byte) error {
	// Try string first
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		mc.Text = &str
		return nil
	}

	// Try array of parts
	var parts []ContentPart
	if err := json.Unmarshal(data, &parts); err != nil {
		return err
	}
	mc.Parts = parts
	return nil
}

// MarshalJSON handles MessageContent serialization.
// Single text part is serialized as plain string, otherwise array.
func (mc MessageContent) MarshalJSON() ([]byte, error) {
	// If we have a direct text, serialize as string
	if mc.Text != nil {
		return json.Marshal(*mc.Text)
	}

	// If we have parts
	if len(mc.Parts) > 0 {
		// Optimization: single text part serializes as plain string
		if len(mc.Parts) == 1 && mc.Parts[0].Type == "text" && mc.Parts[0].Text != nil {
			return json.Marshal(*mc.Parts[0].Text)
		}
		return json.Marshal(mc.Parts)
	}

	// No content
	return json.Marshal(nil)
}

// IsString returns true if content is a simple string.
func (mc MessageContent) IsString() bool {
	return mc.Text != nil || (len(mc.Parts) == 1 && mc.Parts[0].Type == "text" && mc.Parts[0].Text != nil)
}

// String returns the text content if it's a string, empty otherwise.
func (mc MessageContent) String() string {
	if mc.Text != nil {
		return *mc.Text
	}
	if len(mc.Parts) == 1 && mc.Parts[0].Type == "text" && mc.Parts[0].Text != nil {
		return *mc.Parts[0].Text
	}
	return ""
}

// UnmarshalJSON handles both string and array forms of StopSequences.
func (s *StopSequences) UnmarshalJSON(data []byte) error {
	// Try string first
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		s.Single = str
		return nil
	}

	// Try array
	var arr []string
	if err := json.Unmarshal(data, &arr); err != nil {
		return err
	}
	s.Multiple = arr
	return nil
}

// MarshalJSON handles StopSequences serialization.
func (s StopSequences) MarshalJSON() ([]byte, error) {
	if len(s.Multiple) > 0 {
		return json.Marshal(s.Multiple)
	}
	if s.Single != "" {
		return json.Marshal(s.Single)
	}
	return json.Marshal(nil)
}

// ToSlice converts StopSequences to a slice.
func (s StopSequences) ToSlice() []string {
	if s.Multiple != nil {
		return s.Multiple
	}
	if s.Single != "" {
		return []string{s.Single}
	}
	return nil
}

// UnmarshalJSON handles both string and object forms of ToolChoice.
func (tc *ToolChoice) UnmarshalJSON(data []byte) error {
	// Try string first
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		tc.Mode = str
		return nil
	}

	// Try object form: {"type": "function", "function": {"name": "..."}}
	var obj struct {
		Type     string `json:"type"`
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
	}
	if err := json.Unmarshal(data, &obj); err != nil {
		return err
	}
	tc.Mode = obj.Type
	tc.Function = &obj.Function.Name
	return nil
}

// MarshalJSON handles ToolChoice serialization.
func (tc ToolChoice) MarshalJSON() ([]byte, error) {
	// String form for simple modes
	if tc.Function == nil {
		return json.Marshal(tc.Mode)
	}

	// Object form for named function
	return json.Marshal(struct {
		Type     string `json:"type"`
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
	}{
		Type: tc.Mode,
		Function: struct {
			Name string `json:"name"`
		}{Name: *tc.Function},
	})
}

// mergeExtraBody merges opaque ExtraBody JSON into the serialized request body.
// ExtraBody keys take precedence over existing keys (user intent).
// Returns original body unchanged if extraBody is nil or empty.
func mergeExtraBody(body []byte, extraBody json.RawMessage) ([]byte, error) {
	if len(extraBody) == 0 {
		return body, nil
	}

	var base map[string]interface{}
	if err := json.Unmarshal(body, &base); err != nil {
		return body, nil // non-object body, return as-is
	}

	var extra map[string]interface{}
	if err := json.Unmarshal(extraBody, &extra); err != nil {
		return body, nil // invalid extra body, ignore
	}

	// Merge: ExtraBody keys take precedence
	for k, v := range extra {
		base[k] = v
	}

	return json.Marshal(base)
}
