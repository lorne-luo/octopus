package relay

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/bestruirui/octopus/internal/helper"
	dbmodel "github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/op"
	"github.com/bestruirui/octopus/internal/relay/balancer"
	"github.com/bestruirui/octopus/internal/server/resp"
	"github.com/bestruirui/octopus/internal/transformer2/adapter"
	"github.com/bestruirui/octopus/internal/transformer2/canonical"
	"github.com/bestruirui/octopus/internal/utils/log"
	"github.com/gin-gonic/gin"
	"github.com/tmaxmax/go-sse"
)

// Handler 处理入站请求并转发到上游服务
func Handler(clientType adapter.ClientType, c *gin.Context) {
	// 解析请求
	canonicalReq, clientAdapter, body, err := parseRequest(clientType, c)
	if err != nil {
		return
	}
	supportedModels := c.GetString("supported_models")
	if supportedModels != "" {
		supportedModelsArray := strings.Split(supportedModels, ",")
		if !slices.Contains(supportedModelsArray, canonicalReq.Model) {
			resp.Error(c, http.StatusBadRequest, "model not supported")
			return
		}
	}

	requestModel := canonicalReq.Model
	apiKeyID := c.GetInt("api_key_id")

	// 获取通道分组
	group, err := op.GroupGetMap(requestModel, c.Request.Context())
	if err != nil {
		resp.Error(c, http.StatusNotFound, "model not found")
		return
	}

	// 创建迭代器（策略排序 + 粘性优先）
	iter := balancer.NewIterator(group, apiKeyID, requestModel)
	if iter.Len() == 0 {
		resp.Error(c, http.StatusServiceUnavailable, "no available channel")
		return
	}

	// 初始化 Metrics
	metrics := NewRelayMetrics(apiKeyID, requestModel, canonicalReq)
	metrics.SetRawRequest(string(body))

	// 请求级上下文
	req := &relayRequest{
		c:             c,
		clientAdapter: clientAdapter,
		canonicalReq:  canonicalReq,
		metrics:       metrics,
		apiKeyID:      apiKeyID,
		requestModel:  requestModel,
		iter:          iter,
		rawBody:       body,
	}

	var lastErr error

	for iter.Next() {
		select {
		case <-c.Request.Context().Done():
			log.Infof("request context canceled, stopping retry")
			// 确保有 Token 统计
			metrics.CalcTokensFromRequest()
			metrics.Save(c.Request.Context(), false, context.Canceled, iter.Attempts())
			return
		default:
		}

		item := iter.Item()

		// 获取通道
		channel, err := op.ChannelGet(item.ChannelID, c.Request.Context())
		if err != nil {
			log.Warnf("failed to get channel %d: %v", item.ChannelID, err)
			iter.Skip(item.ChannelID, 0, fmt.Sprintf("channel_%d", item.ChannelID), 0, "", fmt.Sprintf("channel not found: %v", err))
			lastErr = err
			continue
		}
		if !channel.Enabled {
			iter.Skip(channel.ID, 0, channel.Name, int(channel.Type), "", "channel disabled")
			continue
		}

		var usedKey dbmodel.ChannelKey
		if channel.UseOAuth {
			apiKey, err := GetChannelKey(c.Request.Context(), channel)
			if err != nil {
				iter.Skip(channel.ID, 0, channel.Name, int(channel.Type), "", "oauth failed: "+err.Error())
				continue
			}
			usedKey = dbmodel.ChannelKey{
				ChannelID:  channel.ID,
				ChannelKey: apiKey,
				Enabled:    true,
			}
		} else {
			usedKey = channel.GetChannelKey()
		}
		if usedKey.ChannelKey == "" {
			iter.Skip(channel.ID, 0, channel.Name, int(channel.Type), "", "no available key")
			continue
		}

		apiKeySuffix := ""
		if len(usedKey.ChannelKey) > 4 {
			apiKeySuffix = usedKey.ChannelKey[len(usedKey.ChannelKey)-4:]
		}

		// 熔断检查
		if iter.SkipCircuitBreak(channel.ID, usedKey.ID, channel.Name, int(channel.Type), apiKeySuffix) {
			continue
		}

		// 出站适配器
		providerAdapter := adapter.GetProvider(adapter.ProviderType(channel.Type))
		if providerAdapter == nil {
			iter.Skip(channel.ID, usedKey.ID, channel.Name, int(channel.Type), apiKeySuffix, fmt.Sprintf("unsupported channel type: %d", channel.Type))
			continue
		}

		// 类型兼容性检查
		if canonicalReq.Kind == canonical.KindEmbedding && !adapter.IsEmbeddingProvider(adapter.ProviderType(channel.Type)) {
			iter.Skip(channel.ID, usedKey.ID, channel.Name, int(channel.Type), apiKeySuffix, "channel type not compatible with embedding request")
			continue
		}
		if canonicalReq.Kind == canonical.KindChat && !adapter.IsChatProvider(adapter.ProviderType(channel.Type)) {
			iter.Skip(channel.ID, usedKey.ID, channel.Name, int(channel.Type), apiKeySuffix, "channel type not compatible with chat request")
			continue
		}

		// 设置实际模型
		canonicalReq.Model = item.ModelName

		log.Infof("request model %s, mode: %d, forwarding to channel: %s model: %s (attempt %d/%d, sticky=%t)",
			requestModel, group.Mode, channel.Name, item.ModelName,
			iter.Index()+1, iter.Len(), iter.IsSticky())

		// 检测是否可以走 passthrough 路径
		isPassthrough := false
		if pp, ok := providerAdapter.(adapter.PassthroughProvider); ok && pp != nil {
			// Only enable passthrough when the source format matches the provider type
			if matchesProvider(canonicalReq.SourceFormat, adapter.ProviderType(channel.Type)) {
				isPassthrough = true
				log.Infof("passthrough mode enabled for channel %s", channel.Name)
			}
		}

		// 检测是否为原始流模式（非SSE）
		isRawStream := false
		if rs, ok := providerAdapter.(adapter.RawStreamProvider); ok {
			isRawStream = rs.IsRawStream()
			if isRawStream {
				log.Infof("raw stream mode enabled for channel %s", channel.Name)
			}
		}

		// 构造尝试级上下文
		ra := &relayAttempt{
			relayRequest:         req,
			providerAdapter:      providerAdapter,
			channel:              channel,
			channelID:            channel.ID,
			channelName:          channel.Name,
			channelType:          int(channel.Type),
			baseUrl:              channel.GetBaseUrl(),
			usedKey:              usedKey,
			firstTokenTimeOutSec: group.FirstTokenTimeOut,
			isPassthrough:        isPassthrough,
			isRawStream:          isRawStream,
		}

		result := ra.attempt()
		if result.Success {
			metrics.Save(c.Request.Context(), true, nil, iter.Attempts())
			return
		}
		if result.Written {
			// 如果响应已经写入，但发生错误，确保统计了 Token
			if metrics.Stats.InputToken == 0 {
				metrics.CalcTokensFromRequest()
			}
			metrics.Save(c.Request.Context(), false, result.Err, iter.Attempts())
			return
		}
		lastErr = result.Err
	}

	// 所有通道都失败
	metrics.SetFinalResponse(errorResponseBody(http.StatusBadGateway, "all channels failed"))
	metrics.Save(c.Request.Context(), false, lastErr, iter.Attempts())
	resp.Error(c, http.StatusBadGateway, "all channels failed")
}

