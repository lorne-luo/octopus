package openai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bestruirui/octopus/internal/transformer2/canonical"
)

func TestEmbeddingBuildRequest_Basic(t *testing.T) {
	req := &canonical.Request{
		Kind:  canonical.KindEmbedding,
		Model: "text-embedding-3-small",
		EmbeddingInput: &canonical.EmbeddingInput{
			Text: "Hello, world!",
		},
	}

	adapter := NewEmbeddingProviderAdapter()
	ctx := context.Background()

	httpReq, err := adapter.BuildRequest(ctx, req, "https://api.openai.com/v1", "test-key")
	if err != nil {
		t.Fatalf("BuildRequest failed: %v", err)
	}

	// Verify HTTP method and URL
	if httpReq.Method != "POST" {
		t.Errorf("Expected POST method, got %s", httpReq.Method)
	}
	// Base URL with /v1 prefix, endpoint is /embeddings
	if httpReq.URL.String() != "https://api.openai.com/v1/embeddings" {
		t.Errorf("Expected URL 'https://api.openai.com/v1/embeddings', got %s", httpReq.URL.String())
	}

	// Verify headers
	if httpReq.Header.Get("Content-Type") != "application/json" {
		t.Errorf("Expected Content-Type 'application/json', got %s", httpReq.Header.Get("Content-Type"))
	}
	if !strings.HasPrefix(httpReq.Header.Get("Authorization"), "Bearer ") {
		t.Errorf("Expected Authorization header with Bearer token, got %s", httpReq.Header.Get("Authorization"))
	}

	// Verify body
	body, err := io.ReadAll(httpReq.Body)
	if err != nil {
		t.Fatalf("Failed to read request body: %v", err)
	}

	var embReq EmbeddingRequest
	if err := json.Unmarshal(body, &embReq); err != nil {
		t.Fatalf("Failed to unmarshal request body: %v", err)
	}

	if embReq.Model != "text-embedding-3-small" {
		t.Errorf("Expected model 'text-embedding-3-small', got '%s'", embReq.Model)
	}
	if embReq.Input.Single != "Hello, world!" {
		t.Errorf("Expected input 'Hello, world!', got '%s'", embReq.Input.Single)
	}
}

func TestEmbeddingBuildRequest_ArrayInput(t *testing.T) {
	req := &canonical.Request{
		Kind:  canonical.KindEmbedding,
		Model: "text-embedding-3-small",
		EmbeddingInput: &canonical.EmbeddingInput{
			Texts: []string{"Hello", "World"},
		},
	}

	adapter := NewEmbeddingProviderAdapter()
	ctx := context.Background()

	httpReq, err := adapter.BuildRequest(ctx, req, "https://api.openai.com/v1", "test-key")
	if err != nil {
		t.Fatalf("BuildRequest failed: %v", err)
	}

	body, err := io.ReadAll(httpReq.Body)
	if err != nil {
		t.Fatalf("Failed to read request body: %v", err)
	}

	var embReq EmbeddingRequest
	if err := json.Unmarshal(body, &embReq); err != nil {
		t.Fatalf("Failed to unmarshal request body: %v", err)
	}

	if len(embReq.Input.Multiple) != 2 {
		t.Errorf("Expected 2 inputs, got %d", len(embReq.Input.Multiple))
	}
	if embReq.Input.Multiple[0] != "Hello" {
		t.Errorf("Expected first input 'Hello', got '%s'", embReq.Input.Multiple[0])
	}
	if embReq.Input.Multiple[1] != "World" {
		t.Errorf("Expected second input 'World', got '%s'", embReq.Input.Multiple[1])
	}
}

