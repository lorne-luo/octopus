# Cross-Cutting Conversion Matrix


### 7.1 FinishReason Mapping

- canonical: "stop", "length", "tool_calls", "content_filter"
- OpenAI Chat: "stop", "length", "tool_calls", "content_filter", "function_call" (deprecated)
- OpenAI Responses: status-based mapping — "completed"->"stop", "incomplete"->"length", "failed"->error handling
- Anthropic: "end_turn"->"stop", "max_tokens"->"length", "tool_use"->"tool_calls", "stop_sequence"->"stop" (+ preserve `stop_sequence` string in `Choice.StopSequence`), "pause_turn"->"stop", "refusal"->"content_filter"
- Gemini: "STOP"->"stop", "MAX_TOKENS"->"length", "SAFETY"->"content_filter", "RECITATION"->"content_filter", "BLOCKLIST"->"content_filter", "PROHIBITED_CONTENT"->"content_filter", "SPII"->"content_filter"

### 7.2 Reasoning/Thinking Mapping

#### Mode (ReasoningConfig.Enabled)

| Canonical | OpenAI | Anthropic | Gemini |
| --- | --- | --- | --- |
| `Enabled = nil` (adaptive) | N/A (use Effort) | `thinking: {type: "adaptive"}` | N/A (use Effort) |
| `Enabled = true` (manual) | N/A (use Effort) | `thinking: {type: "enabled", budget_tokens: N}` | N/A (use Effort) |
| `Enabled = false` (off) | N/A | omit `thinking` | N/A |

#### Effort (ReasoningConfig.Effort)

| Canonical | OpenAI | Anthropic (adaptive) | Anthropic (legacy) | Gemini |
| --- | --- | --- | --- | --- |
| `"none"` | `reasoning_effort: "none"` | omit thinking | omit thinking | omit thinkingConfig |
| `"minimal"` | `reasoning_effort: "minimal"` | `output_config.effort: "low"` | `budget_tokens: 1024` | `thinkingLevel: "minimal"` |
| `"low"` | `reasoning_effort: "low"` | `output_config.effort: "low"` | `budget_tokens: 5000` | `thinkingLevel: "low"` |
| `"medium"` | `reasoning_effort: "medium"` | `output_config.effort: "medium"` | `budget_tokens: 15000` | `thinkingLevel: "medium"` |
| `"high"` | `reasoning_effort: "high"` | `output_config.effort: "high"` | `budget_tokens: 30000` | `thinkingLevel: "high"` |
| `"xhigh"` | `reasoning_effort: "xhigh"` | `output_config.effort: "max"` | `budget_tokens: 60000` | `thinkingLevel: "high"` |
| `"max"` | `reasoning_effort: "xhigh"` | `output_config.effort: "max"` (**Opus 4.6 only**) | N/A | `thinkingLevel: "high"` |

Notes:
- Anthropic `output_config.effort` is only emitted when `thinking.type == "adaptive"`
- Anthropic `"max"` effort is Opus 4.6 only; other models return error
- When no Effort is set, Anthropic adaptive defaults to `"high"`
- `budget_tokens` is **deprecated** on Opus 4.6 / Sonnet 4.6; use adaptive + effort instead

#### Budget (ReasoningConfig.BudgetTokens)

- canonical `ReasoningConfig{BudgetTokens:8192}` ->
  - Anthropic (legacy): `thinking: {type: "enabled", budget_tokens: 8192}`
  - Gemini: `generationConfig.thinkingConfig.thinkingBudget: 8192`
  - OpenAI: ignored (no budget concept)
  - Anthropic (adaptive): ignored (use Effort instead)

#### Response Thinking Content

- Thinking output -> canonical `Message.Reasoning` + `Message.ReasoningSignature`
- Anthropic `redacted_thinking` -> canonical `Message.RedactedThinking` (opaque passthrough)
- **Summarized thinking** (Claude 4): `thinking` text is a summary, `signature` carries encrypted full thinking. Proxy passes through as-is; billed tokens ≠ visible text.

#### Interleaved Thinking (Anthropic Adaptive Mode)

With `thinking.type: "adaptive"`, thinking blocks can appear **between** tool use blocks within a single turn (interleaved thinking). This is different from manual mode where thinking only appears at the start.

Example response content order: `[thinking, text, tool_use, thinking, tool_use, thinking, text]`

The proxy must:
1. **Preserve interleaved order** when routing Anthropic→Anthropic (passthrough)
2. **Collapse to single reasoning** when routing Anthropic→OpenAI (OpenAI has no interleaved concept)
3. **Handle streaming**: `thinking_delta` events can arrive between `content_block_stop`(tool_use) and `content_block_start`(text)

