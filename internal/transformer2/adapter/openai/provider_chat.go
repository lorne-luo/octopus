package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/bestruirui/octopus/internal/transformer2/canonical"
)

// ChatProviderAdapter implements ProviderAdapter for OpenAI Chat Completions API.
type ChatProviderAdapter struct{}

// NewChatProviderAdapter creates a new ChatProviderAdapter.
func NewChatProviderAdapter() *ChatProviderAdapter {
	return &ChatProviderAdapter{}
}

// BuildRequest builds an HTTP request for OpenAI from canonical Request.
func (a *ChatProviderAdapter) BuildRequest(ctx context.Context, req *canonical.Request, baseURL, key string) (*http.Request, error) {
	oreq := convertCanonicalToOpenAIRequest(req)

	body, err := json.Marshal(oreq)
	if err != nil {
		return nil, err
	}

	url := strings.TrimSuffix(baseURL, "/") + "/v1/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+key)

	return httpReq, nil
}

// ParseResponse parses OpenAI HTTP response to canonical Response.
func (a *ChatProviderAdapter) ParseResponse(ctx context.Context, resp *http.Response) (*canonical.Response, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		var errResp ErrorResponse
		if json.Unmarshal(body, &errResp) == nil && errResp.Error.Message != "" {
			return &canonical.Response{
				StatusCode: resp.StatusCode,
				Error: &canonical.Error{
					Code:       errResp.Error.Code,
					Message:    errResp.Error.Message,
					Type:       errResp.Error.Type,
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

	var oresp ChatCompletionResponse
	if err := json.Unmarshal(body, &oresp); err != nil {
		return nil, err
	}

	return convertOpenAIResponseToCanonical(&oresp, resp.StatusCode), nil
}

// ParseStreamChunk parses SSE data to canonical Chunk.
func (a *ChatProviderAdapter) ParseStreamChunk(ctx context.Context, data []byte) (*canonical.Chunk, error) {
	dataStr := string(data)

	// Handle [DONE]
	if dataStr == "[DONE]" {
		return &canonical.Chunk{Done: true}, nil
	}

	var oresp ChatCompletionResponse
	if err := json.Unmarshal(data, &oresp); err != nil {
		return nil, err
	}

	chunk := &canonical.Chunk{
		ID:      oresp.ID,
		Model:   oresp.Model,
		Created: oresp.Created,
	}

	// Handle usage
	if oresp.Usage != nil {
		chunk.Usage = convertOpenAIUsageToCanonical(oresp.Usage)
	}

	// Handle choices
	if len(oresp.Choices) > 0 {
		chunk.Deltas = make([]canonical.ChoiceDelta, len(oresp.Choices))
		for i, choice := range oresp.Choices {
			chunk.Deltas[i] = canonical.ChoiceDelta{
				Index:        choice.Index,
				FinishReason: choice.FinishReason,
			}
			if choice.Delta != nil {
				chunk.Deltas[i].Delta = convertOpenAIMessageToCanonical(*choice.Delta)
			}
		}
	}

	return chunk, nil
}

// convertCanonicalToOpenAIRequest converts canonical Request to OpenAI format.
func convertCanonicalToOpenAIRequest(req *canonical.Request) *ChatCompletionRequest {
	oreq := &ChatCompletionRequest{
		Model:               req.Model,
		Temperature:         req.Temperature,
		TopP:                req.TopP,
		TopLogprobs:         req.TopLogprobs,
		MaxTokens:           req.MaxTokens,
		MaxCompletionTokens: req.MaxCompletionTokens,
		FrequencyPenalty:    req.FrequencyPenalty,
		PresencePenalty:     req.PresencePenalty,
		Seed:                req.Seed,
		Logprobs:            req.Logprobs,
		Store:               req.Store,
		User:                req.User,
		ServiceTier:         req.ServiceTier,
	}

	// Handle stream
	if req.Stream {
		oreq.Stream = &req.Stream
	}
	if req.StreamOptions != nil {
		oreq.StreamOptions = &StreamOptions{
			IncludeUsage: req.StreamOptions.IncludeUsage,
		}
	}

	// Handle stop sequences
	if req.Stop != nil {
		oreq.Stop = &StopSequences{
			Single:   req.Stop.Single,
			Multiple: req.Stop.Multiple,
		}
	}

	// Handle modalities
	if len(req.Modalities) > 0 {
		oreq.Modalities = req.Modalities
	}
	if req.AudioConfig != nil {
		oreq.Audio = &AudioConfig{
			Format: req.AudioConfig.Format,
			Voice:  req.AudioConfig.Voice,
		}
	}

	// Handle response format
	if req.ResponseFormat != nil {
		oreq.ResponseFormat = &ResponseFormat{
			Type: req.ResponseFormat.Type,
		}
		if req.ResponseFormat.JsonSchema != nil {
			strict := req.ResponseFormat.JsonSchema.Strict
			oreq.ResponseFormat.JsonSchema = &JsonSchemaFormat{
				Name:        req.ResponseFormat.JsonSchema.Name,
				Description: req.ResponseFormat.JsonSchema.Description,
				Schema:      req.ResponseFormat.JsonSchema.Schema,
				Strict:      &strict,
			}
		}
	}

	// Handle reasoning
	if req.Reasoning != nil && req.Reasoning.Effort != nil {
		oreq.ReasoningEffort = req.Reasoning.Effort
	}

	// Handle thinking (Qwen)
	if req.EnableThinking != nil {
		oreq.EnableThinking = req.EnableThinking
	}

	// Convert messages
	oreq.Messages = make([]ChatMessage, len(req.Messages))
	for i, msg := range req.Messages {
		oreq.Messages[i] = convertCanonicalMessageToOpenAIForProvider(msg)
	}

	// Convert tools
	if len(req.Tools) > 0 {
		oreq.Tools = make([]Tool, len(req.Tools))
		for i, tool := range req.Tools {
			oreq.Tools[i] = convertCanonicalToolToOpenAI(tool)
		}
	}

	// Convert tool_choice
	if req.ToolChoice != nil {
		oreq.ToolChoice = convertCanonicalToolChoiceToOpenAI(req.ToolChoice)
		if req.ToolChoice.DisableParallelToolUse != nil {
			parallel := !*req.ToolChoice.DisableParallelToolUse
			oreq.ParallelToolCalls = &parallel
		}
	}

	// Handle logit_bias
	if len(req.LogitBias) > 0 {
		oreq.LogitBias = req.LogitBias
	}

	return oreq
}

// convertCanonicalMessageToOpenAIForProvider converts canonical Message to OpenAI format (for provider).
func convertCanonicalMessageToOpenAIForProvider(msg canonical.Message) ChatMessage {
	omsg := ChatMessage{
		Role: string(msg.Role),
	}

	// Handle content
	if len(msg.Content) > 0 {
		if len(msg.Content) == 1 && msg.Content[0].Type == canonical.ContentText {
			omsg.Content.Text = &msg.Content[0].Text
		} else {
			omsg.Content.Parts = make([]ContentPart, len(msg.Content))
			for i, cb := range msg.Content {
				omsg.Content.Parts[i] = convertCanonicalContentBlockToOpenAI(cb)
			}
		}
	}

	// Handle tool_calls
	if len(msg.ToolCalls) > 0 {
		omsg.ToolCalls = make([]ToolCall, len(msg.ToolCalls))
		for i, tc := range msg.ToolCalls {
			omsg.ToolCalls[i] = ToolCall{
				ID:    tc.ID,
				Type:  tc.Type,
				Index: tc.Index,
				Function: FunctionCall{
					Name:      tc.Name,
					Arguments: tc.Arguments,
				},
			}
		}
	}

	// Handle tool call result
	if msg.ToolCallID != nil {
		omsg.ToolCallID = msg.ToolCallID
		// Content for tool result
		if len(msg.Content) > 0 && msg.Content[0].Type == canonical.ContentText {
			omsg.Content.Text = &msg.Content[0].Text
		}
	}

	// Handle name
	if msg.Name != nil {
		omsg.Name = msg.Name
	}

	// Handle reasoning
	if msg.Reasoning != nil {
		omsg.ReasoningContent = msg.Reasoning
	}

	return omsg
}

// convertCanonicalToolToOpenAI converts canonical Tool to OpenAI format.
func convertCanonicalToolToOpenAI(tool canonical.Tool) Tool {
	otool := Tool{
		Type: tool.Type,
	}

	if tool.Type == "function" && tool.Name != "" {
		otool.Function = &FunctionDef{
			Name:        tool.Name,
			Description: tool.Description,
			Parameters:  tool.Parameters,
			Strict:      tool.Strict,
		}
	}

	if tool.ImageGeneration != nil {
		otool.ImageGeneration = &ImageGeneration{
			Background:        tool.ImageGeneration.Background,
			OutputFormat:      tool.ImageGeneration.OutputFormat,
			Quality:           tool.ImageGeneration.Quality,
			Size:              tool.ImageGeneration.Size,
			OutputCompression: tool.ImageGeneration.OutputCompression,
		}
	}

	return otool
}

// convertCanonicalToolChoiceToOpenAI converts canonical ToolChoice to OpenAI format.
func convertCanonicalToolChoiceToOpenAI(tc *canonical.ToolChoice) *ToolChoice {
	if tc == nil {
		return nil
	}

	return &ToolChoice{
		Mode:     tc.Mode,
		Function: tc.Function,
	}
}

// convertOpenAIResponseToCanonical converts OpenAI response to canonical format.
func convertOpenAIResponseToCanonical(oresp *ChatCompletionResponse, statusCode int) *canonical.Response {
	creq := &canonical.Response{
		ID:                oresp.ID,
		Model:             oresp.Model,
		Created:           oresp.Created,
		Object:            oresp.Object,
		SystemFingerprint: oresp.SystemFingerprint,
		ServiceTier:       oresp.ServiceTier,
		StatusCode:        statusCode,
	}

	if oresp.Usage != nil {
		creq.Usage = convertOpenAIUsageToCanonical(oresp.Usage)
	}

	if len(oresp.Choices) > 0 {
		creq.Choices = make([]canonical.Choice, len(oresp.Choices))
		for i, choice := range oresp.Choices {
			creq.Choices[i] = canonical.Choice{
				Index:        choice.Index,
				FinishReason: choice.FinishReason,
			}
			if choice.Message != nil {
				creq.Choices[i].Message = convertOpenAIMessageToCanonical(*choice.Message)
			}
		}
	}

	return creq
}

// convertOpenAIUsageToCanonical converts OpenAI usage to canonical format.
func convertOpenAIUsageToCanonical(usage *Usage) *canonical.Usage {
	if usage == nil {
		return nil
	}

	cusage := &canonical.Usage{
		PromptTokens:     usage.PromptTokens,
		CompletionTokens: usage.CompletionTokens,
		TotalTokens:      usage.TotalTokens,
	}

	if usage.PromptTokensDetails != nil {
		cusage.PromptTokensDetails = &canonical.PromptTokensDetails{
			CachedTokens: usage.PromptTokensDetails.CachedTokens,
			AudioTokens:  usage.PromptTokensDetails.AudioTokens,
		}
	}

	if usage.CompletionTokensDetails != nil {
		cusage.CompletionTokensDetails = &canonical.CompletionTokensDetails{
			ReasoningTokens: usage.CompletionTokensDetails.ReasoningTokens,
			AudioTokens:     usage.CompletionTokensDetails.AudioTokens,
		}
	}

	return cusage
}
