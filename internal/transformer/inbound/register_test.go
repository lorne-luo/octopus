package inbound

import (
	"testing"

	"github.com/bestruirui/octopus/internal/transformer/outbound"
)

func TestMatchesOutbound(t *testing.T) {
	tests := []struct {
		name     string
		inbound  InboundType
		outbound outbound.OutboundType
		want     bool
	}{
		{
			name:     "anthropic matches",
			inbound:  InboundTypeAnthropic,
			outbound: outbound.OutboundTypeAnthropic,
			want:     true,
		},
		{
			name:     "openai chat matches",
			inbound:  InboundTypeOpenAIChat,
			outbound: outbound.OutboundTypeOpenAIChat,
			want:     true,
		},
		{
			name:     "openai response matches",
			inbound:  InboundTypeOpenAIResponse,
			outbound: outbound.OutboundTypeOpenAIResponse,
			want:     true,
		},
		{
			name:     "openai embedding matches",
			inbound:  InboundTypeOpenAIEmbedding,
			outbound: outbound.OutboundTypeOpenAIEmbedding,
			want:     true,
		},
		{
			name:     "anthropic in / openai chat out - no match",
			inbound:  InboundTypeAnthropic,
			outbound: outbound.OutboundTypeOpenAIChat,
			want:     false,
		},
		{
			name:     "openai chat in / anthropic out - no match",
			inbound:  InboundTypeOpenAIChat,
			outbound: outbound.OutboundTypeAnthropic,
			want:     false,
		},
		{
			name:     "gemini has no mapping",
			inbound:  InboundTypeGemini,
			outbound: outbound.OutboundTypeGemini,
			want:     false,
		},
		{
			name:     "openai chat in / openai response out - no match",
			inbound:  InboundTypeOpenAIChat,
			outbound: outbound.OutboundTypeOpenAIResponse,
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MatchesOutbound(tt.inbound, tt.outbound)
			if got != tt.want {
				t.Errorf("MatchesOutbound(%d, %d) = %v, want %v", tt.inbound, tt.outbound, got, tt.want)
			}
		})
	}
}
