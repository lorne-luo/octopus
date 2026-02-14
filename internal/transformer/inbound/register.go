package inbound

import (
	"github.com/bestruirui/octopus/internal/transformer/inbound/anthropic"
	"github.com/bestruirui/octopus/internal/transformer/inbound/openai"
	"github.com/bestruirui/octopus/internal/transformer/model"
	"github.com/bestruirui/octopus/internal/transformer/outbound"
)

type InboundType int

const (
	InboundTypeOpenAIChat InboundType = iota
	InboundTypeOpenAIResponse
	InboundTypeAnthropic
	InboundTypeGemini
	InboundTypeOpenAIEmbedding

	// Compatibility alias for legacy naming
	InboundTypeOpenAI = InboundTypeOpenAIChat
)

var inboundFactories = map[InboundType]func() model.Inbound{
	InboundTypeOpenAIChat:      func() model.Inbound { return &openai.ChatInbound{} },
	InboundTypeOpenAIResponse:  func() model.Inbound { return &openai.ResponseInbound{} },
	InboundTypeOpenAIEmbedding: func() model.Inbound { return &openai.EmbeddingInbound{} },
	InboundTypeAnthropic:       func() model.Inbound { return &anthropic.MessagesInbound{} },
}

func Get(inboundType InboundType) model.Inbound {
	if factory, ok := inboundFactories[inboundType]; ok {
		return factory()
	}
	return nil
}

// inboundToOutbound 映射入站格式到对应的出站 channel 类型
var inboundToOutbound = map[InboundType]outbound.OutboundType{
	InboundTypeAnthropic:       outbound.OutboundTypeAnthropic,
	InboundTypeOpenAIChat:      outbound.OutboundTypeOpenAIChat,
	InboundTypeOpenAIResponse:  outbound.OutboundTypeOpenAIResponse,
	InboundTypeOpenAIEmbedding: outbound.OutboundTypeOpenAIEmbedding,
}

// MatchesOutbound 判断入站格式是否与出站 channel 类型匹配（可走 passthrough 路径）
func MatchesOutbound(in InboundType, out outbound.OutboundType) bool {
	match, ok := inboundToOutbound[in]
	return ok && match == out
}
