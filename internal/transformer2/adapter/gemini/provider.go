package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/bestruirui/octopus/internal/transformer2/canonical"
	"github.com/bestruirui/octopus/internal/transformer2/urlutil"
)

// ProviderAdapter implements ProviderAdapter for Gemini backends.
type ProviderAdapter struct{}

// NewProviderAdapter creates a new ProviderAdapter.
func NewProviderAdapter() *ProviderAdapter {
	return &ProviderAdapter{}
}

// BuildRequest builds an HTTP request for Gemini from canonical Request.
func (a *ProviderAdapter) BuildRequest(ctx context.Context, req *canonical.Request, baseURL, key string) (*http.Request, error) {
	greq := convertCanonicalToGeminiRequest(req)

	body, err := json.Marshal(greq)
	if err != nil {
		return nil, err
	}

	// Merge ExtraBody if present (user intent takes precedence)
	if len(req.ExtraBody) > 0 {
		body, err = mergeExtraBody(body, req.ExtraBody)
		if err != nil {
			return nil, err
		}
	}

	// Build URL based on request kind
	var endpoint string
	switch req.Kind {
	case canonical.KindCountTokens:
		endpoint = "/v1beta/models/" + req.Model + ":countTokens"
	default:
		if req.Stream {
			endpoint = "/v1beta/models/" + req.Model + ":streamGenerateContent?alt=sse"
		} else {
			endpoint = "/v1beta/models/" + req.Model + ":generateContent"
		}
	}

	reqURL, err := urlutil.BuildURL(baseURL, endpoint)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, "POST", reqURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-goog-api-key", key)

	return httpReq, nil
}

// convertCanonicalToGeminiRequest converts canonical Request to Gemini format.
func convertCanonicalToGeminiRequest(req *canonical.Request) *GenerateContentRequest {
	greq := &GenerateContentRequest{}

	// Extract system/developer messages into system_instruction
	greq.SystemInstruction = buildSystemInstruction(req.Messages)

	// Convert remaining messages to contents
	nonSystemMessages := filterNonSystemMessages(req.Messages)
	greq.Contents = make([]Content, len(nonSystemMessages))
	for i, msg := range nonSystemMessages {
		greq.Contents[i] = convertCanonicalToGeminiContent(msg)
	}

	// Convert tools
	if len(req.Tools) > 0 {
		tool := Tool{
			FunctionDeclarations: make([]FunctionDeclaration, len(req.Tools)),
		}
		for i, t := range req.Tools {
			tool.FunctionDeclarations[i] = convertCanonicalToGeminiTool(t)
		}
		greq.Tools = []Tool{tool}
	}

	// Convert tool choice
	if req.ToolChoice != nil {
		greq.ToolConfig = &ToolConfig{
			FunctionCallingConfig: mapCanonicalToolChoiceToGemini(req.ToolChoice),
		}
	}

	// Build generation config
	greq.GenerationConfig = buildGenerationConfig(req)

	// Preserve safety settings from hints
	if len(req.Hints.GeminiSafetySettings) > 0 {
		var settings []SafetySetting
		if json.Unmarshal(req.Hints.GeminiSafetySettings, &settings) == nil {
			greq.SafetySettings = settings
		}
	}

	return greq
}

// buildGenerationConfig builds Gemini generation config from canonical request.
func buildGenerationConfig(req *canonical.Request) *GenerationConfig {
	config := &GenerationConfig{}

	// Basic parameters
	config.Temperature = req.Temperature
	config.TopP = req.TopP

	if req.TopK != nil {
		topK := int64(*req.TopK)
		config.TopK = &topK
	}

	if req.MaxTokens != nil {
		config.MaxOutputTokens = req.MaxTokens
	}

	// Stop sequences
	if req.Stop != nil {
		if req.Stop.Single != "" {
			config.StopSequences = []string{req.Stop.Single}
		} else if len(req.Stop.Multiple) > 0 {
			config.StopSequences = req.Stop.Multiple
		}
	}

	// Response format
	if req.ResponseFormat != nil {
		switch req.ResponseFormat.Type {
		case "json_object":
			config.ResponseMimeType = "application/json"
		case "json_schema":
			config.ResponseMimeType = "application/json"
			if req.ResponseFormat.JsonSchema != nil {
				config.ResponseSchema = req.ResponseFormat.JsonSchema.Schema
			}
		}
	}

	// Thinking/Reasoning config
	if req.Reasoning != nil {
		config.ThinkingConfig = buildThinkingConfig(req.Reasoning)
	}

	// Media resolution
	if req.MediaResolution != nil {
		config.MediaResolution = "media_resolution_" + *req.MediaResolution
	}

	// Return nil if no config was set
	if config.Temperature == nil && config.TopP == nil && config.TopK == nil &&
		config.MaxOutputTokens == nil && len(config.StopSequences) == 0 &&
		config.ResponseMimeType == "" && config.ThinkingConfig == nil &&
		config.MediaResolution == "" {
		return nil
	}

	return config
}

