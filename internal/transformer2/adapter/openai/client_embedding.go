package openai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/bestruirui/octopus/internal/transformer2/canonical"
)

// EmbeddingClientAdapter implements ClientAdapter for OpenAI Embeddings API.
type EmbeddingClientAdapter struct{}

// NewEmbeddingClientAdapter creates a new EmbeddingClientAdapter.
func NewEmbeddingClientAdapter() *EmbeddingClientAdapter {
	return &EmbeddingClientAdapter{}
}

// ParseRequest converts OpenAI Embeddings request to canonical Request.
func (a *EmbeddingClientAdapter) ParseRequest(ctx context.Context, body []byte, header http.Header) (*canonical.Request, error) {
	var req EmbeddingRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}

	creq := &canonical.Request{
		Kind:         canonical.KindEmbedding,
		Model:        req.Model,
		User:         req.User,
		SourceFormat: canonical.FormatOpenAIEmbed,
		Headers:      header,
		RawRequest:   body,
	}

	// Handle input
	if req.Input.Single != "" {
		creq.EmbeddingInput = &canonical.EmbeddingInput{
			Text: req.Input.Single,
		}
	} else if len(req.Input.Multiple) > 0 {
		creq.EmbeddingInput = &canonical.EmbeddingInput{
			Texts: req.Input.Multiple,
		}
	} else if len(req.Input.Tokens) > 0 {
		creq.EmbeddingInput = &canonical.EmbeddingInput{
			Tokens: req.Input.Tokens,
		}
	}

	// Handle dimensions
	if req.Dimensions != nil {
		creq.EmbeddingDimensions = req.Dimensions
	}

	// Handle encoding format
	if req.EncodingFormat != nil {
		creq.EmbeddingEncodingFormat = req.EncodingFormat
	}

	return creq, nil
}

// FormatResponse converts canonical Response to OpenAI Embeddings format.
func (a *EmbeddingClientAdapter) FormatResponse(ctx context.Context, resp *canonical.Response) ([]byte, error) {
	oresp := convertCanonicalToEmbeddingResponse(resp)
	return json.Marshal(oresp)
}

// FormatStreamChunk converts canonical Chunk to OpenAI SSE format.
// Embeddings API does not support streaming.
func (a *EmbeddingClientAdapter) FormatStreamChunk(ctx context.Context, chunk *canonical.Chunk) ([]byte, error) {
	return nil, errors.New("embeddings API does not support streaming")
}

// AggregateStream aggregates all collected chunks into a full response.
// Embeddings API does not support streaming.
func (a *EmbeddingClientAdapter) AggregateStream(ctx context.Context) (*canonical.Response, error) {
	return nil, errors.New("embeddings API does not support streaming")
}

// FormatError converts canonical Error to OpenAI error format.
func (a *EmbeddingClientAdapter) FormatError(ctx context.Context, err *canonical.Error) ([]byte, error) {
	oerr := ErrorResponse{
		Error: ErrorDetail{
			Code:    err.Code,
			Message: err.Message,
			Type:    err.Type,
		},
	}
	return json.Marshal(oerr)
}

// convertCanonicalToEmbeddingResponse converts canonical Response to EmbeddingResponse.
func convertCanonicalToEmbeddingResponse(resp *canonical.Response) *EmbeddingResponse {
	oresp := &EmbeddingResponse{
		Object: resp.Object,
		Model:  resp.Model,
	}

	// Convert embeddings
	if len(resp.Embeddings) > 0 {
		oresp.Data = make([]EmbeddingData, len(resp.Embeddings))
		for i, emb := range resp.Embeddings {
			oresp.Data[i] = EmbeddingData{
				Object: emb.Object,
				Index:  emb.Index,
				Embedding: EmbeddingVector{
					Floats: emb.Embedding,
				},
			}
		}
	}

	// Convert usage
	if resp.Usage != nil {
		oresp.Usage = &EmbeddingUsage{
			PromptTokens: resp.Usage.PromptTokens,
			TotalTokens:  resp.Usage.TotalTokens,
		}
	}

	return oresp
}