For canonical representation, interleaved thinking blocks should be preserved as `ContentBlock{Type: ContentThinking}` within `Message.Content` to maintain ordering. The existing `Message.Reasoning *string` field is a convenience for the common case (single thinking block at start).

### 7.6 Server/Built-in Tools Passthrough Strategy

- **As a proxy, server-side/built-in tools should be passed through transparently without conversion.**
- Anthropic server tools (web_search, web_fetch, code_execution, bash, text_editor): stored as `json.RawMessage` in canonical `Tool.RawJSON`, forwarded to Anthropic backend as-is
- Server tool response blocks in messages: stored in `Message.ServerToolBlocks` as `[]json.RawMessage`
- This avoids needing to model every server tool's complex schema in canonical types

### 7.7 Provider-Specific HTTP Header Passthrough

Some providers require critical metadata in HTTP headers (not the JSON body). The proxy must preserve and/or generate these headers.

- **Anthropic `anthropic-beta`**: Comma-separated list of beta feature flags. Required for server tools (bash, text_editor, code_execution), extended thinking (budget_tokens), prompt caching, etc. Missing this header causes hard API errors.
- **Anthropic `anthropic-version`**: API version string (e.g. `2023-06-01`). Should be forwarded if present, or set to a default.
- **OpenAI**: No equivalent header-level feature gating currently; all config is in the JSON body.
- **Gemini**: No equivalent header-level feature gating; uses URL path for API version.

**Strategy** (two-phase — extract then forward+supplement):

1. **Extract** (`ClientAdapter.ParseRequest`): receives `http.Header` from the relay layer, picks out provider-relevant headers, and stores them in `canonical.Request.Headers http.Header`.
   - Anthropic adapter: extracts `anthropic-beta`, `anthropic-version`
   - OpenAI adapter: no header extraction needed (all config in JSON body)
2. **Forward + supplement** (`ProviderAdapter.BuildRequest`): reads `Request.Headers`, copies them onto the outbound `*http.Request`, then inspects canonical request content to auto-generate any missing required headers.
   - Anthropic: merge passthrough `anthropic-beta` values with auto-detected beta flags (from server tools / thinking config / cache_control), deduplicate.
   - Others: no-op or simple copy.

### 7.8 Error Format Mapping

The proxy must return errors in the format the **client** expects, regardless of which provider produced the error (or if the proxy itself errored). `ClientAdapter.FormatError` handles this conversion.

- **canonical `Error{Code, Message, Type, StatusCode}`** ->
  - OpenAI Chat: `{"error":{"message":"...", "type":"...", "code":"...", "param":null}}`
  - OpenAI Responses: `{"error":{"message":"...", "type":"...", "code":"...", "param":null}}` (same as Chat)
  - Anthropic: `{"type":"error", "error":{"type":"...", "message":"..."}}`
  - Gemini: N/A (Gemini is provider-only, no client adapter)
- **Error sources**:
  - Provider returns 4xx/5xx: `ProviderAdapter.ParseResponse` populates `Response.Error` from provider's error JSON; relay calls `ClientAdapter.FormatError` to re-serialize in client format.
  - Proxy internal error (channel exhausted, timeout, etc.): relay constructs `canonical.Error` directly, then calls `FormatError`.
- **Why this matters**: Claude Code crashes if it receives an OpenAI-shaped error when it expects Anthropic format. The proxy must always match the client's wire format, even for errors.

### 7.9 Role Mapping & Developer/System Fallback

canonical roles: `system`, `developer`, `user`, `assistant`, `tool`

- **OpenAI Chat**: all five roles natively supported. `developer` passes through as-is (used by o1/o3-mini).
- **Anthropic**: only `user` and `assistant` in `messages[]`.
  - Both `RoleSystem` and `RoleDeveloper` must be **extracted and merged** into the top-level `system` field. Multiple system/developer messages → concatenate text (newline-separated) into a single `system` string, or build a `[{type:"text", text, cache_control}]` array if any block carries `cache_control`.
  - **Tool Result Aggregation**: Anthropic API strictly requires alternating `user`/`assistant` turns and lacks a `tool` role. Continuous canonical `RoleTool` messages (e.g. from parallel tool calls) **MUST** be aggregated into a single `user` message, where the `content` is an array of `tool_result` blocks. Failing to fold continuous `RoleTool` messages will trigger a 400 validation error in Anthropic's API.
- **Gemini**: only `user` and `model` in `contents[]`. Both `RoleSystem` and `RoleDeveloper` must be **extracted and merged** into the top-level `system_instruction` field.
  - `system_instruction` accepts a single `Content{parts}` structure. Multiple system/developer messages → concatenate all text into one `{text}` part (newline-separated).
  - Canonical `RoleTool` messages -> map to `user` parts containing `functionResponse`, concatenated continuously if needed (Gemini allows multiple parts in a `user` turn).

