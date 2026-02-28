# OpenAI Adapters

> Package location: `internal/transformer2/adapter/openai/`

### 6.1 OpenAI Chat Completions Adapter (`adapter/openai/`)

Covers `/v1/chat/completions` (POST).

#### Request Conversion (`canonical.Request` -> OpenAI Chat wire format)

- **Count Tokens**: OpenAI `/v1/chat/completions` does not have a native token counting endpoint (typically done locally via tiktoken). `canonical.RequestKind == KindCountTokens` routed to OpenAI adapter should return an unsupported error or utilize a proxy-local tiktoken counter.

- **Messages**: `messages[]` with `role` = "system"|"developer"|"user"|"assistant"|"tool"
  - `developer` role maps to canonical `RoleDeveloper` (pass through)
  - `content` can be string or array of `{"type":"text",...}`, `{"type":"image_url",...}`, `{"type":"input_audio",...}`, `{"type":"file",...}`
- **Tools**: `tools[].{type:"function", function:{name, description, parameters, strict}}`
  - canonical `Tool` -> wrap in `{"type":"function","function":{...}}`
  - canonical `Tool.ImageGeneration` -> strip (not valid for Chat Completions)
- **ToolChoice**: `tool_choice` = "none"|"auto"|"required"|`{"type":"function","function":{"name":"..."}}`
  - canonical `ToolChoice.Mode` maps directly (`"required"` maps to `"required"`); `ToolChoice.Function` -> named choice
- **ParallelToolCalls**: `parallel_tool_calls` (top-level bool, defaults to true)
  - `false` → `ToolChoice.DisableParallelToolUse = ptr(true)` (create ToolChoice with `Mode:"auto"` if absent)
  - `DisableParallelToolUse == true` → emit `parallel_tool_calls: false`
- **Reasoning**: `reasoning_effort` field ("none"|"minimal"|"low"|"medium"|"high"|"xhigh")
  - canonical `ReasoningConfig.Effort` -> `reasoning_effort`
- **Streaming**: `stream: true` + `stream_options: {"include_usage": true}`
- **ResponseFormat**: `response_format` = `{"type":"text"}` | `{"type":"json_schema","json_schema":{...}}` | `{"type":"json_object"}`
- **Modalities**: `modalities` = `["text"]` | `["text","audio"]` — passthrough from canonical
- **Audio**: `audio: {format, voice}` — passthrough from canonical `AudioConfig`
- **N**: `n` — number of completions, passthrough
- **Passthrough fields**: `logit_bias`, `service_tier`, `prompt_cache_key`, `safety_identifier`, `user`, `store`, `metadata` — pass through from canonical `Request` to wire format

#### Response Conversion (OpenAI Chat wire format -> `canonical.Response`)

- **Non-streaming**: `choices[].{index, message:{role, content, tool_calls, refusal}, finish_reason, logprobs}`
  - `finish_reason`: "stop"|"length"|"tool_calls"|"content_filter" -> canonical as-is
  - `tool_calls[].{id, type:"function", function:{name, arguments}}` -> canonical `ToolCall`
  - `message.content` can be string -> wrap in single `ContentBlock{Type:ContentText}`
  - `message.audio` -> canonical audio content (if modalities includes audio)
- **Streaming**: SSE `data: {json}\n\n`, sentinel `data: [DONE]`
  - `data: [DONE]` → emit `Chunk{Done: true}` (replaces old `Object == "[DONE]"` pattern)
  - Each chunk: `choices[].{index, delta:{role, content, tool_calls, refusal}, finish_reason}`
  - `delta.tool_calls[].{index, id, function:{name, arguments}}` - incremental, concat by index
  - Final chunk with `stream_options.include_usage`: `usage` field populated
- **Usage**: `{prompt_tokens, completion_tokens, total_tokens, prompt_tokens_details:{cached_tokens, audio_tokens}, completion_tokens_details:{reasoning_tokens, audio_tokens, accepted_prediction_tokens, rejected_prediction_tokens}}`

#### Files

- `client_chat.go` - ClientAdapter: parse OpenAI Chat request body, format response
- `provider_chat.go` - ProviderAdapter: build HTTP request to OpenAI-compat backend, parse response
- `types_chat.go` - Wire types: `ChatCompletionRequest`, `ChatCompletionResponse`, `ChatCompletionChunk`


### 6.2 OpenAI Responses API Adapter (`adapter/openai/`)

Covers `/v1/responses` (POST). This is a **production-critical** endpoint used by clients that interact via the OpenAI Responses API format.

The Responses API has a fundamentally different wire format from Chat Completions:
- Input is `string | items[]` (not `messages[]`)
- System messages are extracted to `instructions`
- Tools include `image_generation` type (not just `function`)
- Streaming uses named events (`response.created`, `response.output_text.delta`, etc.) instead of generic `data:` chunks
- Response output is `output[]` items (not `choices[]`)

#### Client Adapter (`client_responses.go`)

Parses inbound `/v1/responses` requests and formats outbound responses in Responses API format.

**Request Parsing** (Responses wire → `canonical.Request`):
- `input` can be string or `items[]` array
  - String input → single user message with text content
  - Items array → convert each item by type:
    - `{role:"user", content:[{type:"input_text",...}, {type:"input_image",...}]}` → canonical user Message
    - `{type:"function_call", call_id, name, arguments}` → canonical assistant Message with ToolCall
    - `{type:"function_call_output", call_id, output}` → canonical tool Message
    - `{type:"message", role:"assistant", content:[{type:"output_text",...}]}` → canonical assistant Message
  - Set `Request.Hints.ResponsesArrayInput = true` when input is array format