// attempt 统一管理一次通道尝试的完整生命周期
func (ra *relayAttempt) attempt() attemptResult {
	apiKeySuffix := ""
	if len(ra.usedKey.ChannelKey) > 4 {
		apiKeySuffix = ra.usedKey.ChannelKey[len(ra.usedKey.ChannelKey)-4:]
	}
	span := ra.iter.StartAttempt(ra.channelID, ra.usedKey.ID, ra.channelName, ra.channelType, apiKeySuffix)

	// 转发请求
	var statusCode int
	var fwdErr error
	if ra.isPassthrough {
		if pp, ok := ra.providerAdapter.(adapter.PassthroughProvider); ok {
			statusCode, fwdErr = ra.forwardPassthrough(pp)
		} else {
			statusCode, fwdErr = ra.forward()
		}
	} else {
		statusCode, fwdErr = ra.forward()
	}

	// 更新 channel key 状态
	ra.usedKey.StatusCode = statusCode
	ra.usedKey.LastUseTimeStamp = time.Now().Unix()

	if fwdErr == nil {
		// ====== 成功 ======
		ra.collectResponse()
		ra.usedKey.TotalCost += ra.metrics.Stats.InputCost + ra.metrics.Stats.OutputCost
		ra.usedKey.TotalToken += ra.metrics.Stats.InputToken + ra.metrics.Stats.OutputToken // Feature 020
		op.ChannelKeyUpdate(ra.usedKey)

		span.End(dbmodel.AttemptSuccess, statusCode, "")

		// Channel 维度统计
		op.StatsChannelUpdate(ra.channel.ID, dbmodel.StatsMetrics{
			WaitTime:       span.Duration().Milliseconds(),
			RequestSuccess: 1,
		})

		// 熔断器：记录成功
		balancer.RecordSuccess(ra.channel.ID, ra.usedKey.ID, ra.canonicalReq.Model)
		// 会话保持：更新粘性记录
		balancer.SetSticky(ra.apiKeyID, ra.requestModel, ra.channel.ID, ra.usedKey.ID)

		// 成功提权：如果是 SuccessBoost 模式，成功后提升该项优先级
		if ra.iter.Mode == dbmodel.GroupModeSuccessBoost {
			item := ra.iter.Item()
			groupID := ra.iter.GroupID
			go func() {
				// 使用背景上下文避免请求结束导致数据库操作被取消
				if err := op.GroupItemPromote(groupID, item.ChannelID, item.ModelName, context.Background()); err != nil {
					log.Warnf("failed to promote group item %d/%s: %v", item.ChannelID, item.ModelName, err)
				}
			}()
		}

		return attemptResult{Success: true}
	}

	// ====== 失败 ======
	op.ChannelKeyUpdate(ra.usedKey)
	span.End(dbmodel.AttemptFailed, statusCode, fwdErr.Error())

	// Channel 维度统计
	op.StatsChannelUpdate(ra.channel.ID, dbmodel.StatsMetrics{
		WaitTime:      span.Duration().Milliseconds(),
		RequestFailed: 1,
	})

	// 熔断器：记录失败
	balancer.RecordFailure(ra.channel.ID, ra.usedKey.ID, ra.canonicalReq.Model)

	written := ra.c.Writer.Written()
	if written {
		ra.collectResponse()
	}
	return attemptResult{
		Success: false,
		Written: written,
		Err:     fmt.Errorf("channel %s failed: %v", ra.channel.Name, fwdErr),
	}
}

