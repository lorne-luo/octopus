package relay

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/op"
	"github.com/bestruirui/octopus/internal/price"
	transformerModel "github.com/bestruirui/octopus/internal/transformer/model"
	"github.com/bestruirui/octopus/internal/utils/log"
	"github.com/bestruirui/octopus/internal/utils/tokenizer"
)

// RelayMetrics 负责最终的日志收集与持久化
type RelayMetrics struct {
	APIKeyID     int
	RequestModel string
	StartTime    time.Time

	// 首 Token 时间
	FirstTokenTime time.Time

	// 请求和响应内容
	InternalRequest  *transformerModel.InternalLLMRequest
	InternalResponse *transformerModel.InternalLLMResponse

	// 原始请求和响应（用于调试）
	RawRequest  string
	RawResponse *strings.Builder

	// 统计指标
	ActualModel string
	Stats       model.StatsMetrics
}

func NewRelayMetrics(apiKeyID int, requestModel string, req *transformerModel.InternalLLMRequest) *RelayMetrics {
	return &RelayMetrics{
		APIKeyID:        apiKeyID,
		RequestModel:    requestModel,
		StartTime:       time.Now(),
		InternalRequest: req,
		RawResponse:     &strings.Builder{},
	}
}

func (m *RelayMetrics) SetFirstTokenTime(t time.Time) {
	m.FirstTokenTime = t
}

// SetRawRequest sets the raw request string for logging/debugging purposes
func (m *RelayMetrics) SetRawRequest(req string) {
	m.RawRequest = req
}

func (m *RelayMetrics) AppendRawResponse(resp string) {
	m.RawResponse.WriteString(resp)
}

func (m *RelayMetrics) SetInternalResponse(resp *transformerModel.InternalLLMResponse, actualModel string) {
	m.InternalResponse = resp
	m.ActualModel = actualModel

	if resp != nil && resp.Usage != nil {
		usage := resp.Usage
		m.Stats.InputToken = usage.PromptTokens
		m.Stats.OutputToken = usage.CompletionTokens
	} else {
		// 如果 Usage 缺失，进行估算
		m.Stats.InputToken = int64(m.calcInputTokens())
		m.Stats.OutputToken = int64(m.calcOutputTokens())
	}

	modelPrice := price.GetLLMPrice(actualModel)
	if modelPrice == nil {
		// 如果没有价格配置，尝试回退到简单的估算（如果有默认价格逻辑的话，这里假设没有）
		// 但我们至少应该保留 Token 计数
		return
	}

	var cachedTokens int64
	var anthropicUsage bool
	var cacheCreationInputTokens int64

	if resp != nil && resp.Usage != nil {
		if resp.Usage.PromptTokensDetails != nil {
			cachedTokens = resp.Usage.PromptTokensDetails.CachedTokens
		}
		anthropicUsage = resp.Usage.AnthropicUsage
		cacheCreationInputTokens = resp.Usage.CacheCreationInputTokens
	}

	// 费用计算逻辑
	if anthropicUsage {
		// Anthropic: Input (Cache Read) + Input (Base) + Input (Cache Write)
		m.Stats.InputCost = (float64(cachedTokens)*modelPrice.CacheRead +
			float64(m.Stats.InputToken)*modelPrice.Input +
			float64(cacheCreationInputTokens)*modelPrice.CacheWrite) * 1e-6
	} else {
		// OpenAI Style: Cache Read + (Total Input - Cache Read) * Input Price
		// 注意：这里的 InputToken 通常包含了 CachedTokens，所以要减去
		inputBase := m.Stats.InputToken - cachedTokens
		if inputBase < 0 {
			inputBase = 0 // 防御性编程
		}
		m.Stats.InputCost = (float64(cachedTokens)*modelPrice.CacheRead + float64(inputBase)*modelPrice.Input) * 1e-6
	}
	m.Stats.OutputCost = float64(m.Stats.OutputToken) * modelPrice.Output * 1e-6
}