func TestEmbeddingBuildRequest_WithDimensions(t *testing.T) {
	dims := int64(256)
	encoding := "float"
	user := "user-123"

	req := &canonical.Request{
		Kind:  canonical.KindEmbedding,
		Model: "text-embedding-3-small",
		EmbeddingInput: &canonical.EmbeddingInput{
			Text: "Hello, world!",
		},
		EmbeddingDimensions:     &dims,
		EmbeddingEncodingFormat: &encoding,
		User:                    &user,
	}

	adapter := NewEmbeddingProviderAdapter()
	ctx := context.Background()

	httpReq, err := adapter.BuildRequest(ctx, req, "https://api.openai.com/v1", "test-key")
	if err != nil {
		t.Fatalf("BuildRequest failed: %v", err)
	}

	body, err := io.ReadAll(httpReq.Body)
	if err != nil {
		t.Fatalf("Failed to read request body: %v", err)
	}

	var embReq EmbeddingRequest
	if err := json.Unmarshal(body, &embReq); err != nil {
		t.Fatalf("Failed to unmarshal request body: %v", err)
	}

	if embReq.Dimensions == nil || *embReq.Dimensions != 256 {
		t.Errorf("Expected dimensions 256, got %v", embReq.Dimensions)
	}
	if embReq.EncodingFormat == nil || *embReq.EncodingFormat != "float" {
		t.Errorf("Expected encoding_format 'float', got %v", embReq.EncodingFormat)
	}
	if embReq.User == nil || *embReq.User != "user-123" {
		t.Errorf("Expected user 'user-123', got %v", embReq.User)
	}
}

func TestEmbeddingBuildRequest_TrailingSlash(t *testing.T) {
	req := &canonical.Request{
		Kind:  canonical.KindEmbedding,
		Model: "text-embedding-3-small",
		EmbeddingInput: &canonical.EmbeddingInput{
			Text: "Hello",
		},
	}

	adapter := NewEmbeddingProviderAdapter()
	ctx := context.Background()

	tests := []struct {
		name    string
		baseURL string
		wantURL string
	}{
		{
			name:    "no_trailing_slash",
			baseURL: "https://api.openai.com/v1",
			wantURL: "https://api.openai.com/v1/embeddings",
		},
		{
			name:    "with_trailing_slash",
			baseURL: "https://api.openai.com/v1/",
			wantURL: "https://api.openai.com/v1/embeddings",
		},
		{
			name:    "without_v1_prefix",
			baseURL: "https://api.openai.com",
			wantURL: "https://api.openai.com/embeddings",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpReq, err := adapter.BuildRequest(ctx, req, tt.baseURL, "test-key")
			if err != nil {
				t.Fatalf("BuildRequest failed: %v", err)
			}
			if httpReq.URL.String() != tt.wantURL {
				t.Errorf("Expected URL '%s', got '%s'", tt.wantURL, httpReq.URL.String())
			}
		})
	}
}

func TestEmbeddingParseResponse_Success(t *testing.T) {
	adapter := NewEmbeddingProviderAdapter()
	ctx := context.Background()

	providerResp := EmbeddingResponse{
		Object: "list",
		Model:  "text-embedding-3-small",
		Data: []EmbeddingData{
			{
				Object: "embedding",
				Index:  0,
				Embedding: EmbeddingVector{
					Floats: []float64{0.1, 0.2, 0.3, 0.4, 0.5},
				},
			},
		},
		Usage: &EmbeddingUsage{
			PromptTokens: 5,
			TotalTokens:  5,
		},
	}

	body, err := json.Marshal(providerResp)
	if err != nil {
		t.Fatalf("Failed to marshal response: %v", err)
	}

	httpResp := &http.Response{
		StatusCode: 200,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader(body)),
	}
	httpResp.Header.Set("Content-Type", "application/json")

	resp, err := adapter.ParseResponse(ctx, httpResp)
	if err != nil {
		t.Fatalf("ParseResponse failed: %v", err)
	}

	// Verify response
	if resp.StatusCode != 200 {
		t.Errorf("Expected status code 200, got %d", resp.StatusCode)
	}
	if resp.Model != "text-embedding-3-small" {
		t.Errorf("Expected model 'text-embedding-3-small', got '%s'", resp.Model)
	}
	if resp.Object != "list" {
		t.Errorf("Expected object 'list', got '%s'", resp.Object)
	}

	// Verify embeddings
	if len(resp.Embeddings) != 1 {
		t.Fatalf("Expected 1 embedding, got %d", len(resp.Embeddings))
	}
	if resp.Embeddings[0].Index != 0 {
		t.Errorf("Expected index 0, got %d", resp.Embeddings[0].Index)
	}
	if len(resp.Embeddings[0].Embedding) != 5 {
		t.Errorf("Expected embedding length 5, got %d", len(resp.Embeddings[0].Embedding))
	}
	if resp.Embeddings[0].Embedding[0] != 0.1 {
		t.Errorf("Expected first embedding value 0.1, got %f", resp.Embeddings[0].Embedding[0])
	}

	// Verify usage
	if resp.Usage == nil {
		t.Fatal("Expected usage, got nil")
	}
	if resp.Usage.PromptTokens != 5 {
		t.Errorf("Expected prompt_tokens 5, got %d", resp.Usage.PromptTokens)
	}
	if resp.Usage.TotalTokens != 5 {
		t.Errorf("Expected total_tokens 5, got %d", resp.Usage.TotalTokens)
	}
}