// buildThinkingConfig builds Gemini thinking config from canonical reasoning config.
func buildThinkingConfig(rc *canonical.ReasoningConfig) *ThinkingConfig {
	if rc == nil {
		return nil
	}

	tc := &ThinkingConfig{}

	// Map effort level
	if rc.Effort != nil {
		level := mapCanonicalEffortToGeminiThinkingLevel(rc.Effort)
		if level != "" && level != "minimal" {
			tc.ThinkingLevel = level
		}
	}

	// Map budget tokens
	if rc.BudgetTokens != nil {
		tc.ThinkingBudget = rc.BudgetTokens
	}

	// Return nil if no config was set
	if tc.ThinkingLevel == "" && tc.ThinkingBudget == nil {
		return nil
	}

	return tc
}

// ParseResponse parses Gemini HTTP response to canonical Response.
func (a *ProviderAdapter) ParseResponse(ctx context.Context, resp *http.Response) (*canonical.Response, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		var errResp ErrorResponse
		if json.Unmarshal(body, &errResp) == nil && errResp.Error != nil && errResp.Error.Message != "" {
			return &canonical.Response{
				StatusCode: resp.StatusCode,
				Error: &canonical.Error{
					Code:       errResp.Error.Status,
					Message:    errResp.Error.Message,
					Type:       errResp.Error.Status,
					StatusCode: resp.StatusCode,
				},
			}, nil
		}
		return &canonical.Response{
			StatusCode: resp.StatusCode,
			Error: &canonical.Error{
				Message:    string(body),
				StatusCode: resp.StatusCode,
			},
		}, nil
	}

	var gresp GenerateContentResponse
	if err := json.Unmarshal(body, &gresp); err != nil {
		return nil, err
	}

	return convertGeminiResponseToCanonical(&gresp, resp.StatusCode), nil
}

// convertGeminiResponseToCanonical converts Gemini response to canonical format.
func convertGeminiResponseToCanonical(resp *GenerateContentResponse, statusCode int) *canonical.Response {
	creq := &canonical.Response{
		Model:      resp.ModelVersion,
		Object:     "chat.completion",
		StatusCode: statusCode,
	}

	// Convert candidates
	if len(resp.Candidates) > 0 {
		creq.Choices = make([]canonical.Choice, len(resp.Candidates))
		for i, candidate := range resp.Candidates {
			creq.Choices[i] = canonical.Choice{
				Index:        candidate.Index,
				FinishReason: mapGeminiFinishReasonToCanonical(candidate.FinishReason),
			}

			if candidate.Content != nil {
				creq.Choices[i].Message = convertGeminiContentToCanonical(*candidate.Content)
			}
		}
	}

	// Convert usage
	creq.Usage = convertGeminiUsageToCanonical(resp.UsageMetadata)

	return creq
}

// ParseStreamChunk parses Gemini SSE data to canonical Chunk.
func (a *ProviderAdapter) ParseStreamChunk(ctx context.Context, data []byte) (*canonical.Chunk, error) {
	// Gemini doesn't use a sentinel like [DONE], stream ends on connection close
	if len(data) == 0 {
		return &canonical.Chunk{}, nil
	}

	var gresp StreamChunk
	if err := json.Unmarshal(data, &gresp); err != nil {
		return nil, err
	}

	chunk := &canonical.Chunk{
		Model: gresp.ModelVersion,
	}

	// Convert usage if present
	if gresp.UsageMetadata != nil {
		chunk.Usage = convertGeminiUsageToCanonical(gresp.UsageMetadata)
	}

	// Convert candidates
	if len(gresp.Candidates) > 0 {
		chunk.Deltas = make([]canonical.ChoiceDelta, len(gresp.Candidates))
		for i, candidate := range gresp.Candidates {
			chunk.Deltas[i] = canonical.ChoiceDelta{
				Index:        candidate.Index,
				FinishReason: mapGeminiFinishReasonToCanonical(candidate.FinishReason),
			}

			if candidate.Content != nil {
				msg := convertGeminiContentToCanonical(*candidate.Content)

				// Handle empty-text chunks with thoughtSignature
				// We must preserve the signature even if text is empty
				if msg.ReasoningSignature != nil || msg.ThoughtSummary != nil {
					// Ensure the delta has the signature preserved
					msg.Reasoning = nil // Clear reasoning if empty text
				}

				chunk.Deltas[i].Delta = msg
			}
		}
	}

	return chunk, nil
}
