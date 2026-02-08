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

	// 基础信息
	ChannelID      int
	ChannelType    int
	APIKeyID       int
	ChannelName    string // 渠道名称
	RequestModel   string // 请求的模型名称
	ActualModel    string // 实际使用的模型名称
	StartTime      time.Time
	FirstTokenTime time.Time // 首个 Token 时间（流式场景）

	// 请求和响应内容
	InternalRequest  *transformerModel.InternalLLMRequest
	InternalResponse *transformerModel.InternalLLMResponse

	// 统计指标
	ActualModel string
	Stats       model.StatsMetrics
	Stats model.StatsMetrics

	// 重试信息
	Attempts []model.ChannelAttempt

	// 原始请求和响应
	RawRequest  string
	RawResponse *strings.Builder
}

func NewRelayMetrics(apiKeyID int, requestModel string, req *transformerModel.InternalLLMRequest) *RelayMetrics {
	return &RelayMetrics{
		APIKeyID:        apiKeyID,
		RequestModel:    requestModel,
		StartTime:       time.Now(),
		InternalRequest: req,
		RawResponse:  &strings.Builder{},
	}
}

func (m *RelayMetrics) SetRawRequest(body []byte) {
	m.RawRequest = string(body)
}

func (m *RelayMetrics) AppendRawResponse(body []byte) {
	m.RawResponse.Write(body)
}

func (m *RelayMetrics) SetAPIKeyID(apiKeyID int) {
	m.APIKeyID = apiKeyID
}

// SetChannel 设置通道信息
func (m *RelayMetrics) SetChannel(channelID int, channelType int, channelName string, actualModel string) {
	m.ChannelID = channelID
	m.ChannelType = channelType
	m.ChannelName = channelName
	m.ActualModel = actualModel
}

// SetFirstTokenTime 设置首个 Token 时间
func (m *RelayMetrics) SetFirstTokenTime(t time.Time) {
	m.FirstTokenTime = t
}

// SetInternalRequest 设置内部请求
func (m *RelayMetrics) SetInternalRequest(req *transformerModel.InternalLLMRequest) {
	m.InternalRequest = req
}

// AddAttempt 记录单次渠道尝试的信息
func (m *RelayMetrics) AddAttempt(round int, attemptNum int, success bool, err error, duration time.Duration, apiKeySuffix string) {
	attempt := model.ChannelAttempt{
		ChannelID:    m.ChannelID,
		ChannelName:  m.ChannelName,
		ModelName:    m.ActualModel,
		Round:        round,
		AttemptNum:   attemptNum,
		Success:      success,
		ApiKeySuffix: apiKeySuffix,
		Duration:     int(duration.Milliseconds()),
	}
	if err != nil {
		attempt.Error = err.Error()
	}
	m.Attempts = append(m.Attempts, attempt)
	m.saveStats(success, duration)
}