// calcInputTokens 计算输入 Token
func (m *RelayMetrics) calcInputTokens() int {
	if m.InternalRequest == nil {
		return 0
	}
	// 手动拼接内容进行计算，或者使用 GetFullContent (如果已实现)
	// 这里根据 022/023 的描述，我们手动遍历拼接
	var contentBuilder strings.Builder
	for _, msg := range m.InternalRequest.Messages {
		if msg.Content.Content != nil && *msg.Content.Content != "" {
			contentBuilder.WriteString(*msg.Content.Content)
		}
		for _, part := range msg.Content.MultipleContent {
			if part.Type == "text" && part.Text != nil {
				contentBuilder.WriteString(*part.Text)
			}
		}
		// TODO: Tool Calls 等其他内容是否计入取决于 Tokenizer 实现和模型特性
	}
	// 这里简单起见，只计算文本内容。如果 GetFullContent 已实现，应优先使用。
	// 但鉴于我尚未实现 GetFullContent，这里做简单处理。
	// 为了更准确，应该包含 System Prompt 等。

	// 修正：023 提到 "Use GetFullContent". 我将在 internal/model/metrics.go 中不实现 GetFullContent，而是依赖 calcInputTokens 的逻辑。
	// 但是 024 是 "Refactor: Use GetFullContent".
	// 我在 Phase 3 中，024 不在 Prompt List 中。
	// 所以我在这里实现逻辑。

	return int(tokenizer.CountTokens(contentBuilder.String(), m.ActualModel))
}

// calcOutputTokens 计算输出 Token
func (m *RelayMetrics) calcOutputTokens() int {
	if m.InternalResponse == nil {
		return 0
	}
	var contentBuilder strings.Builder
	for _, choice := range m.InternalResponse.Choices {
		if choice.Message != nil && choice.Message.Content.Content != nil {
			contentBuilder.WriteString(*choice.Message.Content.Content)
			// Handle multiple content if needed
		}
		if choice.Delta != nil && choice.Delta.Content.Content != nil {
			contentBuilder.WriteString(*choice.Delta.Content.Content)
		}
	}
	return int(tokenizer.CountTokens(contentBuilder.String(), m.ActualModel))
}

// CalcTokensFromRequest 用于无响应时的兜底统计
func (m *RelayMetrics) CalcTokensFromRequest() {
	m.Stats.InputToken = int64(m.calcInputTokens())

	modelPrice := price.GetLLMPrice(m.ActualModel)
	if modelPrice != nil {
		m.Stats.InputCost = float64(m.Stats.InputToken) * modelPrice.Input * 1e-6
	}
}

func (m *RelayMetrics) Save(ctx context.Context, success bool, err error, attempts []model.ChannelAttempt) {
	duration := time.Since(m.StartTime)

	globalStats := model.StatsMetrics{
		WaitTime:    duration.Milliseconds(),
		InputToken:  m.Stats.InputToken,
		OutputToken: m.Stats.OutputToken,
		InputCost:   m.Stats.InputCost,
		OutputCost:  m.Stats.OutputCost,
	}
	if success {
		globalStats.RequestSuccess = 1
	} else {
		globalStats.RequestFailed = 1
	}

	op.StatsTotalUpdate(globalStats)
	op.StatsHourlyUpdate(globalStats)
	op.StatsDailyUpdate(context.Background(), globalStats)
	op.StatsAPIKeyUpdate(m.APIKeyID, globalStats)

	channelID, channelName, channelType := finalChannel(attempts)

	log.Infof("relay complete: model=%s, channel=%d(%s), type=%d, success=%t, duration=%dms, input_token=%d, output_token=%d, input_cost=%f, output_cost=%f, total_cost=%f, attempts=%d",
		m.RequestModel, channelID, channelName, channelType, success, duration.Milliseconds(),
		m.Stats.InputToken, m.Stats.OutputToken,
		m.Stats.InputCost, m.Stats.OutputCost, m.Stats.InputCost+m.Stats.OutputCost,
		len(attempts))

	m.saveLog(ctx, err, duration, attempts, channelID, channelName, channelType)
}

func finalChannel(attempts []model.ChannelAttempt) (int, string, int) {
	var lastID int
	var lastName string
	var lastType int
	for i := len(attempts) - 1; i >= 0; i-- {
		a := attempts[i]
		if a.Status == model.AttemptSuccess {
			return a.ChannelID, a.ChannelName, a.ChannelType
		}
		if a.Status == model.AttemptFailed && lastID == 0 {
			lastID = a.ChannelID
			lastName = a.ChannelName
			lastType = a.ChannelType
		}
	}
	return lastID, lastName, lastType
}

