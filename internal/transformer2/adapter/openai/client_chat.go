package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/bestruirui/octopus/internal/transformer2/canonical"
)

// ChatClientAdapter implements ClientAdapter for OpenAI Chat Completions API.
type ChatClientAdapter struct {
	aggregator *streamAggregator
}

// streamAggregator accumulates stream chunks and aggregates them into a full response.
type streamAggregator struct {
	chunks []*canonical.Chunk
	full   *canonical.Response
}

func newStreamAggregator() *streamAggregator {
	return &streamAggregator{
		chunks: make([]*canonical.Chunk, 0),
	}
}

func (a *streamAggregator) addChunk(c *canonical.Chunk) {
	a.chunks = append(a.chunks, c)
}

func (a *streamAggregator) setFull(r *canonical.Response) {
	a.full = r
}

func (a *streamAggregator) aggregate() (*canonical.Response, error) {
	if a.full != nil {
		return a.full, nil
	}

	if len(a.chunks) == 0 {
		return &canonical.Response{}, nil
	}

	// Build response from chunks
	resp := &canonical.Response{
		ID:      a.chunks[0].ID,
		Model:   a.chunks[0].Model,
		Created: a.chunks[0].Created,
		Object:  "chat.completion",
	}

	// Aggregate choices by index
	choiceDeltas := make(map[int]*canonical.Message)
	var finishReason *string
	var lastUsage *canonical.Usage

	for _, chunk := range a.chunks {
		// Track usage (last wins)
		if chunk.Usage != nil {
			lastUsage = chunk.Usage
		}

		// Aggregate deltas
		for _, delta := range chunk.Deltas {
			if _, ok := choiceDeltas[delta.Index]; !ok {
				choiceDeltas[delta.Index] = &canonical.Message{
					Role:    delta.Delta.Role,
					Content: make([]canonical.ContentBlock, 0),
				}
			}

			msg := choiceDeltas[delta.Index]

			// Concatenate text content
			for _, cb := range delta.Delta.Content {
				if cb.Type == canonical.ContentText {
					// Find or append text block
					found := false
					for i := range msg.Content {
						if msg.Content[i].Type == canonical.ContentText {
							msg.Content[i].Text += cb.Text
							found = true
							break
						}
					}
					if !found {
						msg.Content = append(msg.Content, cb)
					}
				} else {
					msg.Content = append(msg.Content, cb)
				}
			}

			// Aggregate tool calls by index
			for _, tc := range delta.Delta.ToolCalls {
				// Find or create tool call
				found := false
				for i := range msg.ToolCalls {
					if msg.ToolCalls[i].Index == tc.Index {
						msg.ToolCalls[i].Arguments += tc.Arguments
						found = true
						break
					}
				}
				if !found {
					msg.ToolCalls = append(msg.ToolCalls, tc)
				}
			}

			// Track reasoning signature (last wins)
			if delta.Delta.ReasoningSignature != nil {
				msg.ReasoningSignature = delta.Delta.ReasoningSignature
			}

			// Track finish reason (last non-nil wins)
			if delta.FinishReason != nil {
				finishReason = delta.FinishReason
			}
		}
	}

	// Build choices
	for idx, msg := range choiceDeltas {
		resp.Choices = append(resp.Choices, canonical.Choice{
			Index:        idx,
			Message:      *msg,
			FinishReason: finishReason,
		})
	}

	// Set usage if present
	if lastUsage != nil {
		resp.Usage = lastUsage
	}

	return resp, nil
}

// NewChatClientAdapter creates a new ChatClientAdapter.
func NewChatClientAdapter() *ChatClientAdapter {
	return &ChatClientAdapter{
		aggregator: newStreamAggregator(),
	}
}

