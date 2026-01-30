package relay

import (
	"github.com/bestruirui/octopus/internal/transformer/model"
	"github.com/bestruirui/octopus/internal/utils/tokenizer"
)

// EstimateRequestTokens 估算请求内容的 token 数
func EstimateRequestTokens(req *model.InternalLLMRequest) int {
	if req == nil {
		return 0
	}

	var totalTokens int

	// 估算 embedding 请求的 token 数
	if req.EmbeddingInput != nil {
		if req.EmbeddingInput.Single != nil {
			totalTokens += tokenizer.CountTokens(*req.EmbeddingInput.Single, req.Model)
		}
		for _, text := range req.EmbeddingInput.Multiple {
			totalTokens += tokenizer.CountTokens(text, req.Model)
		}
	}

	// 估算 messages 中的文本内容
	for _, msg := range req.Messages {
		if msg.Content.Content != nil {
			totalTokens += tokenizer.CountTokens(*msg.Content.Content, req.Model)
		}

		// 处理 multiple content
		for _, part := range msg.Content.MultipleContent {
			if part.Type == "text" && part.Text != nil {
				totalTokens += tokenizer.CountTokens(*part.Text, req.Model)
			}
		}
	}

	return totalTokens
}

// EstimateResponseTokens 估算响应内容的 token 数
func EstimateResponseTokens(resp *model.InternalLLMResponse) int {
	if resp == nil {
		return 0
	}

	var totalTokens int

	// 遍历 Choices
	for _, choice := range resp.Choices {
		// 处理 Message
		if choice.Message != nil {
			if choice.Message.Content.Content != nil {
				totalTokens += tokenizer.CountTokens(*choice.Message.Content.Content, resp.Model)
			}
			// 处理 multiple content
			for _, part := range choice.Message.Content.MultipleContent {
				if part.Type == "text" && part.Text != nil {
					totalTokens += tokenizer.CountTokens(*part.Text, resp.Model)
				}
			}
			// 处理 ReasoningContent
			if choice.Message.ReasoningContent != nil {
				totalTokens += tokenizer.CountTokens(*choice.Message.ReasoningContent, resp.Model)
			}
		}

		// 处理 Delta
		if choice.Delta != nil {
			if choice.Delta.Content.Content != nil {
				totalTokens += tokenizer.CountTokens(*choice.Delta.Content.Content, resp.Model)
			}
			// 处理 multiple content
			for _, part := range choice.Delta.Content.MultipleContent {
				if part.Type == "text" && part.Text != nil {
					totalTokens += tokenizer.CountTokens(*part.Text, resp.Model)
				}
			}
			// 处理 ReasoningContent
			if choice.Delta.ReasoningContent != nil {
				totalTokens += tokenizer.CountTokens(*choice.Delta.ReasoningContent, resp.Model)
			}
		}
	}

	return totalTokens
}