func (m *RelayMetrics) saveLog(ctx context.Context, err error, duration time.Duration, attempts []model.ChannelAttempt, channelID int, channelName string, channelType int) {
	actualModel := m.ActualModel
	if actualModel == "" {
		actualModel = m.RequestModel
	}

	relayLog := model.RelayLog{
		Time:             m.StartTime.Unix(),
		RequestModelName: m.RequestModel,
		ChannelName:      channelName,
		ChannelId:        channelID,
		ChannelType:      channelType,
		ActualModelName:  actualModel,
		UseTime:          int(duration.Milliseconds()),
		Attempts:         attempts,
		TotalAttempts:    len(attempts),
	}

	// 首字时间
	if !m.FirstTokenTime.IsZero() {
		relayLog.Ftut = int(m.FirstTokenTime.Sub(m.StartTime).Milliseconds())
	}

	// Usage
	// 使用 Metrics 中的统计值，因为可能来自估算
	relayLog.InputTokens = m.Stats.InputToken
	relayLog.OutputTokens = m.Stats.OutputToken
	relayLog.Cost = m.Stats.InputCost + m.Stats.OutputCost

	// 请求内容
	// 优先使用 RawRequest
	if m.RawRequest != "" {
		relayLog.RequestContent = strings.ReplaceAll(m.RawRequest, "\n", " ")
	} else if m.InternalRequest != nil {
		if reqJSON, jsonErr := json.Marshal(m.InternalRequest); jsonErr == nil {
			relayLog.RequestContent = string(reqJSON)
		}
	}

	// 响应内容
	if m.InternalResponse != nil {
		// 如果有 InternalResponse，使用过滤后的 JSON
		respForLog := m.filterResponseForLog(m.InternalResponse)
		if respJSON, jsonErr := json.Marshal(respForLog); jsonErr == nil {
			if m.InternalResponse.Usage != nil && m.InternalResponse.Usage.AnthropicUsage {
				respStr := string(respJSON)
				old := `"usage":{`
				insert := fmt.Sprintf(`"usage":{"cache_creation_input_tokens":%d,`, m.InternalResponse.Usage.CacheCreationInputTokens)
				respJSON = []byte(strings.Replace(respStr, old, insert, 1))
			}
			relayLog.ResponseContent = string(respJSON)
		}
	} else if m.RawResponse.Len() > 0 {
		// 如果没有 InternalResponse (例如直接透传或失败)，尝试使用 RawResponse
		relayLog.ResponseContent = m.RawResponse.String()
	}

	// 错误信息
	if err != nil {
		relayLog.Error = err.Error()
	}

	if logErr := op.RelayLogAdd(ctx, relayLog); logErr != nil {
		log.Warnf("failed to save relay log: %v", logErr)
	}
}

// filterResponseForLog 创建响应的浅拷贝，过滤掉 images、MultipleContent 中的图片数据和 Audio.Data 以减少存储压力
func (m *RelayMetrics) filterResponseForLog(resp *transformerModel.InternalLLMResponse) *transformerModel.InternalLLMResponse {
	if resp == nil {
		return nil
	}

	filterMsg := func(msg *transformerModel.Message) *transformerModel.Message {
		if msg == nil {
			return nil
		}
		c := *msg
		c.Images = nil
		if len(c.Content.MultipleContent) > 0 {
			parts := make([]transformerModel.MessageContentPart, 0, len(c.Content.MultipleContent))
			for _, p := range c.Content.MultipleContent {
				if p.Type == "image_url" && p.ImageURL != nil {
					parts = append(parts, transformerModel.MessageContentPart{
						Type:     "image_url",
						ImageURL: &transformerModel.ImageURL{URL: "[image data omitted for storage]"},
					})
				} else {
					parts = append(parts, p)
				}
			}
			c.Content = transformerModel.MessageContent{Content: c.Content.Content, MultipleContent: parts}
		}
		if c.Audio != nil && c.Audio.Data != "" {
			a := *c.Audio
			a.Data = "[audio data omitted for storage]"
			c.Audio = &a
		}
		return &c
	}

	filtered := *resp
	filtered.Choices = make([]transformerModel.Choice, len(resp.Choices))
	for i, choice := range resp.Choices {
		filtered.Choices[i] = choice
		filtered.Choices[i].Message = filterMsg(choice.Message)
		filtered.Choices[i].Delta = filterMsg(choice.Delta)
	}
	return &filtered
}
