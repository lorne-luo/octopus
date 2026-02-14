package iflow

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchAPIKeyInfo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("expected GET request, got %s", r.Method)
		}
		if r.Header.Get("Cookie") != "test-cookie" {
			t.Errorf("expected Cookie header, got %s", r.Header.Get("Cookie"))
		}

		resp := IFlowAPIKeyResponse{
			Success: true,
			Data: struct {
				APIKey     string `json:"apiKey"`
				ExpireTime string `json:"expireTime"`
				HasExpired bool   `json:"hasExpired"`
				Name       string `json:"name"`
				APIKeyMask string `json:"apiKeyMask"`
			}{
				APIKey:     "existing-api-key",
				ExpireTime: "2025-01-01 12:00",
				HasExpired: false,
				Name:       "test-key-name",
				APIKeyMask: "sk-xxx",
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	originalEndpoint := IFlowAPIKeyEndpoint
	IFlowAPIKeyEndpoint = server.URL
	defer func() { IFlowAPIKeyEndpoint = originalEndpoint }()

	resp, err := FetchAPIKeyInfo(context.Background(), "test-cookie")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Data.Name != "test-key-name" {
		t.Errorf("expected key name 'test-key-name', got %s", resp.Data.Name)
	}
	if resp.Data.APIKey != "existing-api-key" {
		t.Errorf("expected api key 'existing-api-key', got %s", resp.Data.APIKey)
	}
}

func TestRefreshAPIKey(t *testing.T) {
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
		if req.Name != "test-key-name" {
			t.Errorf("expected key name 'test-key-name', got %s", req.Name)
		}

		resp := IFlowAPIKeyResponse{
			Success: true,
			Data: struct {
				APIKey     string `json:"apiKey"`
				ExpireTime string `json:"expireTime"`
				HasExpired bool   `json:"hasExpired"`
				Name       string `json:"name"`
				APIKeyMask string `json:"apiKeyMask"`
			}{
				APIKey:     "new-api-key",
				ExpireTime: "2025-02-01 12:00",
				HasExpired: false,
				Name:       "test-key-name",
				APIKeyMask: "sk-yyy",
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	originalEndpoint := IFlowAPIKeyEndpoint
	IFlowAPIKeyEndpoint = server.URL
	defer func() { IFlowAPIKeyEndpoint = originalEndpoint }()

	resp, err := RefreshAPIKey(context.Background(), "test-cookie", "test-key-name")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Data.APIKey != "new-api-key" {
		t.Errorf("expected api key 'new-api-key', got %s", resp.Data.APIKey)
	}
	if resp.Data.Name != "test-key-name" {
		t.Errorf("expected key name 'test-key-name', got %s", resp.Data.Name)
	}
}

func TestFetchAPIKeyInfo_EmptyCookie(t *testing.T) {
	_, err := FetchAPIKeyInfo(context.Background(), "")
	if err == nil {
		t.Error("expected error for empty cookie")
	}
}

func TestRefreshAPIKey_EmptyCookie(t *testing.T) {
	_, err := RefreshAPIKey(context.Background(), "", "test-key")
	if err == nil {
		t.Error("expected error for empty cookie")
	}
}

func TestRefreshAPIKey_EmptyKeyName(t *testing.T) {
	_, err := RefreshAPIKey(context.Background(), "test-cookie", "")
	if err == nil {
		t.Error("expected error for empty key name")
	}
}