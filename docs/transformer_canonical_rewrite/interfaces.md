# Adapter Interfaces

> Package location: `internal/transformer2/adapter/`

### `adapter/adapter.go`

```go
// ClientAdapter handles client-facing format conversion.
// Stateful per-request (tracks stream chunks for aggregation).
type ClientAdapter interface {
    ParseRequest(ctx context.Context, body []byte, header http.Header) (*canonical.Request, error)
    FormatResponse(ctx context.Context, resp *canonical.Response) ([]byte, error)
    FormatStreamChunk(ctx context.Context, chunk *canonical.Chunk) ([]byte, error)
    AggregateStream(ctx context.Context) (*canonical.Response, error)

    // FormatError converts a canonical.Error into the client's expected
    // error JSON format. Called when the proxy itself errors or when a
    // provider returns a non-2xx response that must be relayed to the client
    // in the correct wire format (e.g. Claude Code expects Anthropic-shaped errors).
    FormatError(ctx context.Context, err *canonical.Error) ([]byte, error)
}

// ProviderAdapter handles provider-facing format conversion.
// BuildRequest reads req.Headers (populated by ClientAdapter.ParseRequest)
// and forwards/generates provider-specific HTTP headers as needed.
type ProviderAdapter interface {
    BuildRequest(ctx context.Context, req *canonical.Request, baseUrl, key string) (*http.Request, error)
    ParseResponse(ctx context.Context, resp *http.Response) (*canonical.Response, error)
    ParseStreamChunk(ctx context.Context, data []byte) (*canonical.Chunk, error)
}
```

**Header flow**: The relay layer passes `*http.Request.Header` to `ClientAdapter.ParseRequest`. Each ClientAdapter extracts the headers it cares about into `canonical.Request.Headers`:
- Anthropic ClientAdapter: extracts `anthropic-beta`, `anthropic-version`
- OpenAI ClientAdapter: no provider-specific headers needed (feature gating is in JSON body)
- Other adapters: extract as needed

`ProviderAdapter.BuildRequest` then consumes `Request.Headers` — forwarding, merging with auto-generated values, and setting them on the outbound `*http.Request`.

### `adapter/registry.go`

```go
// ClientType identifies the inbound client API format.
// Values are independent of DB — only used in handler routing.
type ClientType int
const (
    ClientOpenAIChat ClientType = iota
    ClientOpenAIResponse  // OpenAI Responses API (/v1/responses)
    ClientOpenAIEmbedding
    ClientAnthropic
)

// ProviderType identifies the outbound provider API format.
// CRITICAL: values MUST match existing outbound.OutboundType iota values
// because Channel.Type is persisted in the database via GORM.
// DO NOT reorder or insert values — append only.
type ProviderType int
const (
    ProviderOpenAIChat     ProviderType = 0  // was OutboundTypeOpenAIChat
    ProviderOpenAIResponse ProviderType = 1  // was OutboundTypeOpenAIResponse
    ProviderAnthropic      ProviderType = 2  // was OutboundTypeAnthropic
    ProviderGemini         ProviderType = 3  // was OutboundTypeGemini
    ProviderVolcengine     ProviderType = 4  // was OutboundTypeVolcengine
    ProviderOpenAIEmbedding ProviderType = 5 // was OutboundTypeOpenAIEmbedding
)

func GetClient(t ClientType) ClientAdapter { ... }
func GetProvider(t ProviderType) ProviderAdapter { ... }
func IsChatProvider(t ProviderType) bool { ... }
func IsEmbeddingProvider(t ProviderType) bool { ... }
```

### `adapter/aggregator.go`

Extract the stream chunk accumulation logic currently embedded in each inbound adapter into a reusable `StreamAggregator`:

```go
type StreamAggregator struct {
    chunks []*canonical.Chunk
    full   *canonical.Response
}

func (a *StreamAggregator) AddChunk(c *canonical.Chunk)
func (a *StreamAggregator) SetFull(r *canonical.Response)
func (a *StreamAggregator) Aggregate() (*canonical.Response, error)
// Aggregate merges:
//   - text: concatenation across chunks
//   - tool_call args: concatenation by index
//   - finish_reason: from last chunk
//   - usage: from final chunk
//   - ReasoningSignature: last-wins (last non-nil value overwrites),
//     because Gemini may send signature in a trailing chunk with empty text
```

