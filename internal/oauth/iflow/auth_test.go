package iflow

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRefreshAPIKey(t *testing.T) {
	// Mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST request, got %s", r.Method)
		}
		if r.Header.Get("Cookie") != "test-cookie" {
			t.Errorf("expected Cookie header, got %s", r.Header.Get("Cookie"))
		}

		var req apiKeyRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}
		if req.Name != "test-key" {
			t.Errorf("expected key name 'test-key', got %s", req.Name)
		}

		resp := IFlowAPIKeyResponse{
			Success: true,
			Data: struct {
				APIKey     string `json:"apiKey"`
				ExpireTime string `json:"expireTime"`
				HasExpired bool   `json:"hasExpired"`
			}{
				APIKey:     "new-api-key",
				ExpireTime: "2025-01-01 12:00",
				HasExpired: false,
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Override endpoint
	originalEndpoint := IFlowAPIKeyEndpoint
	IFlowAPIKeyEndpoint = server.URL
	defer func() { IFlowAPIKeyEndpoint = originalEndpoint }()

	// Test
	resp, err := RefreshAPIKey("test-cookie", "test-key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Data.APIKey != "new-api-key" {
		t.Errorf("expected api key 'new-api-key', got %s", resp.Data.APIKey)
	}
}