// parseRequest 解析并验证入站请求
func parseRequest(clientType adapter.ClientType, c *gin.Context) (*canonical.Request, adapter.ClientAdapter, []byte, error) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return nil, nil, nil, err
	}

	clientAdapter := adapter.GetClient(clientType)
	if clientAdapter == nil {
		resp.Error(c, http.StatusBadRequest, fmt.Sprintf("unsupported client type: %d", clientType))
		return nil, nil, nil, fmt.Errorf("unsupported client type: %d", clientType)
	}

	canonicalReq, err := clientAdapter.ParseRequest(c.Request.Context(), body, c.Request.Header)
	if err != nil {
		resp.Error(c, http.StatusBadRequest, err.Error())
		return nil, nil, nil, err
	}

	// Pass through the original query parameters
	canonicalReq.Query = c.Request.URL.Query()

	return canonicalReq, clientAdapter, body, nil
}

// forward 转发请求到上游服务
func (ra *relayAttempt) forward() (int, error) {
	ctx := ra.c.Request.Context()

	// 构建出站请求
	outboundRequest, err := ra.providerAdapter.BuildRequest(
		ctx,
		ra.canonicalReq,
		ra.channel.GetBaseUrl(),
		ra.usedKey.ChannelKey,
	)
	if err != nil {
		log.Warnf("failed to create request: %v", err)
		return 0, fmt.Errorf("failed to create request: %w", err)
	}

	// 记录转换后的请求 JSON
	if outboundRequest.Body != nil {
		bodyBytes, readErr := io.ReadAll(outboundRequest.Body)
		if readErr == nil {
			outboundRequest.Body = io.NopCloser(bytes.NewReader(bodyBytes))
			log.Debugf("converted request to channel %s: %s", ra.channel.Name, string(bodyBytes))
		}
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
		body, err := io.ReadAll(response.Body)
		if err != nil {
			return 0, fmt.Errorf("failed to read response body: %w", err)
		}
		// 记录原始错误响应
		ra.metrics.AppendRawResponse(string(body))
		return 0, fmt.Errorf("upstream error: %d: %s", response.StatusCode, string(body))
	}

	// 处理响应
	if ra.canonicalReq.Stream {
		if err := ra.handleStreamResponse(ctx, response); err != nil {
			return 0, err
		}
		return response.StatusCode, nil
	}
	if err := ra.handleResponse(ctx, response); err != nil {
		return 0, err
	}
	return response.StatusCode, nil
}

// copyHeaders 复制请求头，过滤 hop-by-hop 头
func (ra *relayAttempt) copyHeaders(outboundRequest *http.Request) {
	for key, values := range ra.c.Request.Header {
		if hopByHopHeaders[strings.ToLower(key)] {
			continue
		}
		for _, value := range values {
			outboundRequest.Header.Set(key, value)
		}
	}
	if len(ra.channel.CustomHeader) > 0 {
		for _, header := range ra.channel.CustomHeader {
			outboundRequest.Header.Set(header.HeaderKey, header.HeaderValue)
		}
	}
}

