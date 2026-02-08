package relay

import (
	"testing"

	"github.com/bestruirui/octopus/internal/transformer/model"
	"github.com/stretchr/testify/assert"
)

func TestRelayMetrics_SetInternalResponse_NoUsage(t *testing.T) {
	// Setup
	req := &model.InternalLLMRequest{
		Model: "gpt-3.5-turbo",
		Messages: []model.Message{
			{
				Role: "user",
				Content: model.MessageContent{
					Content: new(string),
				},
			},
		},
	}
	*req.Messages[0].Content.Content = "Hello, world!"

	resp := &model.InternalLLMResponse{
		Choices: []model.Choice{
			{
				Message: &model.Message{
					Content: model.MessageContent{
						Content: new(string),
					},
				},
			},
		},
	}
	*resp.Choices[0].Message.Content.Content = "Hello back!"

	m := NewRelayMetrics("gpt-3.5-turbo")
	m.SetInternalRequest(req)
	m.SetChannel(1, 1, "test-channel", "gpt-3.5-turbo")

	// Execute
	m.SetInternalResponse(resp)

	// Verify
	assert.Greater(t, m.Stats.InputToken, int64(0))
	assert.Greater(t, m.Stats.OutputToken, int64(0))
	t.Logf("Input Tokens: %d", m.Stats.InputToken)
	t.Logf("Output Tokens: %d", m.Stats.OutputToken)
}

func TestRelayMetrics_SetInternalResponse_WithUsage(t *testing.T) {
	// Setup
	req := &model.InternalLLMRequest{
		Model: "gpt-3.5-turbo",
	}
	resp := &model.InternalLLMResponse{
		Usage: &model.Usage{
			PromptTokens:     10,
			CompletionTokens: 20,
		},
	}

	m := NewRelayMetrics("gpt-3.5-turbo")
	m.SetInternalRequest(req)
	m.SetChannel(1, 1, "test-channel", "gpt-3.5-turbo")

	// Execute
	m.SetInternalResponse(resp)

	// Verify
	assert.Equal(t, int64(10), m.Stats.InputToken)
	assert.Equal(t, int64(20), m.Stats.OutputToken)
}
