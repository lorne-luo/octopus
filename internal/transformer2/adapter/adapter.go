package adapter

import (
	"context"
	"net/http"
	"net/url"

	"github.com/bestruirui/octopus/internal/transformer2/canonical"
)

// ClientAdapter handles client-facing format conversion.
// Stateful per-request (tracks stream chunks for aggregation).
type ClientAdapter interface {
	// ParseRequest converts client request body to canonical Request.
	ParseRequest(ctx context.Context, body []byte, header http.Header) (*canonical.Request, error)

	// FormatResponse converts canonical Response to client response body.
	FormatResponse(ctx context.Context, resp *canonical.Response) ([]byte, error)

	// FormatStreamChunk converts canonical Chunk to client streaming format.
	FormatStreamChunk(ctx context.Context, chunk *canonical.Chunk) ([]byte, error)

	// AggregateStream aggregates all collected chunks into a full response.
	AggregateStream(ctx context.Context) (*canonical.Response, error)

	// FormatError converts canonical Error to client error format.
	FormatError(ctx context.Context, err *canonical.Error) ([]byte, error)
}

// ProviderAdapter handles provider-facing format conversion.
type ProviderAdapter interface {
	// BuildRequest builds an HTTP request for the provider from canonical Request.
	BuildRequest(ctx context.Context, req *canonical.Request, baseURL, key string) (*http.Request, error)

	// ParseResponse parses provider HTTP response to canonical Response.
	ParseResponse(ctx context.Context, resp *http.Response) (*canonical.Response, error)

	// ParseStreamChunk parses SSE data to canonical Chunk.
	ParseStreamChunk(ctx context.Context, data []byte) (*canonical.Chunk, error)
}

// PassthroughProvider is an optional extension of ProviderAdapter.
// When the inbound format matches the outbound channel format, implementors
// can skip the canonical Request → provider format conversion and directly
// use the raw request body.
type PassthroughProvider interface {
	// BuildPassthroughRequest builds an HTTP request using the raw body.
	// The model name in rawBody has already been replaced by the caller.
	BuildPassthroughRequest(ctx context.Context, rawBody []byte, stream bool, baseURL, key string, query url.Values) (*http.Request, error)
}

// RawStreamProvider is an optional extension of ProviderAdapter.
// Implementors whose response is NOT in SSE format (e.g. Kiro uses
// AWS Event Stream binary format) should implement this interface so
// the relay reads the raw byte stream instead of parsing SSE events.
type RawStreamProvider interface {
	// IsRawStream returns true if this provider uses raw byte stream instead of SSE.
	IsRawStream() bool
}
