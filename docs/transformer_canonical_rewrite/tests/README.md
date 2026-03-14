# Transformer Canonical Test Suite

High-level, format-agnostic test fixtures for the `internal/transformer2` module
**canonical rewrite** (see `docs/transformer_canonical_rewrite/README.md`).
The new package is created alongside the existing `internal/transformer` — see the parallel-package migration strategy in the main README.

These JSON files capture the current transformer behavior as ground truth.
The rewrite must pass all cases — if a new adapter produces different output,
it's either a regression or an intentional improvement that should be documented.

## How To Use During Rewrite

### Phase 1 — Before Writing Code: Lock the Contract

Each JSON file defines input/output pairs at the **public interface boundary**
(Inbound/Outbound, or the new ClientAdapter/ProviderAdapter). Before touching
any adapter code, write a Go test harness that:

1. Loads `tests/*.json` via `embed` or `os.ReadFile`.
2. For each case, calls the **current** (old) transformer and asserts the
   `expected_*` fields match. This proves the fixtures are correct against
   today's code.

```go
//go:embed tests/*.json
var fixtureFS embed.FS

func TestCanonical_CurrentTransformer(t *testing.T) {
    // iterate fixtures → call old inbound/outbound → assert expected fields
}
```

If any fixture fails against the old code, fix the fixture before proceeding.

### Phase 2 — Implement New Adapters: Red → Green

For each new adapter (e.g. `transformer2/adapter/anthropic/`), write a parallel test function
that loads the **same fixtures** but calls the new adapter:

```go
func TestCanonical_NewAnthropicClient(t *testing.T) {
    cases := loadFixtures("03_tool_calling.json", "04_reasoning_thinking.json", ...)
    for _, tc := range cases {
        if tc.InboundFormat != "anthropic" { continue }
        // Phase 1: ParseRequest
        req, err := newClient.ParseRequest(ctx, tc.ClientRequest, nil)
        assertFieldsMatch(t, tc.ExpectedInternalRequest, req)
        // Phase 4: FormatResponse
        out, err := newClient.FormatResponse(ctx, tc.ExpectedInternalResponse)
        assertFieldsMatch(t, tc.ExpectedClientResponse, out)
    }
}
```

The fixtures drive a TDD cycle: all tests start red, and you make them green
one by one as you implement each adapter method.

### Phase 3 — Streaming: Feed Chunk Sequences

For `stream: true` cases, feed `provider_stream_chunks` one-by-one into the
adapter's `ParseStreamChunk` / `FormatStreamChunk`, then call `AggregateStream`
and compare with `expected_aggregated_response`:

```go
for _, chunk := range tc.ProviderStreamChunks {
    internal, _ := providerAdapter.ParseStreamChunk(ctx, chunk)
    clientBytes, _ := clientAdapter.FormatStreamChunk(ctx, internal)
    // optionally assert clientBytes against expected_client_stream_chunks[i]
}
aggregated, _ := clientAdapter.AggregateStream(ctx)
assertFieldsMatch(t, tc.ExpectedAggregatedResponse, aggregated)
```

### Phase 4 — Cross-Format: End-to-End Pipelines

`07_cross_format_routing.json` tests the full A→canonical→B pipeline. Wire both
client and provider adapters together:

```go
// OpenAI client → Anthropic provider
req, _ := openaiClient.ParseRequest(ctx, tc.ClientRequest, nil)
httpReq, _ := anthropicProvider.BuildRequest(ctx, req, baseURL, key)
assertFieldsMatch(t, tc.ExpectedProviderRequest, decodeBody(httpReq))
```

These are the most valuable tests — they prove that any inbound format can
reach any outbound provider without data loss.

### Phase 5 — Cleanup: Drop Old, Keep Fixtures

After the old `internal/transformer/` directory is deleted (Phase 4),
remove `TestCanonical_CurrentTransformer` but **keep the fixtures and the new
adapter tests**. The JSON files become the long-lived regression suite.

### Assertion Strategy

Fixtures use **partial matching** — `expected_*` objects only contain the fields
that matter. The test harness should:

- Ignore fields not present in the expected object.
- Use `assertFieldsMatch` (deep partial compare), not `reflect.DeepEqual`.
- For `notes`, `description`, `variants`, `*_mappings` — these are documentation
  only, skip them during assertion.
- Special pseudo-fields like `content_type`, `content_parts_count`,
  `tool_calls_count`, `messages_count` are assertion helpers, not real struct
  fields — translate them to the appropriate Go checks.

### Fixture-to-Interface Mapping

