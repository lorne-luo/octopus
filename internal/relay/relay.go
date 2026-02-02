package relay

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/samber/lo"

	"github.com/bestruirui/octopus/internal/conf"
	"github.com/bestruirui/octopus/internal/helper"
	"github.com/bestruirui/octopus/internal/op"
	"github.com/bestruirui/octopus/internal/relay/balancer"
	"github.com/bestruirui/octopus/internal/server/resp"
	"github.com/bestruirui/octopus/internal/transformer/inbound"
	"github.com/bestruirui/octopus/internal/transformer/model"
	"github.com/bestruirui/octopus/internal/transformer/outbound"
	"github.com/bestruirui/octopus/internal/utils/log"
	"github.com/gin-gonic/gin"
	"github.com/tmaxmax/go-sse"
)

// Handler 处理入站请求并转发到上游服务
func Handler(inboundType inbound.InboundType, c *gin.Context) {
	// 解析请求
	internalRequest, inAdapter, err := parseRequest(inboundType, c)
	if err != nil {
		return
	}
	supportedModels := c.GetString("supported_models")
	if supportedModels != "" {
		supportedModelsArray := strings.Split(supportedModels, ",")
		if !slices.Contains(supportedModelsArray, internalRequest.Model) {
			resp.Error(c, http.StatusBadRequest, "model not supported")
			return
		}
	}

	// 初始化统计和日志
	apiKeyID := c.GetInt("api_key_id")
	metrics := NewRelayMetrics(internalRequest.Model)
	metrics.SetInternalRequest(internalRequest)
	metrics.SetAPIKeyID(apiKeyID)
	// 获取通道分组
	group, err := op.GroupGetMap(internalRequest.Model, c.Request.Context())
	if err != nil {
		resp.Error(c, http.StatusNotFound, "model not found")
		return
	}

	const maxRounds = 3
	var lastErr error
	itemCount := len(group.Items)
	b := balancer.GetBalancer(group.Mode)
	for round := 0; round < maxRounds; round++ {
		item := b.Select(group.Items)
		if item == nil {
			resp.Error(c, http.StatusServiceUnavailable, "no available channel")
			return
		}

		for i := 0; i < itemCount; i++ {
			select {
			case <-c.Request.Context().Done():
				log.Infof("request context canceled, stopping retry")
				return
			default:
			}

			attemptStart := time.Now()
			channel, err := op.ChannelGet(item.ChannelID, c.Request.Context())
			if err != nil {
				log.Warnf("failed to get channel: %v", err)
				lastErr = err
				item = b.Next(group.Items, item)
				continue
			}
			if channel.Enabled == false {
				lastErr = fmt.Errorf("channel %s is disabled", channel.Name)
				item = b.Next(group.Items, item)
				continue
			}

			log.Infof("request model %s, mode: %d, forwarding to channel: %s model: %s (round %d/%d, item %d/%d)", internalRequest.Model, group.Mode, channel.Name, item.ModelName, round+1, maxRounds, i+1, itemCount)

			internalRequest.Model = item.ModelName
			metrics.SetChannel(channel.ID, channel.Name, item.ModelName)

			outAdapter := outbound.Get(channel.Type)
			if outAdapter == nil {
				log.Warnf("unsupported channel type: %d for channel: %s", channel.Type, channel.Name)
				lastErr = fmt.Errorf("unsupported channel type: %d", channel.Type)
				item = b.Next(group.Items, item)
				continue
			}

			// 验证 channel 类型与请求类型匹配
			if internalRequest.IsEmbeddingRequest() && !outbound.IsEmbeddingChannelType(channel.Type) {
				log.Warnf("channel type %d is not compatible with embedding request for channel: %s", channel.Type, channel.Name)
				lastErr = fmt.Errorf("channel type %d not compatible with embedding request", channel.Type)
				item = b.Next(group.Items, item)
				continue
			}

			if internalRequest.IsChatRequest() && !outbound.IsChatChannelType(channel.Type) {
				log.Warnf("channel type %d is not compatible with chat request for channel: %s", channel.Type, channel.Name)
				lastErr = fmt.Errorf("channel type %d not compatible with chat request", channel.Type)
				item = b.Next(group.Items, item)
				continue
			}

			rc := &relayContext{
				c:                          c,
				inAdapter:                  inAdapter,
				outAdapter:                 outAdapter,
				internalRequest:            internalRequest,
				channel:                    channel,
				metrics:                    metrics,
				usedKey:                    channel.GetChannelKey(),
				firstTokenTimeOutSec:       group.FirstTokenTimeOut,
				nonStreamRequestTimeoutSec: conf.AppConfig.Relay.NonStreamRequestTimeoutSec,
				streamIdleTimeoutSec:       conf.AppConfig.Relay.StreamIdleTimeoutSec,
				streamNoOutputTimeoutSec:   conf.AppConfig.Relay.StreamNoOutputTimeoutSec,
			}

			if statusCode, err := rc.forward(); err == nil {
				// 成功
				attemptDuration := time.Since(attemptStart)
				key := rc.usedKey.ChannelKey
				apiKeySuffix := ""
				if len(key) > 4 {
					apiKeySuffix = key[len(key)-4:]
				} else {
					apiKeySuffix = key
				}
				rc.collectResponse()
				metrics.AddAttempt(round+1, i+1, true, nil, attemptDuration, apiKeySuffix)
				rc.usedKey.StatusCode = statusCode
				rc.usedKey.LastUseTimeStamp = time.Now().Unix()
				rc.usedKey.TotalCost += metrics.Stats.InputCost + metrics.Stats.OutputCost
				op.ChannelKeyUpdate(rc.usedKey)
				metrics.Save(c.Request.Context(), true, nil, round+1)
				return
			} else {
				// 失败
				attemptDuration := time.Since(attemptStart)
				key := rc.usedKey.ChannelKey
				apiKeySuffix := ""
				if len(key) > 4 {
					apiKeySuffix = key[len(key)-4:]
				} else {
					apiKeySuffix = key
				}
				metrics.AddAttempt(round+1, i+1, false, err, attemptDuration, apiKeySuffix)
				rc.usedKey.StatusCode = statusCode
				rc.usedKey.LastUseTimeStamp = time.Now().Unix()
				op.ChannelKeyUpdate(rc.usedKey)
				if c.Writer.Written() {
					// Streaming responses may have already started; retrying would corrupt the client stream.
					rc.collectResponse()
					metrics.Save(c.Request.Context(), false, err, 0)
					return
				}
				lastErr = fmt.Errorf("channel %s failed: %v", channel.Name, err)
			}
			item = b.Next(group.Items, item)
		}
	}

	// 所有通道都失败
	metrics.Save(c.Request.Context(), false, lastErr, 0)
	resp.Error(c, http.StatusBadGateway, "all channels failed")
}