**Rule**: when the target provider lacks a `developer` role, treat `RoleDeveloper` identically to `RoleSystem`. When the provider prohibits continuous turns or tool roles (like Anthropic), the adapter `BuildRequest` must fold all continuous canonical `RoleTool` messages into a single `user` message.

### 7.10 Parallel Tool Calls Mapping

Canonical location: `ToolChoice.DisableParallelToolUse *bool`

- **OpenAI Chat** ↔ canonical:
  - Inbound: `parallel_tool_calls: false` (top-level request field) → `ToolChoice.DisableParallelToolUse = ptr(true)`. If no `ToolChoice` exists yet, create one with `Mode: "auto"`.
  - Outbound: `DisableParallelToolUse == true` → `parallel_tool_calls: false`. If nil or false → omit field (OpenAI defaults to true).
- **Anthropic** ↔ canonical:
  - Inbound: `tool_choice.disable_parallel_tool_use: true` → `ToolChoice.DisableParallelToolUse = ptr(true)`. Direct 1:1 mapping.
  - Outbound: `DisableParallelToolUse == true` → `tool_choice: {..., disable_parallel_tool_use: true}`.
- **Gemini**: no equivalent concept. Field is ignored.

**Why this matters**: Claude Code explicitly disables parallel tool calls to ensure sequential file modifications. If the proxy drops this flag when routing to a different provider, the model may issue parallel tool calls that corrupt file state.

### 7.3 Tool Call Arguments Format

- OpenAI/Anthropic tool call args: JSON string
- Gemini functionCall args: `map[string]any`
- Conversion: Gemini->canonical: `json.Marshal(args)` to string; canonical->Gemini: `json.Unmarshal` to map

### 7.4 Image Input Mapping

- canonical `MediaContent{URL:"https://...", Detail:"high"}` ->
  - OpenAI Chat: `{type:"image_url", image_url:{url, detail:"high"}}`
  - Anthropic: `{type:"image", source:{type:"url", url}}` (no detail param)
  - Gemini: `{fileData:{fileUri, mimeType}}` or `{inlineData:{data, mimeType}}` for base64
- canonical `MediaContent{Base64:"...", MimeType:"image/jpeg"}` ->
  - OpenAI Chat: `{type:"image_url", image_url:{url:"data:image/jpeg;base64,..."}}`
  - Anthropic: `{type:"image", source:{type:"base64", media_type:"image/jpeg", data:"..."}}`
  - Gemini: `{inlineData:{mimeType:"image/jpeg", data:"..."}}`

### 7.5 Usage Token Mapping

- canonical `Usage{PromptTokens:100, CompletionTokens:50}` ->
  - OpenAI Chat: `{prompt_tokens:100, completion_tokens:50, total_tokens:150}`
  - OpenAI Responses: `{input_tokens:100, output_tokens:50, total_tokens:150, input_tokens_details:{cached_tokens}, output_tokens_details:{reasoning_tokens}}`
  - Anthropic: `{input_tokens:100, output_tokens:50}`
  - Gemini: `{promptTokenCount:100, candidatesTokenCount:50, totalTokenCount:150}`

### 7.11 Stream Termination Signal

The old architecture uses `InternalLLMResponse.Object == "[DONE]"` as a sentinel in the relay streaming loop. The new architecture replaces this with `canonical.Chunk.Done bool`.

| Provider | Termination Signal | Chunk Emitted |
| --- | --- | --- |
| OpenAI Chat | `data: [DONE]` | `Chunk{Done: true}` |
| OpenAI Responses | `[DONE]` (same SSE) | `Chunk{Done: true}` |
| Anthropic | `event: message_stop` | `Chunk{Done: true}` |
| Gemini | Connection close (no sentinel) | `Chunk{Done: true}` (emitted by adapter on EOF) |

Relay loop changes from:
```go
if stream.Object == "[DONE]" { break }
```
to:
```go
if chunk.Done { break }
```

### 7.12 Image Generation Passthrough

Image generation is only supported via the OpenAI Responses API (`/v1/responses`).

- **Inbound** (Responses client): `tools[].{type:"image_generation", background, output_format, quality, size}` → canonical `Tool{Type:"image_generation", ImageGeneration:&ImageGenerationConfig{...}}`
- **Outbound** (Responses provider): canonical `Tool{Type:"image_generation"}` → Responses wire `{type:"image_generation", ...}`
- **Response**: `output[].{type:"image_generation_call", result:"base64data"}` → canonical `Message.Images` or `ContentBlock{Type:ContentImage, Media:{Base64:..., MimeType:...}}`
- **Chat Completions**: `Tool.ImageGeneration` is stripped before serialization (not valid for Chat API). The current code explicitly sets `ImageGeneration = nil` in `Tool.MarshalJSON()`.
- **Anthropic/Gemini**: image generation tool type not applicable; adapters should ignore or error.

