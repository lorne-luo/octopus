package openai

import "encoding/json"

// EmbeddingRequest represents OpenAI Embedding API request.
type EmbeddingRequest struct {
	Model          string          `json:"model"`
	Input          EmbeddingInput  `json:"input"`
	EncodingFormat *string         `json:"encoding_format,omitempty"`
	Dimensions     *int64          `json:"dimensions,omitempty"`
	User           *string         `json:"user,omitempty"`
}

// EmbeddingInput represents embedding input (can be string or array).
type EmbeddingInput struct {
	Single  string
	Multiple []string
	Tokens  []int64
}

// EmbeddingResponse represents OpenAI Embedding API response.
type EmbeddingResponse struct {
	Object string          `json:"object"`
	Data   []EmbeddingData `json:"data"`
	Model  string          `json:"model"`
	Usage  *EmbeddingUsage `json:"usage,omitempty"`
}

// EmbeddingData represents a single embedding in the response.
type EmbeddingData struct {
	Object    string          `json:"object"`
	Index     int             `json:"index"`
	Embedding EmbeddingVector `json:"embedding"`
}

// EmbeddingVector represents embedding vector (float array or base64).
type EmbeddingVector struct {
	Floats  []float64
	Base64  string
	IsBase64 bool
}

// EmbeddingUsage represents token usage for embedding requests.
type EmbeddingUsage struct {
	PromptTokens int64 `json:"prompt_tokens"`
	TotalTokens  int64 `json:"total_tokens"`
}

// UnmarshalJSON handles both string and array forms of EmbeddingInput.
func (ei *EmbeddingInput) UnmarshalJSON(data []byte) error {
	// Try string first
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		ei.Single = str
		return nil
	}

	// Try array of strings
	var strArr []string
	if err := json.Unmarshal(data, &strArr); err == nil {
		ei.Multiple = strArr
		return nil
	}

	// Try array of int64 (token arrays)
	var intArr []int64
	if err := json.Unmarshal(data, &intArr); err == nil {
		ei.Tokens = intArr
		return nil
	}

	// Try nested array of int64 (multiple token arrays)
	var nestedArr [][]int64
	if err := json.Unmarshal(data, &nestedArr); err == nil {
		// For now, flatten to first - but this is uncommon
		if len(nestedArr) > 0 {
			ei.Tokens = nestedArr[0]
		}
		return nil
	}

	return nil
}

// MarshalJSON handles EmbeddingInput serialization.
func (ei EmbeddingInput) MarshalJSON() ([]byte, error) {
	if len(ei.Multiple) > 0 {
		return json.Marshal(ei.Multiple)
	}
	if len(ei.Tokens) > 0 {
		return json.Marshal(ei.Tokens)
	}
	if ei.Single != "" {
		return json.Marshal(ei.Single)
	}
	return json.Marshal(nil)
}

// IsArray returns true if input is an array.
func (ei EmbeddingInput) IsArray() bool {
	return len(ei.Multiple) > 0 || len(ei.Tokens) > 0
}

// ToStrings returns all input strings as a slice.
func (ei EmbeddingInput) ToStrings() []string {
	if len(ei.Multiple) > 0 {
		return ei.Multiple
	}
	if ei.Single != "" {
		return []string{ei.Single}
	}
	return nil
}

// UnmarshalJSON handles both float array and base64 string forms of EmbeddingVector.
func (ev *EmbeddingVector) UnmarshalJSON(data []byte) error {
	// Try float array first
	var floats []float64
	if err := json.Unmarshal(data, &floats); err == nil {
		ev.Floats = floats
		ev.IsBase64 = false
		return nil
	}

	// Try base64 string
	var base64Str string
	if err := json.Unmarshal(data, &base64Str); err == nil {
		ev.Base64 = base64Str
		ev.IsBase64 = true
		return nil
	}

	return nil
}

// MarshalJSON handles EmbeddingVector serialization.
func (ev EmbeddingVector) MarshalJSON() ([]byte, error) {
	if ev.IsBase64 && ev.Base64 != "" {
		return json.Marshal(ev.Base64)
	}
	return json.Marshal(ev.Floats)
}