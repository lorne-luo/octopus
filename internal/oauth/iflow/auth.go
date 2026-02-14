package iflow

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

var IFlowAPIKeyEndpoint = "https://platform.iflow.cn/api/openapi/apikey"

type apiKeyRequest struct {
	Name string `json:"name"`
}

func RefreshAPIKey(cookie, keyName string) (*IFlowAPIKeyResponse, error) {
	reqBody, err := json.Marshal(apiKeyRequest{Name: keyName})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", IFlowAPIKeyEndpoint, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", cookie)
	// Add User-Agent to mimic browser if needed, or just standard
	req.Header.Set("User-Agent", "Octopus/1.0")

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("iflow api returned status %d: %s", resp.StatusCode, string(body))
	}

	var parsedResp IFlowAPIKeyResponse
	if err := json.Unmarshal(body, &parsedResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if !parsedResp.Success {
		return nil, fmt.Errorf("iflow api returned false success")
	}

	return &parsedResp, nil
}