- `instructions` → canonical system message (prepended to Messages)
- `tools[]`:
  - `{type:"function", name, description, parameters, strict}` → canonical `Tool`
  - `{type:"image_generation", background, output_format, quality, size}` → canonical `Tool{Type:"image_generation", ImageGeneration:&ImageGenerationConfig{...}}`
- `tool_choice` → canonical `ToolChoice` (mode-based: `"auto"|"none"|"required"` or `{type, name}`)
- `parallel_tool_calls` → canonical `ToolChoice.DisableParallelToolUse`
- `reasoning.effort` → canonical `ReasoningConfig.Effort`
- `include` → `Request.Hints.Include`
- `text.format` → canonical `ResponseFormat`
- `max_output_tokens`, `temperature`, `top_p`, `store`, `service_tier`, `user`, `metadata` → canonical fields

**Response Formatting** (non-streaming: `canonical.Response` → Responses wire):
- Convert canonical Choices back to Responses API `output[]` items
- Map `FinishReason` to Responses API `status` ("completed"|"incomplete"|"failed")
- Include usage in Responses API format

**Stream Formatting** (`canonical.Chunk` → Responses SSE events):

This is the most complex part (~900 lines). The Responses API uses a stateful event protocol:

| Canonical Chunk Content | Responses API Events Emitted |
| --- | --- |
| First chunk with role | `response.created` → `response.in_progress` → `response.output_item.added` → `response.content_part.added` |
| Text delta | `response.output_text.delta` |
| Tool call start | `response.output_item.added` (type:function_call) |
| Tool call args delta | `response.function_call_arguments.delta` |
| Reasoning content | `response.reasoning_summary_text.delta` |
| FinishReason set | Close content parts → close output items → `response.completed` |
| `Chunk.Done == true` | Emit final `[DONE]` |

The client adapter must track state:
- `hasResponseCreated`, `hasMessageItemStarted`, `hasContentPartStarted`
- `hasReasoningItemStarted`, `currentToolCallIndex`
- `sequenceNumber` (monotonically increasing per event)
- Accumulated chunks for `AggregateStream()`

**Error Formatting** (`canonical.Error` → Responses error JSON):
- `{"error":{"message":"...", "type":"...", "code":"...", "param":null}}`

#### Provider Adapter (`provider_responses.go`)

Builds outbound HTTP requests to OpenAI-compatible `/responses` backends and parses responses.

**Request Building** (`canonical.Request` → Responses wire → HTTP):
- `ConvertToResponsesRequest(req)` converts canonical to `ResponsesRequest`:
  - Extract system/developer messages → `instructions`
  - Convert remaining messages → `input` (string or items[])
  - If `Hints.ResponsesArrayInput == false` and single user text → use string input
  - Convert tools (including `image_generation` type)
  - Convert tool choice, reasoning, text format options
- POST to `{baseUrl}/responses`

**Response Parsing** (Responses wire → `canonical.Response`):
- Parse `output[]` items:
  - `{type:"message", content:[{type:"output_text", text}]}` → canonical text content
  - `{type:"function_call", call_id, name, arguments}` → canonical ToolCall
  - `{type:"reasoning", summary:[{type:"summary_text", text}]}` → canonical Reasoning
  - `{type:"image_generation_call", result}` → canonical image ContentBlock
- Map `status` → canonical `FinishReason` ("completed"→"stop", "incomplete"→"length", "failed"→"error")
- Parse `ResponsesUsage` → canonical Usage (with `input_tokens_details.cached_tokens`, `output_tokens_details.reasoning_tokens`)

**Stream Parsing** (Responses SSE events → `canonical.Chunk`):
- `response.created` / `response.in_progress` → initial chunk with ID/model
- `response.output_text.delta` → text content delta
- `response.function_call_arguments.delta` → tool call args delta
- `response.output_item.added` (type:function_call) → tool call start
- `response.reasoning_summary_text.delta` → reasoning delta
- `response.completed` → final chunk with FinishReason + Usage
- `response.failed` / `response.incomplete` / `error` → error chunk
- `[DONE]` → `Chunk{Done: true}`

#### Files

- `client_responses.go` - ClientAdapter: parse Responses API request, format stateful stream events
- `provider_responses.go` - ProviderAdapter: build HTTP request to Responses backend, parse response
- `types_responses.go` - Wire types: `ResponsesRequest`, `ResponsesResponse`, `ResponsesStreamEvent`, `ResponsesItem`, `ResponsesInput`, `ResponsesUsage`, etc.


### 6.3 Volcengine Adapter (`adapter/volcengine/`)

Wraps the **OpenAI Responses API** provider adapter (NOT Chat Completions). The current codebase's `volcengine/response.go` embeds `openai.ResponseOutbound` and posts to `/responses`.

Key differences from vanilla OpenAI Responses:
- Custom `Thinking` struct: `{type: "auto"|"disabled"|"enabled"}` mapped from `ReasoningConfig.Effort`
  - `"minimal"` → `ThinkingTypeDisabled`
  - `"low"|"medium"|"high"` → `ThinkingTypeEnabled`
- Strips `metadata` (Volcengine does not support it)
- Filters `reasoning` by a supported model allowlist (`doubao-seed-*`)
- Volcengine-specific `ResponsesInput` with `partial: true` on last assistant message
- Response/stream parsing delegates entirely to the inner OpenAI Responses adapter

#### Files

- `provider_responses.go` - ProviderAdapter wrapping `openai.ResponsesProviderAdapter`