// ParseRequest converts OpenAI Chat request to canonical Request.
func (a *ChatClientAdapter) ParseRequest(ctx context.Context, body []byte, header http.Header) (*canonical.Request, error) {
	var req ChatCompletionRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}

	creq := &canonical.Request{
		Kind:             canonical.KindChat,
		Model:            req.Model,
		Temperature:      req.Temperature,
		TopP:             req.TopP,
		TopLogprobs:      req.TopLogprobs,
		MaxTokens:        req.MaxTokens,
		FrequencyPenalty: req.FrequencyPenalty,
		PresencePenalty:  req.PresencePenalty,
		Seed:             req.Seed,
		Logprobs:         req.Logprobs,
		Store:            req.Store,
		User:             req.User,
		ServiceTier:      req.ServiceTier,
		SourceFormat:     canonical.FormatOpenAIChat,
		Headers:          header,
		RawRequest:       body,
	}

	// Handle MaxCompletionTokens (o1+ models)
	if req.MaxCompletionTokens != nil {
		creq.MaxCompletionTokens = req.MaxCompletionTokens
	}

	// Handle stop sequences
	if req.Stop != nil {
		creq.Stop = &canonical.StopSequences{
			Single:   req.Stop.Single,
			Multiple: req.Stop.Multiple,
		}
	}

	// Handle stream
	if req.Stream != nil {
		creq.Stream = *req.Stream
	}
	if req.StreamOptions != nil {
		creq.StreamOptions = &canonical.StreamOptions{
			IncludeUsage: req.StreamOptions.IncludeUsage,
		}
	}

	// Handle modalities
	if len(req.Modalities) > 0 {
		creq.Modalities = req.Modalities
	}
	if req.Audio != nil {
		creq.AudioConfig = &canonical.AudioConfig{
			Format: req.Audio.Format,
			Voice:  req.Audio.Voice,
		}
	}

	// Handle reasoning_effort
	if req.ReasoningEffort != nil && *req.ReasoningEffort != "" {
		creq.Reasoning = &canonical.ReasoningConfig{
			Effort: req.ReasoningEffort,
		}
	}

	// Handle reasoning_budget (Qwen enable_thinking)
	if req.EnableThinking != nil {
		creq.EnableThinking = req.EnableThinking
	}

	// Handle response format
	if req.ResponseFormat != nil {
		creq.ResponseFormat = &canonical.ResponseFormat{
			Type: req.ResponseFormat.Type,
		}
		if req.ResponseFormat.JsonSchema != nil {
			creq.ResponseFormat.JsonSchema = &canonical.ResponseSchema{
				Name:        req.ResponseFormat.JsonSchema.Name,
				Description: req.ResponseFormat.JsonSchema.Description,
				Schema:      req.ResponseFormat.JsonSchema.Schema,
				Strict:      req.ResponseFormat.JsonSchema.Strict != nil && *req.ResponseFormat.JsonSchema.Strict,
			}
		}
	}

	// Convert messages
	creq.Messages = make([]canonical.Message, len(req.Messages))
	for i, msg := range req.Messages {
		creq.Messages[i] = convertOpenAIMessageToCanonical(msg)
	}

	// Convert tools
	if len(req.Tools) > 0 {
		creq.Tools = make([]canonical.Tool, len(req.Tools))
		for i, tool := range req.Tools {
			creq.Tools[i] = convertOpenAIToolToCanonical(tool)
		}
	}

	// Convert tool_choice
	if req.ToolChoice != nil {
		creq.ToolChoice = convertOpenAIToolChoiceToCanonical(req.ToolChoice)
	}

	// Handle parallel_tool_calls
	if req.ParallelToolCalls != nil {
		disabled := !*req.ParallelToolCalls
		if creq.ToolChoice == nil {
			creq.ToolChoice = &canonical.ToolChoice{
				Mode: "auto",
			}
		}
		creq.ToolChoice.DisableParallelToolUse = &disabled
	}

	return creq, nil
}

// FormatResponse converts canonical Response to OpenAI format.
func (a *ChatClientAdapter) FormatResponse(ctx context.Context, resp *canonical.Response) ([]byte, error) {
	oresp := convertCanonicalToOpenAIResponse(resp)
	return json.Marshal(oresp)
}

// FormatStreamChunk converts canonical Chunk to OpenAI SSE format.
func (a *ChatClientAdapter) FormatStreamChunk(ctx context.Context, chunk *canonical.Chunk) ([]byte, error) {
	if chunk.Done {
		return []byte("data: [DONE]\n\n"), nil
	}

	oresp := convertCanonicalToOpenAIChunk(chunk)
	data, err := json.Marshal(oresp)
	if err != nil {
		return nil, err
	}
	return append([]byte("data: "), append(data, []byte("\n\n")...)...), nil
}

// AggregateStream aggregates all collected chunks into a full response.
func (a *ChatClientAdapter) AggregateStream(ctx context.Context) (*canonical.Response, error) {
	return a.aggregator.aggregate()
}

// FormatError converts canonical Error to OpenAI error format.
func (a *ChatClientAdapter) FormatError(ctx context.Context, err *canonical.Error) ([]byte, error) {
	oerr := ErrorResponse{
		Error: ErrorDetail{
			Code:    err.Code,
			Message: err.Message,
			Type:    err.Type,
		},
	}
	return json.Marshal(oerr)
}

