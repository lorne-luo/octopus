package volcengine

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/bestruirui/octopus/internal/transformer2/adapter/openai"
	"github.com/bestruirui/octopus/internal/transformer2/canonical"
)

// supportedReasoningModels contains models that support reasoning effort.
var supportedReasoningModels = map[string]bool{
	"doubao-seed-1-8-251228":      true,
	"doubao-seed-1-6-lite-251015": true,
	"doubao-seed-1-6-251015":      true,
}

// ProviderAdapter wraps OpenAI Responses adapter with Volcengine-specific handling.
type ProviderAdapter struct {
	inner *openai.ResponsesProviderAdapter
}

// NewProviderAdapter creates a new ProviderAdapter.
func NewProviderAdapter() *ProviderAdapter {
	return &ProviderAdapter{
		inner: openai.NewResponsesProviderAdapter(),
	}
}

// BuildRequest builds an HTTP request for Volcengine from canonical Request.
func (a *ProviderAdapter) BuildRequest(ctx context.Context, req *canonical.Request, baseURL, key string) (*http.Request, error) {
	// Create a modified copy of the request for Volcengine-specific handling
	modifiedReq := a.modifyRequestForVolcengine(req)

	// Use the inner OpenAI Responses adapter to build the request
	httpReq, err := a.inner.BuildRequest(ctx, modifiedReq, baseURL, key)
	if err != nil {
		return nil, err
	}

	// Read the body
	body, err := io.ReadAll(httpReq.Body)
	if err != nil {
		return nil, err
	}
	httpReq.Body.Close()

	// Parse and modify the JSON body for Volcengine-specific fields
	var bodyMap map[string]interface{}
	if err := json.Unmarshal(body, &bodyMap); err != nil {
		return nil, err
	}

	// Inject thinking field if reasoning effort is specified for supported models
	if req.Reasoning != nil && req.Reasoning.Effort != nil && *req.Reasoning.Effort != "" {
		if supportsReasoning(req.Model) {
			thinkingType := mapReasoningEffortToThinking(*req.Reasoning.Effort)
			bodyMap["thinking"] = map[string]interface{}{
				"type": string(thinkingType),
			}
			bodyMap["reasoning"] = map[string]interface{}{
				"effort": *req.Reasoning.Effort,
			}
		} else {
			// Remove both thinking and reasoning for unsupported models
			delete(bodyMap, "reasoning")
			delete(bodyMap, "thinking")
		}
	}

	// Strip metadata (Volcengine does not support it)
	delete(bodyMap, "metadata")

	// Handle partial assistant message for multi-turn conversations
	if len(req.Messages) > 0 {
		lastMsg := req.Messages[len(req.Messages)-1]
		if lastMsg.Role == canonical.RoleAssistant {
			// Add partial: true to the last input item if it's an assistant message
			if input, ok := bodyMap["input"].([]interface{}); ok && len(input) > 0 {
				lastInput := input[len(input)-1]
				if lastInputMap, ok := lastInput.(map[string]interface{}); ok {
					if role, ok := lastInputMap["role"].(string); ok && role == "assistant" {
						lastInputMap["partial"] = true
					}
				}
			}
		}
	}

	// Re-encode the body
	modifiedBody, err := json.Marshal(bodyMap)
	if err != nil {
		return nil, err
	}

	// Update the request
	httpReq.Body = io.NopCloser(bytes.NewReader(modifiedBody))
	httpReq.ContentLength = int64(len(modifiedBody))

	return httpReq, nil
}

// ParseResponse parses Volcengine HTTP response to canonical Response.
func (a *ProviderAdapter) ParseResponse(ctx context.Context, resp *http.Response) (*canonical.Response, error) {
	return a.inner.ParseResponse(ctx, resp)
}

// ParseStreamChunk parses SSE data to canonical Chunk.
func (a *ProviderAdapter) ParseStreamChunk(ctx context.Context, data []byte) (*canonical.Chunk, error) {
	return a.inner.ParseStreamChunk(ctx, data)
}

// modifyRequestForVolcengine applies Volcengine-specific modifications to the request.
func (a *ProviderAdapter) modifyRequestForVolcengine(req *canonical.Request) *canonical.Request {
	// Create a shallow copy of the request
	modified := *req

	// Strip metadata (Volcengine does not support it)
	modified.Metadata = nil

	return &modified
}

// mapReasoningEffortToThinking maps OpenAI reasoning effort to Volcengine thinking type.
func mapReasoningEffortToThinking(effort string) ThinkingType {
	// Map minimal to disabled, all others to enabled
	switch strings.ToLower(effort) {
	case "minimal":
		return ThinkingTypeDisabled
	default:
		return ThinkingTypeEnabled
	}
}

// supportsReasoning checks if the model supports reasoning/thinking.
func supportsReasoning(model string) bool {
	// Check for doubao-seed-* pattern
	if strings.HasPrefix(model, "doubao-seed-") {
		return supportedReasoningModels[model]
	}
	return false
}