package relay

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/bestruirui/octopus/internal/helper"
	"github.com/bestruirui/octopus/internal/transformer2/adapter"
	"github.com/bestruirui/octopus/internal/transformer2/canonical"
	"github.com/bestruirui/octopus/internal/utils/log"
	"github.com/tmaxmax/go-sse"
)

// matchesProvider checks if the inbound API format matches the outbound provider type.
// This determines if passthrough mode can be used (skip canonical conversion).
func matchesProvider(format canonical.APIFormat, providerType adapter.ProviderType) bool {
	switch format {
	case canonical.FormatOpenAIChat:
		return providerType == adapter.ProviderOpenAIChat || providerType == adapter.ProviderVolcengine
	case canonical.FormatOpenAIResponse:
		return providerType == adapter.ProviderOpenAIResponse
	case canonical.FormatAnthropic:
		return providerType == adapter.ProviderAnthropic
	default:
		return false
	}
}

// errorResponseBody creates a JSON error response body string for logging.
func errorResponseBody(statusCode int, message string) string {
	body := map[string]interface{}{
		"error": map[string]interface{}{
			"message": message,
			"type":    "relay_error",
			"code":    statusCode,
		},
	}
	b, _ := json.Marshal(body)
	return string(b)
}

// forwardPassthrough handles passthrough forwarding where the raw request body
// is sent directly to the upstream provider without canonical conversion.
func (ra *relayAttempt) forwardPassthrough(pp adapter.PassthroughProvider) (int, error) {
	ctx := ra.c.Request.Context()

	// Replace model name in raw body
	rawBody := replaceModelInBody(ra.rawBody, ra.canonicalReq.Model)

	// Build passthrough request
	outboundRequest, err := pp.BuildPassthroughRequest(
		ctx,
		rawBody,
		ra.canonicalReq.Stream,
		ra.baseUrl,
		ra.usedKey.ChannelKey,
		ra.canonicalReq.Query,
	)
	if err != nil {
		log.Warnf("failed to build passthrough request: %v", err)
		return 0, fmt.Errorf("failed to build passthrough request: %w", err)
	}

	// Copy relevant headers
	ra.copyHeaders(outboundRequest)

	// Send request
	httpClient, err := helper.ChannelHttpClient(ra.channel)
	if err != nil {
		return 0, fmt.Errorf("failed to get http client: %w", err)
	}

	response, err := httpClient.Do(outboundRequest)
	if err != nil {
		return 0, fmt.Errorf("failed to send passthrough request: %w", err)
	}
	defer response.Body.Close()

	// Check response status
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, _ := io.ReadAll(response.Body)
		ra.metrics.AppendRawResponse(string(body))
		return response.StatusCode, fmt.Errorf("upstream error: %d: %s", response.StatusCode, string(body))
	}

	// Handle response
	if ra.canonicalReq.Stream {
		if ra.isRawStream {
			return response.StatusCode, ra.handleRawStreamPassthrough(ctx, response)
		}
		return response.StatusCode, ra.handleSSEPassthrough(ctx, response)
	}

	// Non-stream: read body and write directly
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return 0, fmt.Errorf("failed to read passthrough response: %w", err)
	}
	ra.metrics.SetFinalResponse(string(body))
	ra.c.Data(http.StatusOK, "application/json", body)
	return response.StatusCode, nil
}

// handleSSEPassthrough forwards SSE events directly without format conversion.
func (ra *relayAttempt) handleSSEPassthrough(ctx context.Context, response *http.Response) error {
	// Set SSE response headers
	ra.c.Header("Content-Type", "text/event-stream")
	ra.c.Header("Cache-Control", "no-cache")
	ra.c.Header("Connection", "keep-alive")
	ra.c.Header("X-Accel-Buffering", "no")

	firstToken := true
	var rawResponse strings.Builder

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
	if ra.firstTokenTimeOutSec > 0 {
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
			return fmt.Errorf("first token timeout (%ds)", ra.firstTokenTimeOutSec)
		case r, ok := <-results:
			if !ok {
				// Stream ended
				ra.metrics.SetFinalResponse(rawResponse.String())
				log.Infof("passthrough stream end")
				return nil
			}
			if r.err != nil {
				log.Warnf("failed to read event: %v", r.err)
				return fmt.Errorf("failed to read stream event: %w", r.err)
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

			// Write raw SSE event directly
			sseData := fmt.Sprintf("data: %s\n\n", r.data)
			ra.c.Writer.Write([]byte(sseData))
			ra.c.Writer.Flush()
			rawResponse.WriteString(sseData)
		}
	}
}

// handleRawStreamPassthrough handles raw binary stream (e.g., Kiro AWS Event Stream)
// by parsing raw bytes into canonical chunks, formatting them as SSE, and forwarding.
func (ra *relayAttempt) handleRawStreamPassthrough(ctx context.Context, response *http.Response) error {
	// Set SSE response headers (we convert raw stream → SSE for the client)
	ra.c.Header("Content-Type", "text/event-stream")
	ra.c.Header("Cache-Control", "no-cache")
	ra.c.Header("Connection", "keep-alive")
	ra.c.Header("X-Accel-Buffering", "no")

	firstToken := true
	var rawResponse strings.Builder
	buf := make([]byte, 4096)

	var firstTokenTimer *time.Timer
	var firstTokenC <-chan time.Time
	if ra.firstTokenTimeOutSec > 0 {
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
			log.Infof("client disconnected, stopping raw stream")
			return nil
		case <-firstTokenC:
			log.Warnf("first token timeout (%ds), switching channel", ra.firstTokenTimeOutSec)
			_ = response.Body.Close()
			return fmt.Errorf("first token timeout (%ds)", ra.firstTokenTimeOutSec)
		default:
			n, readErr := response.Body.Read(buf)
			if n > 0 {
				// Parse raw bytes through provider adapter
				chunk, parseErr := ra.providerAdapter.ParseStreamChunk(ctx, buf[:n])
				if parseErr != nil {
					log.Warnf("failed to parse raw stream chunk: %v", parseErr)
					continue
				}
				if chunk == nil {
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

				// Format chunk as SSE and write to client
				clientData, fmtErr := ra.clientAdapter.FormatStreamChunk(ctx, chunk)
				if fmtErr != nil || len(clientData) == 0 {
					continue
				}
				ra.c.Writer.Write(clientData)
				ra.c.Writer.Flush()
				rawResponse.Write(clientData)
			}
			if readErr != nil {
				if readErr != io.EOF {
					log.Warnf("raw stream read error: %v", readErr)
				}
				ra.metrics.SetFinalResponse(rawResponse.String())
				log.Infof("raw stream end")
				return nil
			}
		}
	}
}

// replaceModelInBody replaces the model name in the raw JSON body.
func replaceModelInBody(body []byte, newModel string) []byte {
	var parsed map[string]interface{}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return body
	}
	if _, ok := parsed["model"]; ok {
		parsed["model"] = newModel
		newBody, err := json.Marshal(parsed)
		if err != nil {
			return body
		}
		return newBody
	}
	return body
}