// convertOpenAIMessageToCanonical converts OpenAI message to canonical format.
func convertOpenAIMessageToCanonical(msg ChatMessage) canonical.Message {
	cmsg := canonical.Message{
		Role: canonical.Role(msg.Role),
	}

	// Handle content (string or array)
	if msg.Content.Text != nil {
		cmsg.Content = []canonical.ContentBlock{
			{Type: canonical.ContentText, Text: *msg.Content.Text},
		}
	} else if len(msg.Content.Parts) > 0 {
		cmsg.Content = make([]canonical.ContentBlock, len(msg.Content.Parts))
		for i, part := range msg.Content.Parts {
			cmsg.Content[i] = convertContentPartToCanonical(part)
		}
	}

	// Handle name
	if msg.Name != nil {
		cmsg.Name = msg.Name
	}

	// Handle tool_calls
	if len(msg.ToolCalls) > 0 {
		cmsg.ToolCalls = make([]canonical.ToolCall, len(msg.ToolCalls))
		for i, tc := range msg.ToolCalls {
			cmsg.ToolCalls[i] = canonical.ToolCall{
				ID:        tc.ID,
				Type:      tc.Type,
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
				Index:     tc.Index,
			}
		}
	}

	// Handle tool call result
	if msg.ToolCallID != nil {
		cmsg.ToolCallID = msg.ToolCallID
	}

	// Handle refusal
	if msg.Refusal != "" {
		cmsg.Refusal = msg.Refusal
	}

	// Handle reasoning_content
	if msg.ReasoningContent != nil {
		cmsg.Reasoning = msg.ReasoningContent
	}

	return cmsg
}

// convertContentPartToCanonical converts OpenAI content part to canonical format.
func convertContentPartToCanonical(part ContentPart) canonical.ContentBlock {
	cb := canonical.ContentBlock{Type: canonical.ContentType(part.Type)}

	switch part.Type {
	case "text":
		if part.Text != nil {
			cb.Text = *part.Text
		}
	case "image_url":
		if part.ImageURL != nil {
			cb.Type = canonical.ContentImage
			cb.Media = &canonical.MediaContent{
				URL:    part.ImageURL.URL,
				Detail: part.ImageURL.Detail,
			}
			// Parse data URL if present
			if strings.HasPrefix(part.ImageURL.URL, "data:") {
				// Extract MIME type and base64 data from data URL
				// Format: data:image/jpeg;base64,/9j/4AAQ==
				parts := strings.SplitN(part.ImageURL.URL, ",", 2)
				if len(parts) == 2 {
					mimePart := parts[0]
					cb.Media.Base64 = parts[1]
					// Extract MIME type from "data:image/jpeg;base64"
					if strings.HasPrefix(mimePart, "data:") && strings.HasSuffix(mimePart, ";base64") {
						cb.Media.MimeType = strings.TrimSuffix(strings.TrimPrefix(mimePart, "data:"), ";base64")
					}
				}
			}
		}
	case "input_audio":
		if part.Audio != nil {
			cb.Type = canonical.ContentAudio
			cb.Media = &canonical.MediaContent{
				AudioFormat: part.Audio.Format,
				AudioData:   part.Audio.Data,
			}
		}
	case "file":
		if part.File != nil {
			cb.Type = canonical.ContentFile
			cb.Media = &canonical.MediaContent{
				FileName: part.File.Filename,
				FileData: part.File.FileData,
			}
		}
	}

	return cb
}

// convertOpenAIToolToCanonical converts OpenAI tool to canonical format.
func convertOpenAIToolToCanonical(tool Tool) canonical.Tool {
	ctool := canonical.Tool{
		Type: tool.Type,
	}

	if tool.Function != nil {
		ctool.Name = tool.Function.Name
		ctool.Description = tool.Function.Description
		ctool.Parameters = tool.Function.Parameters
		ctool.Strict = tool.Function.Strict
	}

	if tool.ImageGeneration != nil {
		ctool.ImageGeneration = &canonical.ImageGenerationConfig{
			Background:        tool.ImageGeneration.Background,
			OutputFormat:      tool.ImageGeneration.OutputFormat,
			Quality:           tool.ImageGeneration.Quality,
			Size:              tool.ImageGeneration.Size,
			OutputCompression: tool.ImageGeneration.OutputCompression,
		}
	}

	return ctool
}

// convertOpenAIToolChoiceToCanonical converts OpenAI tool_choice to canonical format.
func convertOpenAIToolChoiceToCanonical(tc *ToolChoice) *canonical.ToolChoice {
	if tc == nil {
		return nil
	}

	return &canonical.ToolChoice{
		Mode:     tc.Mode,
		Function: tc.Function,
	}
}

