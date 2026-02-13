# Raw Endpoint Log Content Design

## Background

The current log detail page displays request and response content in normalized internal format (`InternalLLMRequest` / OpenAI-like response shape).  
This is not aligned with what endpoint clients actually send and receive.

Goal:
- Request content should reflect the original inbound endpoint payload shape.
- Response content should reflect the final outbound payload shape returned to the client.
- Must support both non-stream HTTP and SSE.
- For SSE, response content should be the merged final JSON (not event chunks).

## Confirmed Decisions

- Failed request behavior: store exact client-facing error body in `response_content`.
- SSE logging shape: store final reconstructed JSON only.
- Request logging shape: parse and re-serialize JSON before storing.

## Recommended Approach

Use a relay-centric capture strategy with minimal adapter changes:

1. Capture and normalize inbound raw request body in relay.
2. Capture final client-facing response bytes in relay.
3. For SSE, reconstruct final response at stream end using adapter aggregation, then convert once to endpoint JSON.
4. Persist these captured endpoint payloads in metrics/log model fields.

This provides endpoint-accurate logs with low refactor risk.

## Architecture

### Relay Responsibilities

- Keep a copy of incoming raw body from `parseRequest(...)`.
- Normalize request JSON for logging (compact canonical JSON when valid).
- Capture final response bytes:
  - Non-stream: bytes produced by `inAdapter.TransformResponse(...)`.
  - Stream: bytes produced by `inAdapter.TransformResponse(...)` on merged internal response from `GetInternalResponse(...)`.
- Capture error response body when returning failure to client.

### Metrics Responsibilities

- Stop serializing `InternalLLMRequest` / `InternalLLMResponse` into `request_content` / `response_content`.
- Persist pre-captured `request` and `response` payload strings provided by relay.
- Keep token/cost/attempt metrics logic unchanged.

## Data Flow

### Request Path

1. Read inbound body in relay.
2. Pass body to `inAdapter.TransformRequest(...)` for business processing.
3. Normalize body for logs:
   - Valid JSON: unmarshal and marshal back to compact JSON.
   - Invalid JSON: fallback to raw string.
4. Store normalized result as request payload for `request_content`.

### Non-stream Response Path

1. Upstream response -> `outAdapter.TransformResponse(...)` -> internal response.
2. Internal response -> `inAdapter.TransformResponse(...)` -> endpoint JSON bytes.
3. Write bytes to client.
4. Persist the exact same bytes into `response_content`.
5. On failure, persist exact client-facing error body into `response_content`.

### SSE Response Path

1. Continue live chunk forwarding behavior unchanged.
2. After stream completion:
   - Retrieve merged internal response from `inAdapter.GetInternalResponse(...)`.
   - Convert merged response once with `inAdapter.TransformResponse(...)`.
3. Persist resulting endpoint JSON as `response_content`.

## Error Handling and Edge Cases

- If response conversion fails during logging, fallback to best-effort text and do not change runtime response semantics.
- For upstream non-2xx, persist the exact body returned to client in `response_content`.
- If stream aborts early (disconnect/timeout), keep current behavior; persist available error body when present.
- Add optional payload size guard for very large JSON to reduce DB pressure.
- No schema migration needed (`request_content` / `response_content` reused).

## Testing Strategy

### Unit / Integration Coverage

- Request normalization:
  - valid JSON -> normalized compact JSON
  - invalid JSON -> raw fallback
- Non-stream success:
  - `response_content` equals client-facing transformed bytes
- Failure path:
  - `response_content` equals returned error body
- SSE merge:
  - OpenAI Chat endpoint stores final merged OpenAI JSON
  - Anthropic endpoint stores final merged Anthropic JSON
  - OpenAI Responses endpoint stores final merged Responses JSON
- Ensure token/cost/attempt metrics remain unchanged.

### Manual Smoke Checks

- Anthropic HTTP request:
  - request/response in log detail are Anthropic JSON
- Anthropic SSE request:
  - response shows merged final Anthropic JSON
- OpenAI endpoints:
  - request/response stay endpoint-specific and valid JSON in log detail

## Scope and Non-goals

In scope:
- Relay and metrics logging capture updates.
- Endpoint-accurate log content persistence for HTTP and SSE.

Out of scope:
- Frontend rendering redesign (existing JSON viewer remains).
- Broad transformer contract redesign.