### 7.13 Error Format in Relay Flow

The relay must match error format to the **client** adapter, not the provider:

```
Provider returns error
    → ProviderAdapter.ParseResponse() returns canonical.Response with Error field set
    → OR returns error directly
    → Relay constructs canonical.Error{Code, Message, Type, StatusCode}
    → Relay calls clientAdapter.FormatError(ctx, &canonicalError)
    → Relay writes formatted bytes to client with correct HTTP status
```

Error format per client:
- **OpenAI Chat/Responses**: `{"error":{"message":"...", "type":"...", "code":"...", "param":null}}`
- **Anthropic**: `{"type":"error", "error":{"type":"...", "message":"..."}}`

This replaces the current pattern where outbound adapters return `*model.ResponseError` which the relay returns directly (wrong format when client ≠ provider).

### 7.14 OpenAI-Specific Passthrough Fields

These fields exist only in OpenAI wire format and are passed through canonical `Request` without transformation:

| Field | Canonical Location | Notes |
| --- | --- | --- |
| `logit_bias` | `Request.LogitBias` | Token bias map, OpenAI-only |
| `service_tier` | `Request.ServiceTier` | "auto", "default" |
| `prompt_cache_key` | `Request.PromptCacheKey` | OpenAI cache optimization |
| `safety_identifier` | `Request.SafetyIdentifier` | Abuse detection |
| `user` | `Request.User` | End-user ID |
| `store` | `Request.Store` | Distillation/eval storage |
| `metadata` | `Request.Metadata` | Key-value metadata |

Non-OpenAI provider adapters silently ignore these fields. Non-OpenAI client adapters do not populate them.

### 7.15 Qwen/Alibaba EnableThinking Field

The current codebase has `EnableThinking *bool` for Alibaba Qwen models. This is distinct from the standard reasoning config:
- canonical `Request.EnableThinking` → passed through to Qwen-compatible backends as `enable_thinking: true`
- Other providers: ignored
- This field should be emitted by OpenAI Chat provider adapter when the model name matches a Qwen pattern

### 7.16 Round-Trip Fidelity via TransformHints

Several fields in `canonical.Request.Hints` exist solely to preserve inbound wire format for accurate outbound reconstruction:

| Hint Field | Set By | Used By | Purpose |
| --- | --- | --- | --- |
| `ResponsesArrayInput` | OpenAI Responses client | OpenAI Responses provider | Preserve string vs array input format |
| `Include` | OpenAI Responses client | OpenAI Responses provider | Preserve `include` field |
| `AnthropicSystemArrayFormat` | Anthropic client | Anthropic provider | Preserve system prompt as array vs string |
| `GeminiTopK` | Gemini client (N/A) | Gemini provider | Preserve TopK (not in standard canonical params) |
| `GeminiSafetySettings` | Gemini client (N/A) | Gemini provider | Preserve safety settings as raw JSON |

These are adapter-internal concerns and should NOT be accessed by relay or metrics code.

### 7.17 ExtraBody Escape Hatch

`canonical.Request.ExtraBody json.RawMessage` carries opaque JSON from the client that gets merged into the outbound request body. This supports provider-specific extensions without canonical model changes.

Provider adapters should `json.Unmarshal` ExtraBody and merge keys into the outbound wire type before serialization. Unknown keys from ExtraBody take precedence over canonical-derived fields (user intent).

### 7.18 API Format Constants

The current codebase defines format constants used for routing and logging:

| Current Constant (in `internal/transformer`) | New Location (in `internal/transformer2/canonical`) | Status |
| --- | --- | --- |
| `APIFormatOpenAIChatCompletion` | `canonical.APIFormatOpenAIChat` | Rename |
| `APIFormatOpenAIResponse` | `canonical.APIFormatOpenAIResponse` | Keep |
| `APIFormatOpenAIEmbedding` | `canonical.APIFormatOpenAIEmbedding` | Keep |
| `APIFormatOpenAIImageGeneration` | `canonical.APIFormatOpenAIImageGeneration` | Keep |
| `APIFormatAnthropicMessage` | `canonical.APIFormatAnthropic` | Rename |
| `APIFormatGeminiContents` | `canonical.APIFormatGemini` | Rename |
| `APIFormatAiSDKText` | `canonical.APIFormatAiSDKText` | Keep (future use) |
| `APIFormatAiSDKDataStream` | `canonical.APIFormatAiSDKDataStream` | Keep (future use) |

AI SDK formats have no adapter implementations yet but the constants should be preserved for forward compatibility.