// SetInternalResponse 设置内部响应并计算费用
func (m *RelayMetrics) SetInternalResponse(resp *transformerModel.InternalLLMResponse) {
	m.InternalResponse = resp
	m.ActualModel = actualModel

	if resp == nil || resp.Usage == nil {
		return
	// 从响应中提取 Usage 并计算费用
	if resp != nil && resp.Usage != nil {
		m.Stats.InputToken = resp.Usage.PromptTokens
		m.Stats.OutputToken = resp.Usage.CompletionTokens
	}

	// 如果没有 Usage 或者 Token 数为 0，尝试重新计算
	if m.Stats.InputToken == 0 {
		m.Stats.InputToken = m.calcInputTokens()
	}
	if m.Stats.OutputToken == 0 {
		m.Stats.OutputToken = m.calcOutputTokens()
	}

	modelPrice := price.GetLLMPrice(actualModel)
	if modelPrice == nil {
		return
	}

	// 如果有 Usage 并且有 PromptTokensDetails，则可以计算更精确的费用
	if resp != nil && resp.Usage != nil {
		usage := resp.Usage
		if usage.PromptTokensDetails == nil {
			usage.PromptTokensDetails = &transformerModel.PromptTokensDetails{
				CachedTokens: 0,
			}
		}

		// 优先使用已修正的 Token 数
		promptTokens := float64(m.Stats.InputToken)
		cachedTokens := float64(usage.PromptTokensDetails.CachedTokens)

		if usage.AnthropicUsage {
			m.Stats.InputCost = (cachedTokens*modelPrice.CacheRead +
				promptTokens*modelPrice.Input +
				float64(usage.CacheCreationInputTokens)*modelPrice.CacheWrite) * 1e-6
		} else {
			m.Stats.InputCost = (cachedTokens*modelPrice.CacheRead + (promptTokens-cachedTokens)*modelPrice.Input) * 1e-6
		}
	} else {
		// 否则使用普通的计算方式
		m.Stats.InputCost = float64(m.Stats.InputToken) * modelPrice.Input * 1e-6
	}

	m.Stats.OutputCost = float64(m.Stats.OutputToken) * modelPrice.Output * 1e-6
}

// calcInputTokens 计算输入 Token
func (m *RelayMetrics) calcInputTokens() int64 {
	if m.InternalRequest == nil {
		return 0
	}
	content := ""
	for _, msg := range m.InternalRequest.Messages {
		if msg.Content.Content != nil {
			content += *msg.Content.Content
		}
		for _, part := range msg.Content.MultipleContent {
			if part.Text != nil {
				content += *part.Text
			}
		}
	}
	return int64(tokenizer.CountTokens(content, m.RequestModel))
}

// calcOutputTokens 计算输出 Token
func (m *RelayMetrics) calcOutputTokens() int64 {
	if m.InternalResponse == nil {
		return 0
	}
	content := ""
	for _, choice := range m.InternalResponse.Choices {
		if choice.Message != nil {
			content += extractMessageContent(choice.Message)
		}
		if choice.Delta != nil {
			content += extractMessageContent(choice.Delta)
		}
	}
	return int64(tokenizer.CountTokens(content, m.RequestModel))
}

// extractMessageContent extracts all text content from a message, including tool call arguments.
func extractMessageContent(msg *transformerModel.Message) string {
	content := ""
	if msg.Content.Content != nil {
		content += *msg.Content.Content
	}
	for _, part := range msg.Content.MultipleContent {
		if part.Text != nil {
			content += *part.Text
		}
	}
	content += msg.GetReasoningContent()
	for _, tc := range msg.ToolCalls {
		content += tc.Function.Name
		content += tc.Function.Arguments
	}
	return content
}

// CalcTokensFromRequest calculates tokens from the request alone when no response is available.
// This is a last-resort fallback to ensure input tokens are always recorded.
func (m *RelayMetrics) CalcTokensFromRequest() {
	if m.Stats.InputToken == 0 {
		m.Stats.InputToken = m.calcInputTokens()
	}

	// Calculate cost with whatever tokens we have
	modelPrice := price.GetLLMPrice(m.ActualModel)
	if modelPrice == nil {
		return
	}
	m.Stats.InputCost = float64(m.Stats.InputToken) * modelPrice.Input * 1e-6
	m.Stats.OutputCost = float64(m.Stats.OutputToken) * modelPrice.Output * 1e-6
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

	channelID, channelName := finalChannel(attempts)

	log.Infof("relay complete: model=%s, channel=%d(%s), success=%t, duration=%dms, input_token=%d, output_token=%d, input_cost=%f, output_cost=%f, total_cost=%f, attempts=%d",
		m.RequestModel, channelID, channelName, success, duration.Milliseconds(),
		m.Stats.InputToken, m.Stats.OutputToken,
		m.Stats.InputCost, m.Stats.OutputCost, m.Stats.InputCost+m.Stats.OutputCost,
		len(attempts))

	m.saveLog(ctx, err, duration, attempts, channelID, channelName)
}

func finalChannel(attempts []model.ChannelAttempt) (int, string) {
	var lastID int
	var lastName string
	for i := len(attempts) - 1; i >= 0; i-- {
		a := attempts[i]
		if a.Status == model.AttemptSuccess {
			return a.ChannelID, a.ChannelName
		}
		if a.Status == model.AttemptFailed && lastID == 0 {
			lastID = a.ChannelID
			lastName = a.ChannelName
		}
	}
	return lastID, lastName
}

func (m *RelayMetrics) saveLog(ctx context.Context, err error, duration time.Duration, attempts []model.ChannelAttempt, channelID int, channelName string) {
	actualModel := m.ActualModel
	if actualModel == "" {
		actualModel = m.RequestModel
	}

	relayLog := model.RelayLog{
		Time:             m.StartTime.Unix(),
		RequestModelName: m.RequestModel,

		ChannelName:      m.ChannelName,
		ChannelId:        m.ChannelID,
		ChannelType:      m.ChannelType,
		ActualModelName:  m.ActualModel,
		UseTime:          int(duration.Milliseconds()),
		Attempts:         attempts,
		TotalAttempts:    len(attempts),
	}

	// 首字时间
	if !m.FirstTokenTime.IsZero() {
		relayLog.Ftut = int(m.FirstTokenTime.Sub(m.StartTime).Milliseconds())
	}

	// Usage
	if m.InternalResponse != nil && m.InternalResponse.Usage != nil {
		relayLog.InputTokens = int(m.InternalResponse.Usage.PromptTokens)
		relayLog.OutputTokens = int(m.InternalResponse.Usage.CompletionTokens)
		relayLog.Cost = m.Stats.InputCost + m.Stats.OutputCost
	}


	// 设置请求内容
	m.RawRequest = strings.ReplaceAll(m.RawRequest, "\n", "")
	m.RawRequest = strings.ReplaceAll(m.RawRequest, "\r", "")
	relayLog.RequestContent = m.RawRequest

	// 响应内容
	if m.InternalResponse != nil {
		respForLog := m.filterResponseForLog(m.InternalResponse)
		if respJSON, jsonErr := json.Marshal(respForLog); jsonErr == nil {
			if m.InternalResponse.Usage != nil && m.InternalResponse.Usage.AnthropicUsage {
				respStr := string(respJSON)
				ol := `"usage":{`
				insert := fmt.Sprintf(`"usage":{"cache_creation_input_tokens":%d,`, m.InternalResponse.Usage.CacheCreationInputTokens)
				respJSON = []byte(strings.Replace(respStr, ol, insert, 1))
			}
			relayLog.ResponseContent = string(respJSON)
		}
	} else {
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
