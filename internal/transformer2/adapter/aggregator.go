package adapter

import (
	"github.com/bestruirui/octopus/internal/transformer2/canonical"
)

// StreamAggregator accumulates stream chunks and aggregates them into a full response.
type StreamAggregator struct {
	chunks []*canonical.Chunk
	full   *canonical.Response
}

// NewStreamAggregator creates a new StreamAggregator.
func NewStreamAggregator() *StreamAggregator {
	return &StreamAggregator{
		chunks: make([]*canonical.Chunk, 0),
	}
}

// AddChunk adds a chunk to the aggregator.
func (a *StreamAggregator) AddChunk(c *canonical.Chunk) {
	a.chunks = append(a.chunks, c)
}

// SetFull sets the full response (used for final chunk with usage).
func (a *StreamAggregator) SetFull(r *canonical.Response) {
	a.full = r
}

// Aggregate merges all collected chunks into a full response.
func (a *StreamAggregator) Aggregate() (*canonical.Response, error) {
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