// convertCanonicalToOpenAIResponse converts canonical Response to OpenAI format.
func convertCanonicalToOpenAIResponse(resp *canonical.Response) *ChatCompletionResponse {
	oresp := &ChatCompletionResponse{
		ID:                resp.ID,
		Object:            resp.Object,
		Created:           resp.Created,
		Model:             resp.Model,
		SystemFingerprint: resp.SystemFingerprint,
		ServiceTier:       resp.ServiceTier,
	}

	if resp.Usage != nil {
		oresp.Usage = &Usage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		}
		if resp.Usage.PromptTokensDetails != nil {
			oresp.Usage.PromptTokensDetails = &PromptTokensDetails{
				CachedTokens: resp.Usage.PromptTokensDetails.CachedTokens,
				AudioTokens:  resp.Usage.PromptTokensDetails.AudioTokens,
			}
		}
		if resp.Usage.CompletionTokensDetails != nil {
			oresp.Usage.CompletionTokensDetails = &CompletionTokensDetails{
				ReasoningTokens: resp.Usage.CompletionTokensDetails.ReasoningTokens,
				AudioTokens:     resp.Usage.CompletionTokensDetails.AudioTokens,
			}
		}
	}

	if len(resp.Choices) > 0 {
		oresp.Choices = make([]ChatChoice, len(resp.Choices))
		for i, choice := range resp.Choices {
			oresp.Choices[i] = ChatChoice{
				Index:        choice.Index,
				FinishReason: choice.FinishReason,
			}
			if choice.Message.Role != "" {
				msg := convertCanonicalMessageToOpenAI(choice.Message)
				oresp.Choices[i].Message = &msg
			}
		}
	}

	return oresp
}

// convertCanonicalMessageToOpenAI converts canonical Message to OpenAI format.
func convertCanonicalMessageToOpenAI(msg canonical.Message) ChatMessage {
	omsg := ChatMessage{
		Role: string(msg.Role),
	}

	// Handle content
	if len(msg.Content) > 0 {
		// Single text block -> string content
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

	// Handle tool_call_id
	if msg.ToolCallID != nil {
		omsg.ToolCallID = msg.ToolCallID
	}

	// Handle name
	if msg.Name != nil {
		omsg.Name = msg.Name
	}

	return omsg
}

// convertCanonicalContentBlockToOpenAI converts canonical ContentBlock to OpenAI format.
func convertCanonicalContentBlockToOpenAI(cb canonical.ContentBlock) ContentPart {
	part := ContentPart{Type: string(cb.Type)}

	switch cb.Type {
	case canonical.ContentText:
		part.Text = &cb.Text
	case canonical.ContentImage:
		part.Type = "image_url"
		if cb.Media != nil {
			part.ImageURL = &ImageURL{
				URL:    cb.Media.URL,
				Detail: cb.Media.Detail,
			}
		}
	case canonical.ContentAudio:
		part.Type = "input_audio"
		if cb.Media != nil {
			part.Audio = &Audio{
				Format: cb.Media.AudioFormat,
				Data:   cb.Media.AudioData,
			}
		}
	}

	return part
}

// convertCanonicalToOpenAIChunk converts canonical Chunk to OpenAI stream chunk.
func convertCanonicalToOpenAIChunk(chunk *canonical.Chunk) *ChatCompletionResponse {
	oresp := &ChatCompletionResponse{
		ID:      chunk.ID,
		Model:   chunk.Model,
		Created: chunk.Created,
		Object:  "chat.completion.chunk",
	}

	if chunk.Usage != nil {
		oresp.Usage = &Usage{
			PromptTokens:     chunk.Usage.PromptTokens,
			CompletionTokens: chunk.Usage.CompletionTokens,
			TotalTokens:      chunk.Usage.TotalTokens,
		}
	}

	if len(chunk.Deltas) > 0 {
		oresp.Choices = make([]ChatChoice, len(chunk.Deltas))
		for i, delta := range chunk.Deltas {
			oresp.Choices[i] = ChatChoice{
				Index:        delta.Index,
				FinishReason: delta.FinishReason,
			}
			if delta.Delta.Role != "" || len(delta.Delta.Content) > 0 || len(delta.Delta.ToolCalls) > 0 {
				msg := convertCanonicalMessageToOpenAI(delta.Delta)
				oresp.Choices[i].Delta = &msg
			}
		}
	}

	return oresp
}