func TestEmbeddingParseResponse_Base64(t *testing.T) {
	adapter := NewEmbeddingProviderAdapter()
	ctx := context.Background()

	// Test with a mock base64 string - in production this would be properly encoded floats
	base64Embedding := base64.StdEncoding.EncodeToString([]byte("test-embedding-data"))

	providerResp := EmbeddingResponse{
		Object: "list",
		Model:  "text-embedding-3-small",
		Data: []EmbeddingData{
			{
				Object: "embedding",
				Index:  0,
				Embedding: EmbeddingVector{
					Base64:   base64Embedding,
					IsBase64: true,
				},
			},
		},
		Usage: &EmbeddingUsage{
			PromptTokens: 5,
			TotalTokens:  5,
		},
	}

	body, err := json.Marshal(providerResp)
	if err != nil {
		t.Fatalf("Failed to marshal response: %v", err)
	}

	httpResp := &http.Response{
		StatusCode: 200,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader(body)),
	}
	httpResp.Header.Set("Content-Type", "application/json")

	resp, err := adapter.ParseResponse(ctx, httpResp)
	if err != nil {
		t.Fatalf("ParseResponse failed: %v", err)
	}

	// Verify embeddings were parsed
	if len(resp.Embeddings) != 1 {
		t.Fatalf("Expected 1 embedding, got %d", len(resp.Embeddings))
	}

	// For base64, we just verify it was stored (the actual decoding would be done when needed)
	// In the canonical format, embeddings are stored as []float64
	// If base64, we need to decode it
	if resp.Embeddings[0].Object != "embedding" {
		t.Errorf("Expected object 'embedding', got '%s'", resp.Embeddings[0].Object)
	}
}

func TestEmbeddingParseResponse_MultipleEmbeddings(t *testing.T) {
	adapter := NewEmbeddingProviderAdapter()
	ctx := context.Background()

	providerResp := EmbeddingResponse{
		Object: "list",
		Model:  "text-embedding-3-small",
		Data: []EmbeddingData{
			{
				Object: "embedding",
				Index:  0,
				Embedding: EmbeddingVector{
					Floats: []float64{0.1, 0.2},
				},
			},
			{
				Object: "embedding",
				Index:  1,
				Embedding: EmbeddingVector{
					Floats: []float64{0.3, 0.4},
				},
			},
		},
		Usage: &EmbeddingUsage{
			PromptTokens: 10,
			TotalTokens:  10,
		},
	}

	body, err := json.Marshal(providerResp)
	if err != nil {
		t.Fatalf("Failed to marshal response: %v", err)
	}

	httpResp := &http.Response{
		StatusCode: 200,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader(body)),
	}
	httpResp.Header.Set("Content-Type", "application/json")

	resp, err := adapter.ParseResponse(ctx, httpResp)
	if err != nil {
		t.Fatalf("ParseResponse failed: %v", err)
	}

	if len(resp.Embeddings) != 2 {
		t.Fatalf("Expected 2 embeddings, got %d", len(resp.Embeddings))
	}
	if resp.Embeddings[0].Index != 0 {
		t.Errorf("Expected first embedding index 0, got %d", resp.Embeddings[0].Index)
	}
	if resp.Embeddings[1].Index != 1 {
		t.Errorf("Expected second embedding index 1, got %d", resp.Embeddings[1].Index)
	}
}

