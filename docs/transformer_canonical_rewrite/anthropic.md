# Anthropic Adapter

> Package location: `internal/transformer2/adapter/anthropic/`

### 6.3 Anthropic Adapter (`adapter/anthropic/`)

Covers `/v1/messages` (POST) and `/v1/messages/count_tokens` (POST).

#### Request Conversion (`canonical.Request` -> Anthropic wire format)

- **Count Tokens**: `canonical.RequestKind == KindCountTokens` -> route to `/v1/messages/count_tokens` (request body identical to `/v1/messages`). Returns `canonical.Response{Usage: {PromptTokens: response.input_tokens}}`.

- **Messages**: `messages[].{role:"user"|"assistant", content}` - alternating turns required
  - canonical `RoleSystem` **and `RoleDeveloper`** messages -> extracted and merged into top-level `system` field (Anthropic has no `developer` role; treat as system)
  - Multiple system/developer messages → concatenate text (newline-separated) into a single string, or build `[{type:"text", text, cache_control}]` array if any block has `cache_control`
  - **CRITICAL RoleTool Aggregation**: canonical `RoleTool` messages represent tool results. When encountering one or more continuous `RoleTool` messages, they **MUST** be folded/aggregated into a single Anthropic `user` message. Its `content` array will contain all the respective `{"type": "tool_result", ...}` blocks. This ensures compliance with Anthropic's strict "no continuous same role" and "no tool role" rules.
  - `system` can be string or `[{type:"text", text, cache_control}]`
  - `content` = string or array of `MessageContentBlock`:
    - `{type:"text", text, cache_control, citations}`
    - `{type:"image", source:{type:"base64"|"url", media_type, data|url}}`
    - `{type:"document", source:{type:"base64"|"text"|"url"|"content_block",...}, title, context}`
    - `{type:"tool_use", id, name, input}` (in assistant messages)
    - `{type:"tool_result", tool_use_id, content, is_error}` (in user messages)
    - `{type:"thinking", thinking, signature}` (in assistant messages)
    - `{type:"redacted_thinking", data}` (in assistant messages)
    - `{type:"server_tool_use"|"web_search_tool_result"|"web_fetch_tool_result"|"code_execution_tool_result"...}` — server tool blocks, passthrough as-is
- **Tools**: `tools[].{name, description, input_schema, cache_control, strict}` — client tools
  - canonical `Tool.Parameters` -> `input_schema`
  - `strict: true` -> canonical `Tool.Strict` (shared with OpenAI; guarantees schema validation on tool names and inputs)
  - No `type` field (Anthropic infers it for client tools)
- **Server Tools**: passthrough for built-in tools that Claude Code uses:
  - `{type:"web_search_20250305", name, ...}` — web search
  - `{type:"web_fetch_20250305", name, ...}` — web fetch
  - `{type:"code_execution_20250522", name, ...}` — code execution
  - `{type:"text_editor_20250429", name, ...}` — text editor
  - `{type:"bash_20250611", name, ...}` — bash execution  
  - These just need passthrough in the proxy — stored as `json.RawMessage` in canonical `Tool`
- **ToolChoice**: `tool_choice: {type:"auto"|"none"|"tool"|"any", name, disable_parallel_tool_use}`
  - canonical `ToolChoice.Mode == "required"` -> maps to Anthropic `{"type": "any"}`
  - `disable_parallel_tool_use: true` ↔ canonical `ToolChoice.DisableParallelToolUse = ptr(true)` (bidirectional 1:1)
- **Thinking** (three modes):
  - `thinking: {type:"adaptive"}` — **recommended for Opus 4.6 / Sonnet 4.6**
    - canonical `ReasoningConfig.Enabled = nil`
    - Automatically enables interleaved thinking (thinking blocks between tool calls)
    - No `budget_tokens` needed; effort is controlled via `output_config.effort`
  - `thinking: {type:"enabled", budget_tokens:N}` — legacy manual mode
    - canonical `ReasoningConfig.Enabled = ptr(true)` + `ReasoningConfig.BudgetTokens = N`
    - **DEPRECATED on Opus 4.6 / Sonnet 4.6** (still works, will be removed in future)
    - Required for older models (Sonnet 4.5, Opus 4.5, etc.) that don't support adaptive
  - `thinking: {type:"disabled"}` or omit `thinking`
    - canonical `ReasoningConfig.Enabled = ptr(false)` or `ReasoningConfig = nil`
  - **Adapter logic** for BuildRequest:
    - If `Enabled == nil` (adaptive): emit `thinking: {type:"adaptive"}`
    - If `Enabled == true`: emit `thinking: {type:"enabled", budget_tokens: BudgetTokens}`. If `BudgetTokens` is nil, derive from `Effort` via lookup table.
    - If `Enabled == false` or `ReasoningConfig == nil`: omit `thinking` field
