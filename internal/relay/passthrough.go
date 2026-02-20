package relay

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/bestruirui/octopus/internal/transformer/model"
	"github.com/bestruirui/octopus/internal/utils/log"
	"github.com/tmaxmax/go-sse"
)

// replaceModelInBody 在 raw JSON body 中替换顶层 "model" 字段。
// 如果 oldModel 和 newModel 相同，直接返回原始 body。
func replaceModelInBody(body []byte, oldModel, newModel string) []byte {
	if oldModel == newModel || newModel == "" {
		return body
	}

	// 使用 json.RawMessage 方式轻量替换
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		// 解析失败，返回原始 body
		return body
	}

	newModelJSON, err := json.Marshal(newModel)
	if err != nil {
		return body
	}
	raw["model"] = newModelJSON

	result, err := json.Marshal(raw)
	if err != nil {
		return body
	}
	return result
}

// forwardPassthrough 使用 passthrough 模式转发请求
func (ra *relayAttempt) forwardPassthrough(passthrough model.PassthroughOutbound) (int, error) {
	ctx := ra.c.Request.Context()

	// 替换 model 名（如果需要）
	body := replaceModelInBody(ra.rawBody, ra.requestModel, ra.internalRequest.Model)

	stream := ra.internalRequest.Stream != nil && *ra.internalRequest.Stream

	// 构建 passthrough 请求
	// 使用 ra.baseUrl，它已经正确设置了 Channel 或 OAuth Provider 的 base URL
	outboundRequest, err := passthrough.BuildPassthroughRequest(
		ctx,
		body,
		stream,
		ra.baseUrl,
		ra.usedKey.ChannelKey,
		ra.internalRequest.Query,
	)
	if err != nil {
		log.Warnf("failed to create passthrough request: %v", err)
		return 0, fmt.Errorf("failed to create passthrough request: %w", err)
	}

	// 复制请求头
	ra.copyHeaders(outboundRequest)

	// 发送请求
	response, err := ra.sendRequest(outboundRequest)
	if err != nil {
		return 0, fmt.Errorf("failed to send request: %w", err)
	}
	defer response.Body.Close()

	// 检查响应状态
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		respBody, err := io.ReadAll(response.Body)
		if err != nil {
			return 0, fmt.Errorf("failed to read response body: %w", err)
		}
		// 检查是否是 OAuth channel 的 Invalid apiKey 错误
		if ra.channel != nil && ra.channel.UseOAuth && isInvalidAPIKeyError(respBody) {
			return 0, fmt.Errorf("OAuth channel '%s' returned Invalid apiKey. Please update the credential in OAuth Provider settings. Response: %s", ra.channelName, string(respBody))
		}
		return 0, fmt.Errorf("upstream error: %d: %s", response.StatusCode, string(respBody))
	}

	// 处理响应：直接 pipe 原始响应
	if stream {
		if err := ra.handlePassthroughStreamResponse(ctx, response); err != nil {
			return 0, err
		}
		return response.StatusCode, nil
	}
	if err := ra.handlePassthroughResponse(ctx, response); err != nil {
		return 0, err
	}
	return response.StatusCode, nil
}

// handlePassthroughResponse 直接将上游非流式响应 pipe 给客户端
func (ra *relayAttempt) handlePassthroughResponse(ctx context.Context, response *http.Response) error {
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	// 直接写回客户端
	ra.c.Data(http.StatusOK, "application/json", body)
	ra.metrics.SetFinalResponse(string(body))

	// 从 raw response 中解析 usage 用于 metrics
	extractAndSetUsage(ra.metrics, body, ra.internalRequest.Model)

	return nil
}

