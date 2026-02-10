package tokenizer

import (
	"github.com/tiktoken-go/tokenizer/codec"
)

func CountTokens(content, model string) int {
	// TODO 更多模型
	// 默认使用 o200k_base (GPT-4o)
	enc := codec.NewO200kBase()
	tc, err := enc.Count(content)
	if err != nil {
		return 0
	}
	return tc
}
