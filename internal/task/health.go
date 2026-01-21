package task

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/op"
	"github.com/bestruirui/octopus/internal/transformer/outbound"
	"github.com/bestruirui/octopus/internal/utils/log"
)

// RegisterHealthTask registers a health check task
func RegisterHealthTask(hc *model.HealthCheck) {
	interval := time.Duration(hc.IntervalMinutes) * time.Minute
	if interval < time.Minute {
		interval = time.Minute // Minimum 1 minute
	}
	taskName := fmt.Sprintf("health_check_%d", hc.ID)
	Register(taskName, interval, true, func() {
		RunHealthCheck(hc.ID)
	})
}

// UpdateHealthTask updates a health check task's interval
func UpdateHealthTask(hc *model.HealthCheck) {
	interval := time.Duration(hc.IntervalMinutes) * time.Minute
	if interval < time.Minute {
		interval = time.Minute // Minimum 1 minute
	}
	taskName := fmt.Sprintf("health_check_%d", hc.ID)
	Update(taskName, interval)
}

// UnregisterHealthTask removes a health check task
func UnregisterHealthTask(id int) {
	taskName := fmt.Sprintf("health_check_%d", id)
	Update(taskName, 0) // Setting interval to 0 removes the task
}

// RunHealthCheck executes a single health check
func RunHealthCheck(id int) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Get the health check configuration
	hc, err := op.HealthGet(ctx, id)
	if err != nil {
		log.Errorf("failed to get health check %d: %v", id, err)
		return
	}

	// Get the channel
	channel, err := op.ChannelGet(hc.ChannelID, ctx)
	if err != nil {
		errStr := fmt.Sprintf("failed to get channel: %v", err)
		op.HealthUpdateStatus(ctx, id, model.HealthCheckStatusUnhealthy, &errStr, calculateNextCheck(hc.IntervalMinutes), nil)
		log.Errorf("health check %d: %s", id, errStr)
		return
	}

	// Check if channel is enabled
	if !channel.Enabled {
		errStr := "channel is disabled"
		op.HealthUpdateStatus(ctx, id, model.HealthCheckStatusUnhealthy, &errStr, calculateNextCheck(hc.IntervalMinutes), nil)
		log.Warnf("health check %d: channel %s is disabled", id, channel.Name)
		return
	}

	// Update status to checking
	op.HealthUpdateStatus(ctx, id, model.HealthCheckStatusChecking, nil, nil, nil)

	// Record start time for latency measurement
	startTime := time.Now()

	// Build the request based on channel type
	err = performHealthCheck(ctx, channel, hc)

	// Calculate latency
	latencyMs := int(time.Since(startTime).Milliseconds())

	if err != nil {
		errStr := err.Error()
		op.HealthUpdateStatus(ctx, id, model.HealthCheckStatusUnhealthy, &errStr, calculateNextCheck(hc.IntervalMinutes), &latencyMs)
		log.Errorf("health check %d failed: %v", id, err)
		return
	}

	// Success
	op.HealthUpdateStatus(ctx, id, model.HealthCheckStatusHealthy, nil, calculateNextCheck(hc.IntervalMinutes), &latencyMs)
	log.Infof("health check %d succeeded for channel %s model %s (latency: %dms)", id, channel.Name, hc.ModelName, latencyMs)
}

func calculateNextCheck(intervalMinutes int) *time.Time {
	duration := time.Duration(intervalMinutes) * time.Minute
	if duration < time.Minute {
		duration = time.Minute
	}
	next := time.Now().Add(duration)
	return &next
}

func performHealthCheck(ctx context.Context, channel *model.Channel, hc *model.HealthCheck) error {
	// Get a channel key
	if len(channel.Keys) == 0 {
		return fmt.Errorf("no API keys configured for channel")
	}

	var activeKey *model.ChannelKey
	for i := range channel.Keys {
		if channel.Keys[i].Enabled {
			activeKey = &channel.Keys[i]
			break
		}
	}
	if activeKey == nil {
		return fmt.Errorf("no active API keys for channel")
	}

	// Get the base URL
	if len(channel.BaseUrls) == 0 {
		return fmt.Errorf("no base URLs configured for channel")
	}
	baseURL := strings.TrimSuffix(channel.BaseUrls[0].URL, "/")

	// Build request based on channel type
	switch channel.Type {
	case outbound.OutboundTypeOpenAIChat:
		return checkOpenAIChat(ctx, baseURL, activeKey.ChannelKey, hc.ModelName, hc.Prompt, channel)
	case outbound.OutboundTypeAnthropic:
		return checkAnthropicMessages(ctx, baseURL, activeKey.ChannelKey, hc.ModelName, hc.Prompt, channel)
	case outbound.OutboundTypeGemini:
		return checkGemini(ctx, baseURL, activeKey.ChannelKey, hc.ModelName, hc.Prompt, channel)
	default:
		// Default to OpenAI-compatible format
		return checkOpenAIChat(ctx, baseURL, activeKey.ChannelKey, hc.ModelName, hc.Prompt, channel)
	}
}

func checkOpenAIChat(ctx context.Context, baseURL, apiKey, model, prompt string, channel *model.Channel) error {
	reqBody := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"max_tokens": 10,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	// Handle baseURL that may or may not include /v1
	var url string
	if strings.HasSuffix(baseURL, "/v1") || strings.Contains(baseURL, "/v1/") {
		url = strings.TrimSuffix(baseURL, "/") + "/chat/completions"
	} else {
		url = baseURL + "/v1/chat/completions"
	}
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	// Add custom headers if any
	for _, h := range channel.CustomHeader {
		req.Header.Set(h.HeaderKey, h.HeaderValue)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	return nil
}

func checkAnthropicMessages(ctx context.Context, baseURL, apiKey, model, prompt string, channel *model.Channel) error {
	reqBody := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"max_tokens": 10,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	// Handle baseURL that may or may not include /v1
	var url string
	if strings.HasSuffix(baseURL, "/v1") || strings.Contains(baseURL, "/v1/") {
		url = strings.TrimSuffix(baseURL, "/") + "/messages"
	} else {
		url = baseURL + "/v1/messages"
	}
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	// Add custom headers if any
	for _, h := range channel.CustomHeader {
		req.Header.Set(h.HeaderKey, h.HeaderValue)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	return nil
}

func checkGemini(ctx context.Context, baseURL, apiKey, model, prompt string, channel *model.Channel) error {
	reqBody := map[string]any{
		"contents": []map[string]any{
			{
				"parts": []map[string]string{
					{"text": prompt},
				},
			},
		},
		"generationConfig": map[string]int{
			"maxOutputTokens": 10,
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/v1beta/models/%s:generateContent?key=%s", baseURL, model, apiKey)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Add custom headers if any
	for _, h := range channel.CustomHeader {
		req.Header.Set(h.HeaderKey, h.HeaderValue)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	return nil
}

// LoadAndRegisterHealthTasks loads all health checks from DB and registers them as tasks
func LoadAndRegisterHealthTasks() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	healthChecks, err := op.HealthList(ctx)
	if err != nil {
		log.Errorf("failed to load health checks: %v", err)
		return
	}

	for _, hc := range healthChecks {
		hcCopy := hc.HealthCheck // Create a copy to avoid closure issues
		RegisterHealthTask(&hcCopy)
	}

	log.Infof("loaded and registered %d health check tasks", len(healthChecks))
}