func TestEmbeddingParseResponse_Error(t *testing.T) {
	adapter := NewEmbeddingProviderAdapter()
	ctx := context.Background()

	tests := []struct {
		name       string
		statusCode int
		body       string
		wantCode   string
		wantMsg    string
		wantType   string
	}{
		{
			name:       "invalid_api_key",
			statusCode: 401,
			body:       `{"error": {"code": "invalid_api_key", "message": "Invalid API key provided", "type": "invalid_request_error"}}`,
			wantCode:   "invalid_api_key",
			wantMsg:    "Invalid API key provided",
			wantType:   "invalid_request_error",
		},
		{
			name:       "rate_limit",
			statusCode: 429,
			body:       `{"error": {"code": "rate_limit_exceeded", "message": "Rate limit exceeded", "type": "rate_limit_error"}}`,
			wantCode:   "rate_limit_exceeded",
			wantMsg:    "Rate limit exceeded",
			wantType:   "rate_limit_error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpResp := &http.Response{
				StatusCode: tt.statusCode,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(tt.body)),
			}
			httpResp.Header.Set("Content-Type", "application/json")

			resp, err := adapter.ParseResponse(ctx, httpResp)
			if err != nil {
				t.Fatalf("ParseResponse failed: %v", err)
			}

			if resp.StatusCode != tt.statusCode {
				t.Errorf("Expected status code %d, got %d", tt.statusCode, resp.StatusCode)
			}
			if resp.Error == nil {
				t.Fatal("Expected error, got nil")
			}
			if resp.Error.Code != tt.wantCode {
				t.Errorf("Expected error code '%s', got '%s'", tt.wantCode, resp.Error.Code)
			}
			if resp.Error.Message != tt.wantMsg {
				t.Errorf("Expected error message '%s', got '%s'", tt.wantMsg, resp.Error.Message)
			}
			if resp.Error.Type != tt.wantType {
				t.Errorf("Expected error type '%s', got '%s'", tt.wantType, resp.Error.Type)
			}
		})
	}
}

func TestEmbeddingParseStreamChunk_Error(t *testing.T) {
	adapter := NewEmbeddingProviderAdapter()
	ctx := context.Background()

	_, err := adapter.ParseStreamChunk(ctx, []byte("{}"))
	if err == nil {
		t.Error("Expected error for ParseStreamChunk, got nil")
	}
}

// Integration test
func TestEmbeddingProviderAdapter_EndToEnd(t *testing.T) {
	// Create test server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		if r.Method != "POST" {
			t.Errorf("Expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/v1/embeddings" {
			t.Errorf("Expected path '/v1/embeddings', got '%s'", r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Expected Content-Type 'application/json', got '%s'", r.Header.Get("Content-Type"))
		}

		// Return mock response
		resp := EmbeddingResponse{
			Object: "list",
			Model:  "text-embedding-3-small",
			Data: []EmbeddingData{
				{
					Object: "embedding",
					Index:  0,
					Embedding: EmbeddingVector{
						Floats: []float64{0.1, 0.2, 0.3},
					},
				},
			},
			Usage: &EmbeddingUsage{
				PromptTokens: 5,
				TotalTokens:  5,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	// Build request
	providerAdapter := NewEmbeddingProviderAdapter()
	ctx := context.Background()

	req := &canonical.Request{
		Kind:  canonical.KindEmbedding,
		Model: "text-embedding-3-small",
		EmbeddingInput: &canonical.EmbeddingInput{
			Text: "Hello, world!",
		},
	}

	httpReq, err := providerAdapter.BuildRequest(ctx, req, ts.URL, "test-key")
	if err != nil {
		t.Fatalf("BuildRequest failed: %v", err)
	}

	// Execute request
	client := &http.Client{}
	httpResp, err := client.Do(httpReq)
	if err != nil {
		t.Fatalf("HTTP request failed: %v", err)
	}
	defer httpResp.Body.Close()

	// Parse response
	resp, err := providerAdapter.ParseResponse(ctx, httpResp)
	if err != nil {
		t.Fatalf("ParseResponse failed: %v", err)
	}

	// Verify response
	if resp.Model != "text-embedding-3-small" {
		t.Errorf("Expected model 'text-embedding-3-small', got '%s'", resp.Model)
	}
	if len(resp.Embeddings) != 1 {
		t.Fatalf("Expected 1 embedding, got %d", len(resp.Embeddings))
	}
	if len(resp.Embeddings[0].Embedding) != 3 {
		t.Errorf("Expected embedding length 3, got %d", len(resp.Embeddings[0].Embedding))
	}
}