package relay

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/bestruirui/octopus/internal/client"
	"github.com/bestruirui/octopus/internal/helper"
	dbmodel "github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/oauth"
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
	internalRequest, inAdapter, body, err := parseRequest(inboundType, c)
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

	requestModel := internalRequest.Model
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
	metrics := NewRelayMetrics(apiKeyID, requestModel, internalRequest)
	metrics.SetRawRequest(normalizeLogJSONPayload(body))

	// 请求级上下文
	req := &relayRequest{
		c:               c,
		inAdapter:       inAdapter,
		internalRequest: internalRequest,
		metrics:         metrics,
		apiKeyID:        apiKeyID,
		requestModel:    requestModel,
		iter:            iter,
		rawBody:         body,
		inboundType:     inboundType,
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

		// 获取通道或 OAuth Provider
		var channel *dbmodel.Channel
		var oauthProvider *dbmodel.OAuthProvider
		var channelType int
		var channelName string
		var baseUrl string

		if item.ChannelID > 0 {
			// 现有 Channel 逻辑
			var err error
			channel, err = op.ChannelGet(item.ChannelID, c.Request.Context())
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
			channelType = int(channel.Type)
			channelName = channel.Name
			baseUrl = channel.GetBaseUrl()
		} else if item.ChannelID < 0 {
			// OAuth Provider 逻辑
			var err error
			oauthProvider, err = op.OAuthProviderGet(-item.ChannelID, c.Request.Context())
			if err != nil {
				log.Warnf("failed to get oauth provider %d: %v", -item.ChannelID, err)
				iter.Skip(item.ChannelID, 0, "", 0, "", fmt.Sprintf("oauth provider not found: %v", err))
				lastErr = err
				continue
			}
			if oauthProvider.Status != 1 {
				iter.Skip(item.ChannelID, 0, oauthProvider.Name, 0, "", "oauth provider disabled")
				continue
			}
			if oauthProvider.APIKey == "" {
				iter.Skip(item.ChannelID, 0, oauthProvider.Name, 0, "", "oauth provider has no api key")
				continue
			}
			channelType = int(outbound.OutboundTypeOpenAIChat)
			channelName = oauthProvider.Name
			baseUrl = oauthProvider.GetBaseURL()
		} else {
			// channel_id = 0 is invalid
			iter.Skip(item.ChannelID, 0, "", 0, "", "invalid channel_id: 0")
			continue
		}

		var usedKey dbmodel.ChannelKey
		if item.ChannelID > 0 {
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
		} else if item.ChannelID < 0 {
			// OAuth Provider: 检查是否需要刷新 API Key
			manager := oauth.GetManager()
			if manager.ShouldRefresh(oauthProvider) {
				if err := manager.RefreshAPIKey(c.Request.Context(), oauthProvider); err != nil {
					errMsg := fmt.Sprintf("OAuth Provider '%s' API Key refresh failed: %s. Please update the cookie in OAuth Provider settings.", oauthProvider.Name, err.Error())
					iter.Skip(item.ChannelID, 0, oauthProvider.Name, 0, "", errMsg)
					continue
				}
			}
			usedKey = dbmodel.ChannelKey{
				ChannelKey: oauthProvider.APIKey,
				Enabled:    true,
			}
		}
		if usedKey.ChannelKey == "" {
			iter.Skip(item.ChannelID, 0, channelName, channelType, "", "no available key")
			continue
		}

		apiKeySuffix := ""
		if len(usedKey.ChannelKey) > 4 {
			apiKeySuffix = usedKey.ChannelKey[len(usedKey.ChannelKey)-4:]
		}

		// 熔断检查
		if iter.SkipCircuitBreak(item.ChannelID, usedKey.ID, channelName, channelType, apiKeySuffix) {
			continue
		}

		// 出站适配器
		outAdapter := outbound.Get(outbound.OutboundType(channelType))
		if outAdapter == nil {
			iter.Skip(item.ChannelID, usedKey.ID, channelName, channelType, apiKeySuffix, fmt.Sprintf("unsupported channel type: %d", channelType))
			continue
		}

		// 类型兼容性检查
		if internalRequest.IsEmbeddingRequest() && !outbound.IsEmbeddingChannelType(outbound.OutboundType(channelType)) {
			iter.Skip(item.ChannelID, usedKey.ID, channelName, channelType, apiKeySuffix, "channel type not compatible with embedding request")
			continue
		}
		if internalRequest.IsChatRequest() && !outbound.IsChatChannelType(outbound.OutboundType(channelType)) {
			iter.Skip(item.ChannelID, usedKey.ID, channelName, channelType, apiKeySuffix, "channel type not compatible with chat request")
			continue
		}

		// 设置实际模型
		internalRequest.Model = item.ModelName

		log.Infof("request model %s, mode: %d, forwarding to channel: %s model: %s (attempt %d/%d, sticky=%t)",
			requestModel, group.Mode, channelName, item.ModelName,
			iter.Index()+1, iter.Len(), iter.IsSticky())

		// 检测是否可以走 passthrough 路径
		isPassthrough := false
		if inbound.MatchesOutbound(inboundType, outbound.OutboundType(channelType)) {
			if _, ok := outAdapter.(model.PassthroughOutbound); ok {
				isPassthrough = true
				log.Infof("passthrough mode enabled for channel %s (inbound=%d, outbound=%d)", channelName, inboundType, channelType)
			}
		}

		// 构造尝试级上下文
		ra := &relayAttempt{
			relayRequest:         req,
			outAdapter:           outAdapter,
			channel:              channel,
			oauthProvider:        oauthProvider,
			channelID:            item.ChannelID,
			channelName:          channelName,
			channelType:          channelType,
			baseUrl:              baseUrl,
			usedKey:              usedKey,
			firstTokenTimeOutSec: group.FirstTokenTimeOut,
			isPassthrough:        isPassthrough,
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

	// metrics.CalcTokensFromRequest()
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
		passthrough := ra.outAdapter.(model.PassthroughOutbound)
		statusCode, fwdErr = ra.forwardPassthrough(passthrough)
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
		// 仅对 Channel 更新 key 状态（OAuth Provider 没有 key 表）
		if ra.channelID > 0 {
			op.ChannelKeyUpdate(ra.usedKey)
		}

		span.End(dbmodel.AttemptSuccess, statusCode, "")

		// Channel 维度统计（仅对 Channel）
		if ra.channelID > 0 {
			op.StatsChannelUpdate(ra.channelID, dbmodel.StatsMetrics{
				WaitTime:       span.Duration().Milliseconds(),
				RequestSuccess: 1,
			})
		}

		// 熔断器：记录成功
		balancer.RecordSuccess(ra.channelID, ra.usedKey.ID, ra.internalRequest.Model)
		// 会话保持：更新粘性记录
		balancer.SetSticky(ra.apiKeyID, ra.requestModel, ra.channelID, ra.usedKey.ID)

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
	// 仅对 Channel 更新 key 状态
	if ra.channelID > 0 {
		op.ChannelKeyUpdate(ra.usedKey)
	}
	span.End(dbmodel.AttemptFailed, statusCode, fwdErr.Error())

	// Channel 维度统计（仅对 Channel）
	if ra.channelID > 0 {
		op.StatsChannelUpdate(ra.channelID, dbmodel.StatsMetrics{
			WaitTime:      span.Duration().Milliseconds(),
			RequestFailed: 1,
		})
	}

	// 熔断器：记录失败
	balancer.RecordFailure(ra.channelID, ra.usedKey.ID, ra.internalRequest.Model)

	written := ra.c.Writer.Written()
	if written {
		ra.collectResponse()
	}
	return attemptResult{
		Success: false,
		Written: written,
		Err:     fmt.Errorf("channel %s failed: %v", ra.channelName, fwdErr),
	}
}

// parseRequest 解析并验证入站请求
func parseRequest(inboundType inbound.InboundType, c *gin.Context) (*model.InternalLLMRequest, model.Inbound, []byte, error) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return nil, nil, nil, err
	}

	inAdapter := inbound.Get(inboundType)
	internalRequest, err := inAdapter.TransformRequest(c.Request.Context(), body)
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return nil, nil, nil, err
	}

	// Pass through the original query parameters
	internalRequest.Query = c.Request.URL.Query()

	if err := internalRequest.Validate(); err != nil {
		resp.Error(c, http.StatusBadRequest, err.Error())
		return nil, nil, nil, err
	}

	return internalRequest, inAdapter, body, nil
}

