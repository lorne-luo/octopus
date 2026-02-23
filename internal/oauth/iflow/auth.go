package iflow

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

var IFlowAPIKeyEndpoint = "https://platform.iflow.cn/api/openapi/apikey"

type apiKeyRequest struct {
	Name string `json:"name"`
}

// FetchAPIKeyInfo retrieves API key information using GET request with BXAuth cookie
func FetchAPIKeyInfo(ctx context.Context, bxAuth string) (*IFlowAPIKeyResponse, error) {
	if strings.TrimSpace(bxAuth) == "" {
		return nil, fmt.Errorf("iflow: bxAuth is empty")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, IFlowAPIKeyEndpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("iflow: create GET request failed: %w", err)
	}

	setBrowserHeaders(req, bxAuth)

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("iflow: GET request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := readResponseBody(resp)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("iflow: GET request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var parsedResp IFlowAPIKeyResponse
	if err := json.Unmarshal(body, &parsedResp); err != nil {
		return nil, fmt.Errorf("iflow: unmarshal response failed: %w", err)
	}

	if !parsedResp.Success {
		return nil, fmt.Errorf("iflow: request not successful (code=%s): %s", parsedResp.Code, parsedResp.Message)
	}

	// Handle initial response where apiKey field might be apiKeyMask
	// This matches CLIProxyAPI behavior
	if parsedResp.Data.APIKey == "" && parsedResp.Data.APIKeyMask != "" {
		parsedResp.Data.APIKey = parsedResp.Data.APIKeyMask
	}

	return &parsedResp, nil
}

// RefreshAPIKey refreshes the API key using POST request with BXAuth cookie
func RefreshAPIKey(ctx context.Context, bxAuth, keyName string) (*IFlowAPIKeyResponse, error) {
	if strings.TrimSpace(bxAuth) == "" {
		return nil, fmt.Errorf("iflow: bxAuth is empty")
	}
	if strings.TrimSpace(keyName) == "" {
		return nil, fmt.Errorf("iflow: key name is empty")
	}

	reqBody, err := json.Marshal(apiKeyRequest{Name: keyName})
	if err != nil {
		return nil, fmt.Errorf("iflow: marshal request failed: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, IFlowAPIKeyEndpoint, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("iflow: create POST request failed: %w", err)
	}

	setBrowserHeaders(req, bxAuth)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://platform.iflow.cn")
	req.Header.Set("Referer", "https://platform.iflow.cn/")

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("iflow: POST request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := readResponseBody(resp)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("iflow: POST request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var parsedResp IFlowAPIKeyResponse
	if err := json.Unmarshal(body, &parsedResp); err != nil {
		return nil, fmt.Errorf("iflow: unmarshal response failed: %w", err)
	}

	if !parsedResp.Success {
		return nil, fmt.Errorf("iflow: request not successful: %s", parsedResp.Message)
	}

	return &parsedResp, nil
}

// setBrowserHeaders sets headers to mimic browser behavior
// bxAuth is the raw BXAuth value without the "BXAuth=" prefix
func setBrowserHeaders(req *http.Request, bxAuth string) {
	req.Header.Set("Cookie", "BXAuth="+bxAuth)
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
}

// readResponseBody reads response body with gzip decompression support
func readResponseBody(resp *http.Response) ([]byte, error) {
	var reader io.Reader = resp.Body

	if resp.Header.Get("Content-Encoding") == "gzip" {
		gzipReader, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("iflow: create gzip reader failed: %w", err)
		}
		defer gzipReader.Close()
		reader = gzipReader
	}

	body, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("iflow: read response body failed: %w", err)
	}

	return body, nil
}