// handlePassthroughStreamResponse 直接将上游流式响应 pipe 给客户端
func (ra *relayAttempt) handlePassthroughStreamResponse(ctx context.Context, response *http.Response) error {
	if ct := response.Header.Get("Content-Type"); ct != "" && !strings.Contains(strings.ToLower(ct), "text/event-stream") {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 16*1024))
		return fmt.Errorf("upstream returned non-SSE content-type %q for stream request: %s", ct, string(body))
	}

	// 设置 SSE 响应头
	ra.c.Header("Content-Type", "text/event-stream")
	ra.c.Header("Cache-Control", "no-cache")
	ra.c.Header("Connection", "keep-alive")
	ra.c.Header("X-Accel-Buffering", "no")

	firstToken := true
	var lastEventData string
	var rawResponse bytes.Buffer

	type sseReadResult struct {
		data string
		err  error
	}
	results := make(chan sseReadResult, 1)
	go func() {
		defer close(results)
		readCfg := &sse.ReadConfig{MaxEventSize: maxSSEEventSize}
		for ev, err := range sse.Read(response.Body, readCfg) {
			if err != nil {
				results <- sseReadResult{err: err}
				return
			}
			results <- sseReadResult{data: ev.Data}
		}
	}()

	var firstTokenTimer *time.Timer
	var firstTokenC <-chan time.Time
	if firstToken && ra.firstTokenTimeOutSec > 0 {
		firstTokenTimer = time.NewTimer(time.Duration(ra.firstTokenTimeOutSec) * time.Second)
		firstTokenC = firstTokenTimer.C
		defer func() {
			if firstTokenTimer != nil {
				firstTokenTimer.Stop()
			}
		}()
	}

	for {
		select {
		case <-ctx.Done():
			log.Infof("client disconnected, stopping passthrough stream")
			return nil
		case <-firstTokenC:
			log.Warnf("first token timeout (%ds), switching channel", ra.firstTokenTimeOutSec)
			_ = response.Body.Close()
			// 尝试从已收集的数据中解析 metrics
			if rawResponse.Len() > 0 {
				extractAndSetUsage(ra.metrics, rawResponse.Bytes(), ra.internalRequest.Model)
			}
			return fmt.Errorf("first token timeout (%ds)", ra.firstTokenTimeOutSec)
		case r, ok := <-results:
			if !ok {
				// stream 结束，尝试构建完整的 InternalResponse 用于日志
				ra.captureFinalStreamResponse(ctx)

				// 如果 captureFinalStreamResponse 没有生成 FinalResponse (例如解析失败)，
				// 则回退到使用原始累积的 rawResponse
				if ra.metrics.FinalResponse == "" {
					if lastEventData != "" {
						extractAndSetUsage(ra.metrics, []byte(lastEventData), ra.internalRequest.Model)
					}
					ra.metrics.SetFinalResponse(rawResponse.String())
				}
				log.Infof("passthrough stream end")
				return nil
			}
			if r.err != nil {
				log.Warnf("failed to read event: %v", r.err)
				return fmt.Errorf("failed to read stream event: %w", r.err)
			}

			lastEventData = r.data

			if firstToken {
				ra.metrics.SetFirstTokenTime(time.Now())
				firstToken = false
				if firstTokenTimer != nil {
					if !firstTokenTimer.Stop() {
						select {
						case <-firstTokenTimer.C:
						default:
						}
					}
					firstTokenTimer = nil
					firstTokenC = nil
				}
			}

			// 尝试解析并聚合 chunk 用于日志
			// 注意：这里只进行解析和聚合，不影响向客户端的输出
			// 忽略错误，保证 passthrough 的核心功能（转发）不受影响
			if internalStream, err := ra.outAdapter.TransformStream(ctx, []byte(r.data)); err == nil && internalStream != nil {
				_, _ = ra.inAdapter.TransformStream(ctx, internalStream)
			}

			// 直接写入原始 SSE event
			sseData := fmt.Sprintf("data: %s\n\n", r.data)
			ra.c.Writer.Write([]byte(sseData))
			ra.c.Writer.Flush()
			rawResponse.WriteString(sseData)
		}
	}
}

// extractAndSetUsage 从 raw response body 中解析 usage 信息用于 metrics
func extractAndSetUsage(metrics *RelayMetrics, body []byte, actualModel string) {
	// 尝试解析通用 usage 结构
	var resp struct {
		Usage *usageInfo `json:"usage,omitempty"`
	}
	if err := json.Unmarshal(body, &resp); err != nil || resp.Usage == nil {
		return
	}

	internalResp := &model.InternalLLMResponse{
		Usage: &model.Usage{
			PromptTokens:             resp.Usage.InputTokens + resp.Usage.PromptTokens,
			CompletionTokens:         resp.Usage.OutputTokens + resp.Usage.CompletionTokens,
			CacheCreationInputTokens: resp.Usage.CacheCreationInputTokens,
		},
	}

	// Anthropic usage 标记
	if resp.Usage.CacheReadInputTokens > 0 || resp.Usage.CacheCreationInputTokens > 0 {
		internalResp.Usage.AnthropicUsage = true
		if internalResp.Usage.PromptTokensDetails == nil {
			internalResp.Usage.PromptTokensDetails = &model.PromptTokensDetails{}
		}
		internalResp.Usage.PromptTokensDetails.CachedTokens = resp.Usage.CacheReadInputTokens
	}

	// OpenAI cached tokens
	if resp.Usage.PromptTokensDetails != nil && resp.Usage.PromptTokensDetails.CachedTokens > 0 {
		if internalResp.Usage.PromptTokensDetails == nil {
			internalResp.Usage.PromptTokensDetails = &model.PromptTokensDetails{}
		}
		internalResp.Usage.PromptTokensDetails.CachedTokens = resp.Usage.PromptTokensDetails.CachedTokens
	}

	metrics.SetInternalResponse(internalResp, actualModel)
}

// usageInfo 是通用的 usage 结构，兼容 OpenAI 和 Anthropic
type usageInfo struct {
	// OpenAI 字段
	PromptTokens        int64                `json:"prompt_tokens,omitempty"`
	CompletionTokens    int64                `json:"completion_tokens,omitempty"`
	PromptTokensDetails *promptTokensDetails `json:"prompt_tokens_details,omitempty"`

	// Anthropic 字段
	InputTokens              int64 `json:"input_tokens,omitempty"`
	OutputTokens             int64 `json:"output_tokens,omitempty"`
	CacheReadInputTokens     int64 `json:"cache_read_input_tokens,omitempty"`
	CacheCreationInputTokens int64 `json:"cache_creation_input_tokens,omitempty"`
}

type promptTokensDetails struct {
	CachedTokens int64 `json:"cached_tokens,omitempty"`
}

// isInvalidAPIKeyError checks if the response body indicates an invalid API key error
// from iflow or similar OAuth providers
func isInvalidAPIKeyError(body []byte) bool {
	// Check for iflow specific error format: {"status":"434","msg":"Invalid apiKey..."}
	bodyStr := strings.ToLower(string(body))
	return strings.Contains(bodyStr, "invalid apikey") || strings.Contains(bodyStr, "invalid api key")
}