// forward 转发请求到上游服务
func (ra *relayAttempt) forward() (int, error) {
	ctx := ra.c.Request.Context()

	// 构建出站请求
	outboundRequest, err := ra.outAdapter.TransformRequest(
		ctx,
		ra.internalRequest,
		ra.baseUrl,
		ra.usedKey.ChannelKey,
	)
	if err != nil {
		log.Warnf("failed to create request: %v", err)
		return 0, fmt.Errorf("failed to create request: %w", err)
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
		// 检查是否是 OAuth Provider 的 Invalid apiKey 错误
		if ra.channelID < 0 && isInvalidAPIKeyError(body) {
			return 0, fmt.Errorf("OAuth Provider '%s' returned Invalid apiKey. Please update the cookie in OAuth Provider settings. Response: %s", ra.channelName, string(body))
		}
		return 0, fmt.Errorf("upstream error: %d: %s", response.StatusCode, string(body))
	}

	// 处理响应
	if ra.internalRequest.Stream != nil && *ra.internalRequest.Stream {
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
	// 仅 Channel 有自定义头
	if ra.channel != nil && len(ra.channel.CustomHeader) > 0 {
		for _, header := range ra.channel.CustomHeader {
			outboundRequest.Header.Set(header.HeaderKey, header.HeaderValue)
		}
	}
}

// sendRequest 发送 HTTP 请求
func (ra *relayAttempt) sendRequest(req *http.Request) (*http.Response, error) {
	var httpClient *http.Client
	var err error

	// 仅 Channel 有代理配置，OAuth Provider 使用默认客户端
	if ra.channel != nil {
		httpClient, err = helper.ChannelHttpClient(ra.channel)
		if err != nil {
			log.Warnf("failed to get http client: %v", err)
			return nil, err
		}
	} else {
		// OAuth Provider 使用默认 HTTP 客户端（不使用代理）
		httpClient, err = client.GetHTTPClientSystemProxy(false)
		if err != nil {
			log.Warnf("failed to get default http client: %v", err)
			return nil, err
		}
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
			ra.captureFinalStreamResponse(ctx)
			return fmt.Errorf("first token timeout (%ds)", ra.firstTokenTimeOutSec)
		case r, ok := <-results:
			if !ok {
				ra.captureFinalStreamResponse(ctx)
				log.Infof("stream end")
				return nil
			}
			if r.err != nil {
				log.Warnf("failed to read event: %v", r.err)
				return fmt.Errorf("failed to read stream event: %w", r.err)
			}

			// 记录原始流数据
			ra.metrics.AppendRawResponse(r.data)

			data, err := ra.transformStreamData(ctx, r.data)
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
func (ra *relayAttempt) transformStreamData(ctx context.Context, data string) ([]byte, error) {
	internalStream, err := ra.outAdapter.TransformStream(ctx, []byte(data))
	if err != nil {
		log.Warnf("failed to transform stream: %v", err)
		return nil, err
	}
	if internalStream == nil {
		return nil, nil
	}

	inStream, err := ra.inAdapter.TransformStream(ctx, internalStream)
	if err != nil {
		log.Warnf("failed to transform stream: %v", err)
		return nil, err
	}

	return inStream, nil
}

// handleResponse 处理非流式响应
func (ra *relayAttempt) handleResponse(ctx context.Context, response *http.Response) error {
	// 读取并保存原始响应
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}
	ra.metrics.AppendRawResponse(string(body))

	// 重新设置 Response Body 供 TransformResponse 使用
	response.Body = io.NopCloser(bytes.NewReader(body))

	internalResponse, err := ra.outAdapter.TransformResponse(ctx, response)
	if err != nil {
		log.Warnf("failed to transform response: %v", err)
		return fmt.Errorf("failed to transform outbound response: %w", err)
	}

	inResponse, err := ra.inAdapter.TransformResponse(ctx, internalResponse)
	if err != nil {
		log.Warnf("failed to transform response: %v", err)
		// Feature 009: Handle Context Canceled
		if errors.Is(err, context.Canceled) {
			return fmt.Errorf("request canceled by client: %w", context.Canceled)
		}
		return fmt.Errorf("failed to transform inbound response: %w", err)
	}

	ra.c.Data(http.StatusOK, "application/json", inResponse)
	ra.metrics.SetFinalResponse(string(inResponse))
	return nil
}

// collectResponse 收集响应信息
func (ra *relayAttempt) collectResponse() {
	if ra.isPassthrough {
		// Passthrough 模式下，metrics 已经在 handlePassthroughResponse/handlePassthroughStreamResponse 中设置
		return
	}

	internalResponse, err := ra.inAdapter.GetInternalResponse(ra.c.Request.Context())
	if err != nil || internalResponse == nil {
		return
	}

	ra.metrics.SetInternalResponse(internalResponse, ra.internalRequest.Model)
	if ra.metrics.FinalResponse == "" {
		if finalResp, transformErr := ra.inAdapter.TransformResponse(ra.c.Request.Context(), internalResponse); transformErr == nil {
			ra.metrics.SetFinalResponse(string(finalResp))
		}
	}
}

func (ra *relayAttempt) captureFinalStreamResponse(ctx context.Context) {
	internalResponse, err := ra.inAdapter.GetInternalResponse(ctx)
	if err != nil || internalResponse == nil {
		return
	}
	if finalResp, transformErr := ra.inAdapter.TransformResponse(ctx, internalResponse); transformErr == nil {
		ra.metrics.SetFinalResponse(string(finalResp))
	}
}

func normalizeLogJSONPayload(payload []byte) string {
	if len(payload) == 0 {
		return ""
	}
	var jsonData any
	if err := json.Unmarshal(payload, &jsonData); err != nil {
		return string(payload)
	}
	normalized, err := json.Marshal(jsonData)
	if err != nil {
		return string(payload)
	}
	return string(normalized)
}

func errorResponseBody(code int, msg string) string {
	b, err := json.Marshal(resp.ResponseStruct{
		Code:    code,
		Message: msg,
	})
	if err != nil {
		return fmt.Sprintf(`{"code":%d,"message":%q}`, code, msg)
	}
	return string(b)
}