- **output_config.effort** — controls thinking intensity (adaptive mode):
  - `output_config: {effort: "low"|"medium"|"high"|"max"}`
  - **"max" is Opus 4.6 only** — other models return error
  - canonical `ReasoningConfig.Effort` → Anthropic `output_config.effort`
  - Only emitted when `thinking.type == "adaptive"` (ignored in legacy/disabled mode)
  - When `Effort` is nil, Anthropic defaults to `"high"`
- **Interleaved thinking** (adaptive mode):
  - With adaptive mode, thinking blocks can appear **between tool calls**, not just at the start of a turn.
  - The proxy must handle `content[]` with interleaved `{type:"thinking"}` blocks between `{type:"tool_use"}` and `{type:"text"}` blocks.
  - Streaming: `thinking_delta` events can arrive between `content_block_stop` (tool_use) and `content_block_start` (text).
  - The canonical `Message.Reasoning` field is insufficient for interleaved thinking — thinking content should be modeled as part of `Message.Content []ContentBlock` or as separate `ServerToolBlocks`-style passthrough when interleaving matters. For now, the proxy should preserve interleaved thinking blocks in message round-trip by keeping them in `Message.Content` as `ContentBlock{Type: ContentThinking}` when the source format is Anthropic.
- **Summarized thinking** (Claude 4 models):
  - Claude 4 models return a **summary** of thinking, not full thinking text.
  - The `signature` field carries encrypted full thinking for verification/round-trip.
  - Billed output tokens = full thinking tokens (not summary tokens). Token counts in responses won't match visible text.
  - The proxy does not need special handling — `thinking` text and `signature` are passed through as-is. But logging/metrics should note that visible thinking text length ≠ billed tokens.
- **MaxTokens**: `max_tokens` is REQUIRED in Anthropic (not optional like OpenAI). Acts as hard limit on total output (thinking + response text).
- **OutputConfig**: `output: {format: {type:"json", schema:{...}}}` — Anthropic structured output
  - canonical `OutputConfig` -> Anthropic `output` field (distinct from `output_config`)

#### Response Conversion (Anthropic wire format -> `canonical.Response`)

- **Non-streaming**: `{id, type:"message", role, content:[], model, stop_reason, usage}`
  - `content[]` blocks: `{type:"text", text, citations}`, `{type:"tool_use", id, name, input}`, `{type:"thinking", thinking, signature}`, `{type:"redacted_thinking", data}`
  - Server tool response blocks (web_search_tool_result, etc.) -> `Message.ServerToolBlocks` passthrough
  - `stop_reason`: "end_turn"|"max_tokens"|"stop_sequence"|"tool_use"|"pause_turn"|"refusal"
    - Map: "end_turn"->"stop", "max_tokens"->"length", "tool_use"->"tool_calls", "pause_turn"->"stop", "refusal"->"content_filter"
    - **"stop_sequence"->"stop"** + preserve `response.stop_sequence` string → canonical `Choice.StopSequence`
  - `content[type:"tool_use"]` -> canonical `ToolCall{ID, Name, Arguments:marshal(input)}`
  - `content[type:"thinking"]` -> canonical `Message.Reasoning` + `Message.ReasoningSignature`
  - `content[type:"redacted_thinking"]` -> canonical `Message.RedactedThinking`
- **Streaming**: Anthropic-specific SSE event types:
  - `event: message_start` -> `{message:{id, model, usage:{input_tokens}}}`
  - `event: content_block_start` -> `{index, content_block:{type, id, name, ...}}`
  - `event: content_block_delta` -> `{index, delta:{type:"text_delta"|"input_json_delta"|"thinking_delta"|"signature_delta", ...}}`
  - `event: content_block_stop` -> `{index}`
  - `event: message_delta` -> `{delta:{stop_reason, stop_sequence}, usage:{output_tokens}}`
    - When `stop_reason == "stop_sequence"`: extract `delta.stop_sequence` → canonical `Choice.StopSequence`
  - `event: message_stop`
  - Key: `input_json_delta` carries incremental tool call args (concat `partial_json`)
  - Key: `message_start` carries `usage.input_tokens` (and cache stats); `message_delta` carries `usage.output_tokens`. **CRITICAL**: The canonical aggregator must MERGE `Usage` across chunks (e.g. `aggregator.Usage.CompletionTokens += chunk.Usage.CompletionTokens`), otherwise prompt tokens from the first chunk are lost.
  - Key: server tool content blocks also stream via content_block_start/delta/stop