| Fixture Field | Old Interface | New Interface |
|---|---|---|
| `client_request` → `expected_internal_request` | `Inbound.TransformRequest()` | `ClientAdapter.ParseRequest()` |
| `expected_internal_request` → `expected_provider_request` | `Outbound.TransformRequest()` | `ProviderAdapter.BuildRequest()` |
| `provider_response` → `expected_internal_response` | `Outbound.TransformResponse()` | `ProviderAdapter.ParseResponse()` |
| `expected_internal_response` → `expected_client_response` | `Inbound.TransformResponse()` | `ClientAdapter.FormatResponse()` |
| `provider_stream_chunks[i]` → internal chunk | `Outbound.TransformStream()` | `ProviderAdapter.ParseStreamChunk()` |
| internal chunk → `expected_client_stream_chunks[i]` | `Inbound.TransformStream()` | `ClientAdapter.FormatStreamChunk()` |
| all chunks → `expected_aggregated_response` | `Inbound.GetInternalResponse()` | `ClientAdapter.AggregateStream()` |

---

## Architecture Under Test

```
Client Request (Format A)
    ↓  inbound.TransformRequest / ClientAdapter.ParseRequest
InternalLLMRequest / canonical.Request
    ↓  outbound.TransformRequest / ProviderAdapter.BuildRequest
Provider HTTP Request (Format B)
    ↓  provider
Provider HTTP Response (Format B)
    ↓  outbound.TransformResponse / ProviderAdapter.ParseResponse
InternalLLMResponse / canonical.Response
    ↓  inbound.TransformResponse / ClientAdapter.FormatResponse
Client Response (Format A)
```

## Supported Formats

| Alias        | Inbound | Outbound | Notes                    |
|--------------|---------|----------|--------------------------|
| openai_chat  | ✓       | ✓        | `/v1/chat/completions`   |
| openai_resp  | ✓       | ✓        | Responses API            |
| openai_embed | ✓       | ✓        | `/v1/embeddings`         |
| anthropic    | ✓       | ✓        | `/v1/messages`           |
| gemini       |         | ✓        | `generateContent`        |
| volcengine   |         | ✓        | Responses API variant    |

## Test Case Schema

```jsonc
{
  "name": "human-readable test name",
  "description": "what this case validates",
  "inbound_format": "openai_chat | openai_resp | openai_embed | anthropic",
  "outbound_format": "openai_chat | openai_resp | openai_embed | anthropic | gemini | volcengine",

  // Phase 1 – Inbound request transform
  "client_request": { /* raw JSON body the client sends */ },
  "expected_internal_request": { /* canonical fields to assert */ },

  // Phase 2 – Outbound request transform
  "expected_provider_request": { /* provider-format body to assert (optional) */ },

  // Phase 3 – Outbound response transform
  "provider_response": { /* raw provider response body */ },
  "provider_status_code": 200,
  "expected_internal_response": { /* canonical fields to assert */ },

  // Phase 4 – Inbound response transform
  "expected_client_response": { /* response body sent back to client (optional) */ },

  // For streaming tests
  "stream": true,
  "provider_stream_chunks": [ /* ordered array of raw SSE data payloads */ ],
  "expected_client_stream_chunks": [ /* ordered array of client-format SSE payloads */ ],
  "expected_aggregated_response": { /* result of AggregateStream() after all chunks */ }
}
```

Not all phases are required in every test. Use `null` or omit fields to skip
assertion on that phase.

## File Index

| # | File                          | Cases | Focus                                  |
|---|-------------------------------|-------|----------------------------------------|
| 1 | `01_basic_chat.json`          | 9     | Simple text chat, system messages      |
| 2 | `02_multimodal.json`          | 10    | Images, audio, files in content        |
| 3 | `03_tool_calling.json`        | 15    | Tool defs, calls, results, round-trip  |
| 4 | `04_reasoning_thinking.json`  | 11    | Reasoning/thinking + signatures        |
| 5 | `05_streaming.json`           | 11    | Stream chunk sequences & aggregation   |
| 6 | `06_embedding.json`           | 11    | Embedding request/response             |
| 7 | `07_cross_format_routing.json`| 14    | Cross-provider transform (A → B)       |
| 8 | `08_error_handling.json`      | 14    | Error responses across formats         |
| 9 | `09_edge_cases.json`          | 26    | Null, empty, type coercion, validation |
|   |                               | **121** | **Total**                            |

## Running

```bash
# Against current (old) transformer — proves fixtures are correct
go test ./internal/transformer/... -run TestCanonical_Current -v

# Against new adapter (transformer2) — the rewrite target
go test ./internal/transformer2/... -run TestCanonical_New -v
```