// sendRequest 发送 HTTP 请求
func (ra *relayAttempt) sendRequest(req *http.Request) (*http.Response, error) {
	httpClient, err := helper.ChannelHttpClient(ra.channel)
	if err != nil {
		log.Warnf("failed to get http client: %v", err)
		return nil, err
	}

	response, err := httpClient.Do(req)
	if err != nil {
		log.Warnf("failed to send request: %v", err)
		return nil, err
	}

	return response, nil
}

// handleStreamResponse 处理流式响应
func (ra *relayAttempt) handleStreamResponse(ctx context.Context, response *http.Response) error {
	if ct := response.Header.Get("Content-Type"); ct != "" && !strings.Contains(strings.ToLower(ct), "text/event-stream") {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 16*1024))
		ra.metrics.AppendRawResponse(string(body))
		return fmt.Errorf("upstream returned non-SSE content-type %q for stream request: %s", ct, string(body))
	}

	// 设置 SSE 响应头
	ra.c.Header("Content-Type", "text/event-stream")
	ra.c.Header("Cache-Control", "no-cache")
	ra.c.Header("Connection", "keep-alive")
	ra.c.Header("X-Accel-Buffering", "no")

	firstToken := true

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
			log.Infof("client disconnected, stopping stream")
			return nil
		case <-firstTokenC:
			log.Warnf("first token timeout (%ds), switching channel", ra.firstTokenTimeOutSec)
			_ = response.Body.Close()
			return fmt.Errorf("first token timeout (%ds)", ra.firstTokenTimeOutSec)
		case r, ok := <-results:
			if !ok {
				log.Infof("stream end")
				return nil
			}
			if r.err != nil {
				log.Warnf("failed to read event: %v", r.err)
				return fmt.Errorf("failed to read stream event: %w", r.err)
			}

			// 记录原始流数据
			ra.metrics.AppendRawResponse(r.data)

			data, err := ra.transformStreamData(ctx, []byte(r.data))
			if err != nil || len(data) == 0 {
				continue
			}
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

			ra.c.Writer.Write(data)
			ra.c.Writer.Flush()
		}
	}
}

// transformStreamData 转换流式数据
func (ra *relayAttempt) transformStreamData(ctx context.Context, data []byte) ([]byte, error) {
	chunk, err := ra.providerAdapter.ParseStreamChunk(ctx, data)
	if err != nil {
		log.Warnf("failed to parse stream chunk: %v", err)
		return nil, err
	}
	if chunk == nil {
		return nil, nil
	}

	// Check for stream termination
	if chunk.Done {
		return nil, nil
	}

	clientData, err := ra.clientAdapter.FormatStreamChunk(ctx, chunk)
	if err != nil {
		log.Warnf("failed to format stream chunk: %v", err)
		return nil, err
	}

	return clientData, nil
}

// handleResponse 处理非流式响应
func (ra *relayAttempt) handleResponse(ctx context.Context, response *http.Response) error {
	// 读取并保存原始响应
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}
	ra.metrics.AppendRawResponse(string(body))

	// 重新设置 Response Body 供 ParseResponse 使用
	response.Body = io.NopCloser(bytes.NewReader(body))

	canonicalResp, err := ra.providerAdapter.ParseResponse(ctx, response)
	if err != nil {
		log.Warnf("failed to parse response: %v", err)
		return fmt.Errorf("failed to parse provider response: %w", err)
	}

	// Check for error in canonical response
	if canonicalResp.Error != nil {
		errorBytes, err := ra.clientAdapter.FormatError(ctx, canonicalResp.Error)
		if err != nil {
			log.Warnf("failed to format error: %v", err)
			return fmt.Errorf("provider error: %s", canonicalResp.Error.Message)
		}
		ra.c.Data(canonicalResp.Error.StatusCode, "application/json", errorBytes)
		return nil
	}

	clientResp, err := ra.clientAdapter.FormatResponse(ctx, canonicalResp)
	if err != nil {
		log.Warnf("failed to format response: %v", err)
		// Feature 009: Handle Context Canceled
		if errors.Is(err, context.Canceled) {
			return fmt.Errorf("request canceled by client: %w", context.Canceled)
		}
		return fmt.Errorf("failed to format client response: %w", err)
	}

	ra.c.Data(http.StatusOK, "application/json", clientResp)
	return nil
}

// collectResponse 收集响应信息
func (ra *relayAttempt) collectResponse() {
	canonicalResp, err := ra.clientAdapter.AggregateStream(ra.c.Request.Context())
	if err != nil || canonicalResp == nil {
		return
	}

	ra.metrics.SetCanonicalResponse(canonicalResp, ra.canonicalReq.Model)
}