- **Usage**: `{input_tokens, output_tokens, cache_creation_input_tokens, cache_read_input_tokens, cache_creation, server_tool_usage:{web_search_requests, web_fetch_requests}}`
  - **CRITICAL CACHING SEMANTICS**: When caching is enabled, `input_tokens` only represents tokens AFTER the last cache breakpoint, NOT total input tokens.
  - Correct formula: `PromptTokens = cache_read_input_tokens + cache_creation_input_tokens + input_tokens`
  - Map `input_tokens` -> `Usage.InputTokensAfterBreakpoint` (preserve for logging/debugging)
  - Map `output_tokens` -> `CompletionTokens`
  - `cache_creation_input_tokens` + `cache_read_input_tokens` -> canonical passthrough fields
  - `cache_creation` (optional nested object) -> `Usage.CacheCreation`:
    - `{ephemeral_5m_input_tokens, ephemeral_1h_input_tokens}` — breakdown by TTL
    - Only present when mixing 5m and 1h cache TTLs in the same request
    - `cache_creation_input_tokens == ephemeral_5m_input_tokens + ephemeral_1h_input_tokens`
    - Needed for accurate cost calculation: 5m writes = 1.25x base, 1h writes = 2x base
  - `server_tool_usage` -> canonical `ServerToolUsage`
  - See: https://docs.anthropic.com/en/docs/build-with-claude/prompt-caching#tracking-cache-performance

#### CacheControl Passthrough

- Anthropic `cache_control: {type:"ephemeral", ttl:"5m"|"1h"}` on messages, tools, content blocks
- Preserved via canonical `CacheControl` struct, only emitted when formatting back to Anthropic

#### CRITICAL: `anthropic-beta` Header Passthrough

Anthropic enforces Beta feature gating via HTTP request headers. Without the correct `anthropic-beta` header, server tools (bash, text_editor, code_execution, etc.) and extended thinking (`budget_tokens`) will fail at the API level.

Known required beta headers (values evolve over time):
- `computer-use-2024-10-22` — computer use tools
- `prompt-caching-2024-07-31` — prompt caching
- Server tools like `bash_20250611`, `text_editor_20250429` each require their corresponding beta flag
- Extended thinking with `budget_tokens` may require a beta flag depending on model version

**Implementation — two phases across client.go and provider.go:**

**Phase 1 — `client.go` `ParseRequest(ctx, body, header)`** (extract):
- Extract `anthropic-beta` and `anthropic-version` from inbound `http.Header`
- Store into `canonical.Request.Headers`

**Phase 2 — `provider.go` `BuildRequest(ctx, req, baseUrl, key)`** (forward + supplement):
1. **Forward**: Copy `req.Headers["Anthropic-Beta"]` and `req.Headers["Anthropic-Version"]` onto the outbound `*http.Request`.
2. **Auto-detect & append** missing beta values by inspecting canonical request:
   - `Request.Reasoning.BudgetTokens != nil` → append `extended-thinking` beta
   - Any `Tool.RawJSON` containing server tool type (e.g. `bash_20250611`, `text_editor_20250429`, `code_execution_20250522`) → append corresponding beta flag
   - Any message/tool with `CacheControl` set → conditionally append `prompt-caching-2024-07-31` beta flag
     - **CRITICAL CONDITION**: Only append `prompt-caching-2024-07-31` for older Claude 3 models (e.g., `claude-3-opus-*`, `claude-3-sonnet-*`, `claude-3-haiku-*`). For Claude 3.5+ and Claude 4+ where prompt caching is GA, **DO NOT** append this beta flag to avoid interference.
3. **Merge**: Deduplicate all beta values (passthrough + auto-detected), set as comma-separated `anthropic-beta` header.
4. **`anthropic-version`**: Forward if present; otherwise default to `2023-06-01`.

#### Files

- `client.go` - ClientAdapter for `/v1/messages` format
- `provider.go` - ProviderAdapter for Anthropic backends (including beta header logic)
- `types.go` - Wire types: `MessageRequest`, `MessageParam`, `MessageContentBlock`, `StreamEvent`, etc.
- `convert.go` - Conversion helpers + cache_control + thinking handling