// parseRequest 解析并验证入站请求
func parseRequest(inboundType inbound.InboundType, c *gin.Context) (*model.InternalLLMRequest, model.Inbound, error) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return nil, nil, err
	}

	inAdapter := inbound.Get(inboundType)
	internalRequest, err := inAdapter.TransformRequest(c.Request.Context(), body)
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return nil, nil, err
	}

	// Pass through the original query parameters
	internalRequest.Query = c.Request.URL.Query()

	if err := internalRequest.Validate(); err != nil {
		resp.Error(c, http.StatusBadRequest, err.Error())
		return nil, nil, err
	}

	return internalRequest, inAdapter, nil
}

// forward 转发请求到上游服务
func (rc *relayContext) forward() (int, error) {
	ctx := rc.c.Request.Context()

	// 构建出站请求
	outboundRequest, err := rc.outAdapter.TransformRequest(
		ctx,
		rc.internalRequest,
		rc.channel.GetBaseUrl(),
		rc.usedKey.ChannelKey,
	)
	if err != nil {
		log.Warnf("failed to create request: %v", err)
		return 0, fmt.Errorf("failed to create request: %w", err)
	}

	// 复制请求头
	rc.copyHeaders(outboundRequest)

	// 发送请求
	response, err := rc.sendRequest(outboundRequest)
	if err != nil {
		return 0, fmt.Errorf("failed to send request: %w", err)
	}
	defer response.Body.Close()

	// 检查响应状态
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, err := io.ReadAll(response.Body)
		if err != nil {
			return 0, fmt.Errorf("failed to read response body: %w", err)
		}

		// 尝试提取 error.message 字段，如果没有则使用整个 body
		errorContent := extractErrorMessage(body)

		// 创建包含错误信息的 InternalLLMResponse 用于日志记录
		if errorContent != "" {
			rc.metrics.SetInternalResponse(&model.InternalLLMResponse{
				Object: "error",
				Choices: []model.Choice{
					{
						Index: 0,
						FinishReason: lo.ToPtr("error"),
						Message: &model.Message{
							Role: "assistant",
							Content: model.MessageContent{
								Content: lo.ToPtr(errorContent),
							},
						},
					},
				},
			})
		}

		return 0, fmt.Errorf("upstream error: %d: %s", response.StatusCode, errorContent)
	}

	// 处理响应
	if rc.internalRequest.Stream != nil && *rc.internalRequest.Stream {
		if err := rc.handleStreamResponse(ctx, response); err != nil {
			return 0, err
		}
		return response.StatusCode, nil
	}

	// 为非流式响应体读取设置超时，防止 io.ReadAll 挂起
	readCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	if err := rc.handleResponse(readCtx, response); err != nil {
		return 0, err
	}
	return response.StatusCode, nil
}

