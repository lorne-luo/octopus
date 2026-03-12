package relay

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/bestruirui/octopus/internal/helper"
	"github.com/bestruirui/octopus/internal/op"
	"github.com/bestruirui/octopus/internal/transformer/model"
	"github.com/bestruirui/octopus/internal/transformer/outbound"
	"github.com/bestruirui/octopus/internal/utils/log"
	"github.com/gin-gonic/gin"
	"github.com/tmaxmax/go-sse"
)

// TestRequest is the request body for the channel test endpoint.
type TestRequest struct {
	ChannelID    int    `json:"channel_id" binding:"required"`
	KeyIndex     int    `json:"key_index"`
	BaseUrlIndex *int   `json:"base_url_index"` // optional: which base_url to use; nil = auto (lowest delay)
	Model        string `json:"model" binding:"required"`
	TestType     string `json:"test_type" binding:"required"` // text_chat, vision_chat, tool_chat
}

// testTemplates returns predefined OpenAI-format test payloads.
func testTemplate(testType, modelName string) (*model.InternalLLMRequest, error) {
	streamTrue := true
	temp := float64(0.7)
	topP := float64(1)

	switch testType {
	case "text_chat":
		content := "Hi"
		return &model.InternalLLMRequest{
			Model:       modelName,
			Temperature: &temp,
			TopP:        &topP,
			Stream:      &streamTrue,
			Messages: []model.Message{
				{Role: "user", Content: model.MessageContent{Content: &content}},
			},
		}, nil

	case "vision_chat":
		text := "What color is in this image?"
		imgURL := "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8DwHwAFBQIAX8jx0gAAAABJRU5ErkJggg=="
		textType := "text"
		imgType := "image_url"
		return &model.InternalLLMRequest{
			Model:       modelName,
			Temperature: &temp,
			TopP:        &topP,
			Stream:      &streamTrue,
			Messages: []model.Message{
				{
					Role: "user",
					Content: model.MessageContent{
						MultipleContent: []model.MessageContentPart{
							{Type: textType, Text: &text},
							{Type: imgType, ImageURL: &model.ImageURL{URL: imgURL}},
						},
					},
				},
			},
		}, nil

	case "tool_chat":
		content := "What is the weather in San Francisco?"
		return &model.InternalLLMRequest{
			Model:       modelName,
			Temperature: &temp,
			TopP:        &topP,
			Stream:      &streamTrue,
			Messages: []model.Message{
				{Role: "user", Content: model.MessageContent{Content: &content}},
			},
			Tools: []model.Tool{
				{
					Type: "function",
					Function: model.Function{
						Name:        "get_weather",
						Description: "Get the weather",
						Parameters: json.RawMessage(`{
							"type": "object",
							"properties": {
								"location": {"description": "City name", "type": "string"}
							},
							"required": ["location"],
							"additionalProperties": false
						}`),
					},
				},
			},
		}, nil

	default:
		return nil, fmt.Errorf("unknown test_type: %s", testType)
	}
}

// TestHandler handles the channel test proxy request.
// It bypasses the normal Group/Balancer routing and directly sends to a specific channel.
func TestHandler(c *gin.Context) {
	var req TestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: " + err.Error()})
		return
	}

	// 1. Get the channel
	channel, err := op.ChannelGet(req.ChannelID, c.Request.Context())
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("channel %d not found: %v", req.ChannelID, err)})
		return
	}

	// 2. Get the key by index
	if len(channel.Keys) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "channel has no keys"})
		return
	}
	keyIndex := req.KeyIndex
	if keyIndex < 0 || keyIndex >= len(channel.Keys) {
		keyIndex = 0
	}
	usedKey := channel.Keys[keyIndex]

	// 3. Get the outbound adapter
	outAdapter := outbound.Get(channel.Type)
	if outAdapter == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("unsupported channel type: %d", channel.Type)})
		return
	}

	// 4. Build the internal request from the test template
	internalRequest, err := testTemplate(req.TestType, req.Model)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 5. Resolve the base URL
	baseUrl := channel.GetBaseUrl()
	if req.BaseUrlIndex != nil {
		idx := *req.BaseUrlIndex
		if idx >= 0 && idx < len(channel.BaseUrls) {
			baseUrl = channel.BaseUrls[idx].URL
		}
	}
	if baseUrl == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "channel has no base URL configured"})
		return
	}

	// 6. Transform to outbound request
	ctx := c.Request.Context()
	outboundRequest, err := outAdapter.TransformRequest(ctx, internalRequest, baseUrl, usedKey.ChannelKey)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to build outbound request: " + err.Error()})
		return
	}

	// 6. Copy custom headers from channel
	if len(channel.CustomHeader) > 0 {
		for _, header := range channel.CustomHeader {
			outboundRequest.Header.Set(header.HeaderKey, header.HeaderValue)
		}
	}

	// 7. Send the request using the channel's HTTP client
	httpClient, err := helper.ChannelHttpClient(channel)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create http client: " + err.Error()})
		return
	}

	response, err := httpClient.Do(outboundRequest)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to send request: " + err.Error()})
		return
	}
	defer response.Body.Close()

	// 8. Check upstream response status
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, _ := io.ReadAll(response.Body)
		c.JSON(response.StatusCode, gin.H{"error": fmt.Sprintf("upstream error %d: %s", response.StatusCode, string(body))})
		return
	}

	// 9. Handle SSE stream or non-stream response
	if internalRequest.Stream != nil && *internalRequest.Stream {
		handleTestStreamResponse(c, response)
	} else {
		body, err := io.ReadAll(response.Body)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read response: " + err.Error()})
			return
		}
		c.Data(http.StatusOK, "application/json", body)
	}
}

// handleTestStreamResponse transparently proxies SSE from upstream to the client.
func handleTestStreamResponse(c *gin.Context, response *http.Response) {
	ct := response.Header.Get("Content-Type")
	if ct != "" && !strings.Contains(strings.ToLower(ct), "text/event-stream") {
		// Not SSE — read body and return as-is
		body, _ := io.ReadAll(io.LimitReader(response.Body, 64*1024))
		c.Data(http.StatusOK, "application/json", body)
		return
	}

	// Set SSE response headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	ctx := c.Request.Context()

	type sseReadResult struct {
		data string
		err  error
	}
	results := make(chan sseReadResult, 1)
	go func() {
		defer close(results)
		readCfg := &sse.ReadConfig{MaxEventSize: 1 << 20} // 1MB
		for ev, err := range sse.Read(response.Body, readCfg) {
			if err != nil {
				results <- sseReadResult{err: err}
				return
			}
			results <- sseReadResult{data: ev.Data}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			log.Infof("[test] client disconnected")
			return
		case r, ok := <-results:
			if !ok {
				return
			}
			if r.err != nil {
				log.Warnf("[test] SSE read error: %v", r.err)
				return
			}

			var buf bytes.Buffer
			buf.WriteString("data: ")
			buf.WriteString(r.data)
			buf.WriteString("\n\n")
			c.Writer.Write(buf.Bytes())
			c.Writer.Flush()
		}
	}
}
