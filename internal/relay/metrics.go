package relay

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"strings"
	"time"

	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/op"
	"github.com/bestruirui/octopus/internal/price"
	"github.com/bestruirui/octopus/internal/transformer2/canonical"
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
	CanonicalReq  *canonical.Request
	CanonicalResp *canonical.Response

	// 原始请求和响应（用于调试）
	RawRequest    string
	RawResponse   *strings.Builder
	FinalResponse string // passthrough 模式的响应内容

	// 统计指标
	ActualModel string
	Stats       model.StatsMetrics

	// 参数覆盖
	ParamOverride string
}

func NewRelayMetrics(apiKeyID int, requestModel string, req *canonical.Request) *RelayMetrics {
	return &RelayMetrics{
		APIKeyID:     apiKeyID,
		RequestModel: requestModel,
		StartTime:    time.Now(),
		CanonicalReq: req,
		RawResponse:  &strings.Builder{},
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

// SetFinalResponse sets the final response string for passthrough mode
func (m *RelayMetrics) SetFinalResponse(raw string) {
	m.FinalResponse = raw
}

// SetCanonicalResponse sets the canonical response and calculates metrics
func (m *RelayMetrics) SetCanonicalResponse(resp *canonical.Response, actualModel string) {
	m.CanonicalResp = resp
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
		anthropicUsage = resp.Usage.IsAnthropicUsage
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
	if m.CanonicalReq == nil {
		return 0
	}
	// 遍历 canonical messages 计算文本内容
	var contentBuilder strings.Builder
	for _, msg := range m.CanonicalReq.Messages {
		for _, block := range msg.Content {
			if block.Type == canonical.ContentText {
				contentBuilder.WriteString(block.Text)
			}
		}
		// TODO: Tool Calls 等其他内容是否计入取决于 Tokenizer 实现和模型特性
	}

	return int(tokenizer.CountTokens(contentBuilder.String(), m.ActualModel))
}

// calcOutputTokens 计算输出 Token
func (m *RelayMetrics) calcOutputTokens() int {
	if m.CanonicalResp == nil {
		return 0
	}
	var contentBuilder strings.Builder
	for _, choice := range m.CanonicalResp.Choices {
		for _, block := range choice.Message.Content {
			if block.Type == canonical.ContentText {
				contentBuilder.WriteString(block.Text)
			}
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

	channelID, channelName, channelType := finalChannel(attempts)
	op.StatsTotalUpdate(globalStats)
	op.StatsHourlyUpdate(globalStats)
	op.StatsDailyUpdate(context.Background(), globalStats)
	op.StatsAPIKeyUpdate(m.APIKeyID, globalStats)
	op.StatsChannelUpdate(channelID, globalStats)

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
		ActualModelName:  actualModel,
		UseTime:          int(duration.Milliseconds()),
		Attempts:         attempts,
		TotalAttempts:    len(attempts),
	}

	if apiKey, getErr := op.APIKeyGet(m.APIKeyID, ctx); getErr == nil {
		relayLog.RequestAPIKeyName = apiKey.Name
	}

	// 首字时间
	if !m.FirstTokenTime.IsZero() {
		relayLog.Ftut = int(m.FirstTokenTime.Sub(m.StartTime).Milliseconds())
	}

	// Usage
	// 使用 Metrics 中的统计值，因为可能来自估算
	relayLog.InputTokens = int(m.Stats.InputToken)
	relayLog.OutputTokens = int(m.Stats.OutputToken)
	relayLog.Cost = m.Stats.InputCost + m.Stats.OutputCost

	// 请求内容
	// 优先使用 RawRequest
	if m.RawRequest != "" {
		relayLog.RequestContent = strings.ReplaceAll(m.RawRequest, "\n", " ")
	} else if m.CanonicalReq != nil {
		reqJSON, jsonErr := json.Marshal(m.CanonicalReq)
		if jsonErr == nil {
			if m.ParamOverride == "" {
				relayLog.RequestContent = string(reqJSON)
			} else {
				var reqMap map[string]any
				if err := json.Unmarshal(reqJSON, &reqMap); err != nil {
					relayLog.RequestContent = string(reqJSON)
				} else {
					var override map[string]any
					if err := json.Unmarshal([]byte(m.ParamOverride), &override); err != nil {
						relayLog.RequestContent = string(reqJSON)
					} else {
						maps.Copy(reqMap, override)
						if finalJSON, err := json.Marshal(reqMap); err != nil {
							relayLog.RequestContent = string(reqJSON)
						} else {
							relayLog.RequestContent = string(finalJSON)
						}
					}
				}
			}
		}
	}

	// 响应内容
	if m.FinalResponse != "" {
		relayLog.ResponseContent = m.FinalResponse
	} else if m.CanonicalResp != nil {
		// 如果有 CanonicalResp，使用过滤后的 JSON
		respForLog := m.filterResponseForLog(m.CanonicalResp)
		if respJSON, jsonErr := json.Marshal(respForLog); jsonErr == nil {
			if m.CanonicalResp.Usage != nil && m.CanonicalResp.Usage.IsAnthropicUsage {
				respStr := string(respJSON)
				old := `"usage":{`
				insert := fmt.Sprintf(`"usage":{"cache_creation_input_tokens":%d,`, m.CanonicalResp.Usage.CacheCreationInputTokens)
				respJSON = []byte(strings.Replace(respStr, old, insert, 1))
			}
			relayLog.ResponseContent = string(respJSON)
		}
	} else if m.RawResponse.Len() > 0 {
		// 如果没有 CanonicalResp (例如直接透传或失败)，尝试使用 RawResponse
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

// filterResponseForLog 创建响应的浅拷贝，过滤掉 images、Media 中的图片数据和音频数据以减少存储压力
func (m *RelayMetrics) filterResponseForLog(resp *canonical.Response) *canonical.Response {
	if resp == nil {
		return nil
	}

	filterMsg := func(msg canonical.Message) canonical.Message {
		c := msg
		c.Images = nil

		// Filter content blocks with media data
		if len(c.Content) > 0 {
			filteredBlocks := make([]canonical.ContentBlock, 0, len(c.Content))
			for _, block := range c.Content {
				if block.Type == canonical.ContentImage && block.Media != nil {
					// Replace image data with placeholder
					filteredBlock := block
					filteredBlock.Media = &canonical.MediaContent{
						URL:      "[image data omitted for storage]",
						MimeType: block.Media.MimeType,
					}
					filteredBlocks = append(filteredBlocks, filteredBlock)
				} else if block.Type == canonical.ContentAudio && block.Media != nil {
					// Replace audio data with placeholder
					filteredBlock := block
					filteredBlock.Media = &canonical.MediaContent{
						MimeType: block.Media.MimeType,
					}
					filteredBlocks = append(filteredBlocks, filteredBlock)
				} else {
					filteredBlocks = append(filteredBlocks, block)
				}
			}
			c.Content = filteredBlocks
		}
		return c
	}

	filtered := *resp
	filtered.Choices = make([]canonical.Choice, len(resp.Choices))
	for i, choice := range resp.Choices {
		filtered.Choices[i] = choice
		filtered.Choices[i].Message = filterMsg(choice.Message)
	}
	return &filtered
}