// extractErrorMessage 从错误响应中提取错误信息
// 如果包含 error.message 字段则提取该字段，否则使用整个 body
func extractErrorMessage(body []byte) string {
	if len(body) == 0 {
		return ""
	}

	// 尝试解析 JSON 并提取 error.message
	var errResp struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
		} `json:"error"`
	}

	if err := json.Unmarshal(body, &errResp); err == nil && errResp.Error.Message != "" {
		// 如果有 type，也一起显示
		if errResp.Error.Type != "" {
			return fmt.Sprintf("%s: %s", errResp.Error.Type, errResp.Error.Message)
		}
		return errResp.Error.Message
	}

	// 解析失败或没有 error.message，使用整个 body (限制长度以防止超大 HTML 干扰)
	errMsg := strings.TrimSpace(string(body))
	if len(errMsg) > 500 {
		return errMsg[:500] + "..."
	}
	return errMsg
}

// copyHeaders 复制请求头，过滤 hop-by-hop 头
func (rc *relayContext) copyHeaders(outboundRequest *http.Request) {
	for key, values := range rc.c.Request.Header {
		if hopByHopHeaders[strings.ToLower(key)] {
			continue
		}
		for _, value := range values {
			outboundRequest.Header.Set(key, value)
		}
	}
	if len(rc.channel.CustomHeader) > 0 {
		for _, header := range rc.channel.CustomHeader {
			outboundRequest.Header.Set(header.HeaderKey, header.HeaderValue)
		}
	}
}

// sendRequest 发送 HTTP 请求
func (rc *relayContext) sendRequest(req *http.Request) (*http.Response, error) {
	httpClient, err := helper.ChannelHttpClient(rc.channel)
	if err != nil {
		log.Warnf("failed to get http client: %v", err)
		return nil, err
	}

	// 为非流式请求添加总超时控制
	if rc.internalRequest.Stream == nil || !*rc.internalRequest.Stream {
		ctx, cancel := context.WithTimeout(req.Context(), 5*time.Minute)
		defer cancel()
		req = req.WithContext(ctx)
	}

	response, err := httpClient.Do(req)
	if err != nil {
		log.Warnf("failed to send request: %v", err)
		return nil, err
	}

	return response, nil
}

// handleStreamResponse 处理流式响应
func (rc *relayContext) handleStreamResponse(ctx context.Context, response *http.Response) error {
	// 流式响应应当是 SSE
	// 某些上游可能会返回非SSE的JSON响应 (由于 Accept headers 配置错误)
	if ct := response.Header.Get("Content-Type"); ct != "" && !strings.Contains(strings.ToLower(ct), "text/event-stream") {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 16*1024))
		return fmt.Errorf("upstream returned non-SSE content-type %q for stream request: %s", ct, string(body))
	}

	// 设置 SSE 响应头
	rc.c.Header("Content-Type", "text/event-stream")
	rc.c.Header("Cache-Control", "no-cache")
	rc.c.Header("Connection", "keep-alive")
	rc.c.Header("X-Accel-Buffering", "no")

	firstToken := true

	// Streaming timeout: applies before the first token AND between tokens.
	// We read SSE events in a goroutine so we can race the output against a timer.
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

	var firstTokenTimer, streamIdleTimer, streamNoOutputTimer *time.Timer
	var firstTokenC, streamIdleC, streamNoOutputC <-chan time.Time

	if rc.firstTokenTimeOutSec > 0 {
		firstTokenTimer = time.NewTimer(time.Duration(rc.firstTokenTimeOutSec) * time.Second)
		firstTokenC = firstTokenTimer.C
	}
	if rc.streamIdleTimeoutSec > 0 {
		streamIdleTimer = time.NewTimer(time.Duration(rc.streamIdleTimeoutSec) * time.Second)
		streamIdleC = streamIdleTimer.C
	}
	if rc.streamNoOutputTimeoutSec > 0 {
		streamNoOutputTimer = time.NewTimer(time.Duration(rc.streamNoOutputTimeoutSec) * time.Second)
		streamNoOutputC = streamNoOutputTimer.C
	}

	defer func() {
		for _, t := range []*time.Timer{firstTokenTimer, streamIdleTimer, streamNoOutputTimer} {
			if t != nil {
				t.Stop()
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-firstTokenC:
			_ = response.Body.Close()
			if firstToken {
				return fmt.Errorf("first token timeout (%ds)", rc.firstTokenTimeOutSec)
			}
			return fmt.Errorf("stream stalled")
		case <-streamIdleC:
			_ = response.Body.Close()
			return fmt.Errorf("stream idle timeout (%ds)", rc.streamIdleTimeoutSec)
		case <-streamNoOutputC:
			_ = response.Body.Close()
			return fmt.Errorf("stream no output timeout (%ds)", rc.streamNoOutputTimeoutSec)
		case r, ok := <-results:
			if !ok {
				return nil
			}
			if r.err != nil {
				return fmt.Errorf("failed to read stream event: %w", r.err)
			}

			if streamIdleTimer != nil {
				streamIdleTimer.Reset(time.Duration(rc.streamIdleTimeoutSec) * time.Second)
			}

			data, err := rc.transformStreamData(ctx, r.data)
			if err != nil || len(data) == 0 {
				continue
			}

			if firstToken {
				firstToken = false
				rc.metrics.SetFirstTokenTime(time.Now())
				if firstTokenTimer != nil {
					firstTokenTimer.Stop()
					firstTokenC = nil
				}
			} else if firstTokenTimer != nil {
				firstTokenTimer.Reset(time.Duration(rc.firstTokenTimeOutSec) * time.Second)
			}

			if streamNoOutputTimer != nil {
				streamNoOutputTimer.Reset(time.Duration(rc.streamNoOutputTimeoutSec) * time.Second)
			}

			rc.c.Writer.Write(data)
			rc.c.Writer.Flush()
		}
	}
}

// transformStreamData 转换流式数据
func (rc *relayContext) transformStreamData(ctx context.Context, data string) ([]byte, error) {
	// 上游格式 → 内部格式
	internalStream, err := rc.outAdapter.TransformStream(ctx, []byte(data))
	if err != nil {
		log.Warnf("failed to transform stream: %v", err)
		return nil, err
	}
	if internalStream == nil {
		return nil, nil
	}

	// 内部格式 → 入站格式
	inStream, err := rc.inAdapter.TransformStream(ctx, internalStream)
	if err != nil {
		log.Warnf("failed to transform stream: %v", err)
		return nil, err
	}

	return inStream, nil
}

// handleResponse 处理非流式响应
func (rc *relayContext) handleResponse(ctx context.Context, response *http.Response) error {
	// Apply non-stream request timeout if configured
	if rc.nonStreamRequestTimeoutSec > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(rc.nonStreamRequestTimeoutSec)*time.Second)
		defer cancel()
	}

	// 上游格式 → 内部格式
	internalResponse, err := rc.outAdapter.TransformResponse(ctx, response)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("non-stream request timeout (%ds)", rc.nonStreamRequestTimeoutSec)
		}
		if ctx.Err() == context.Canceled {
			return fmt.Errorf("request canceled by client")
		}
		log.Warnf("failed to transform response: %v", err)
		return fmt.Errorf("failed to transform outbound response: %w", err)
	}

	// 内部格式 → 入站格式
	inResponse, err := rc.inAdapter.TransformResponse(ctx, internalResponse)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("non-stream request timeout (%ds)", rc.nonStreamRequestTimeoutSec)
		}
		if ctx.Err() == context.Canceled {
			return fmt.Errorf("request canceled by client")
		}
		log.Warnf("failed to transform response: %v", err)
		return fmt.Errorf("failed to transform inbound response: %w", err)
	}

	rc.c.Data(http.StatusOK, "application/json", inResponse)
	return nil
}

// collectResponse 收集响应信息
func (rc *relayContext) collectResponse() {
	internalResponse, err := rc.inAdapter.GetInternalResponse(rc.c.Request.Context())
	if err != nil || internalResponse == nil {
		return
	}

	// 设置响应内容
	rc.metrics.SetInternalResponse(internalResponse)
}
