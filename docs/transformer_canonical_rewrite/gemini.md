# Gemini Adapter

> Package location: `internal/transformer2/adapter/gemini/`

### 6.4 Gemini Adapter (`adapter/gemini/`)

Covers Gemini `generateContent`, `streamGenerateContent`, and `countTokens` REST APIs.

#### Request Conversion (`canonical.Request` -> Gemini wire format)

- **Count Tokens**: `canonical.RequestKind == KindCountTokens` -> routes to `:countTokens`. Requests take the same `contents` array. Returns `canonical.Response{Usage: {TotalTokens: response.totalTokens}}`.

- **Contents**: `contents[].{role:"user"|"model", parts:[...]}`
  - canonical `RoleAssistant` -> Gemini `"model"`
  - canonical `RoleSystem` **and `RoleDeveloper`** messages -> extracted and merged into `system_instruction` field (Gemini has no `developer` role; treat as system)
  - `system_instruction` accepts a single `Content{parts}` — multiple system/developer messages must be concatenated (newline-separated) into one `{text}` part
  - Parts: `{text}`, `{inlineData:{mimeType, data}}`, `{fileData:{mimeType, fileUri}}`
  - `{functionCall:{name, args}}` (in model parts)
  - `{functionResponse:{name, response}}` (in user parts for tool results)
- **Tools**: `tools[].functionDeclarations[].{name, description, parameters}`
  - canonical `Tool.Parameters` (JSON Schema) -> Gemini `parameters` (map[string]any, unmarshal)
- **ToolConfig**: `toolConfig.functionCallingConfig.{mode:"AUTO"|"ANY"|"NONE", allowedFunctionNames}`
  - canonical `ToolChoice.Mode` -> uppercase (`"required"` maps to `"ANY"`); `ToolChoice.Function` -> `allowedFunctionNames`
- **GenerationConfig**: `generationConfig.{temperature, topP, topK, maxOutputTokens, stopSequences, responseMimeType, responseSchema}`
  - canonical `MaxTokens` -> `maxOutputTokens`
  - canonical `ResponseFormat{type:"json_schema"}` -> `responseMimeType:"application/json"` + `responseSchema`
- **ThinkingConfig**: `generationConfig.thinkingConfig.{thinkingLevel:"minimal"|"low"|"medium"|"high", thinkingBudget}`
  - canonical `ReasoningConfig.Effort` -> Gemini `thinkingLevel` (direct lowercase match for low/medium/high; canonical "none" -> omit config)
  - canonical `ReasoningConfig.BudgetTokens` -> `thinkingBudget`
  - Gemini 3 defaults to `thinkingLevel: "high"` if not specified
- **MediaResolution**: `generationConfig.mediaResolution` = `"media_resolution_low"` | `"medium"` | `"high"` | `"ultra_high"`
  - canonical `Request.MediaResolution` -> Gemini `mediaResolution` (prefix with `"media_resolution_"`)
  - Controls tokens per input image/video frame
- **Thought Signatures** (Gemini 3): `parts[].thoughtSignature` - opaque signature for reasoning continuity
  - Preserve in canonical `Message.ReasoningSignature`
  - **Strict validation for function calls**: missing `thoughtSignature` on `functionCall` parts causes **400 error**
  - For parallel function calls: only the **first** `functionCall` part carries the signature
  - **Round-trip problem**: when flattening parallel `functionCall` parts into canonical `ToolCall[]`, the signature attached to the first part is lost unless explicitly preserved. Solution: `ToolCall.Signature *string` (see canonical.md) — Gemini adapter sets it on the first ToolCall; when converting back to Gemini parts, the adapter writes `thoughtSignature` on the corresponding part.
  - For multi-step sequential calls: all accumulated signatures must be returned in history
  - Text/Chat: not strictly validated but recommended for reasoning quality
  - **Migration dummy**: `"context_engineering_is_the_way to_go"` can bypass strict validation when no real signature exists
  - **URL construction**: `POST {baseUrl}/v1beta/models/{model}:generateContent` (non-streaming), `:streamGenerateContent?alt=sse` (streaming), or `:countTokens` (count tokens)

#### Response Conversion (Gemini wire format -> `canonical.Response`)

- **Non-streaming**: `{candidates[].{content:{role, parts}, finishReason, index}, usageMetadata, modelVersion}`
  - Parts: `{text}`, `{functionCall:{name, args}}`, `{thought:true, text}`, `{thoughtSignature}`
  - `finishReason`: "STOP"|"MAX_TOKENS"|"SAFETY"|"RECITATION"|"OTHER"|"BLOCKLIST"|"PROHIBITED_CONTENT"|"SPII"
    - Map: "STOP"->"stop", "MAX_TOKENS"->"length", "SAFETY"/"BLOCKLIST"/"PROHIBITED_CONTENT"/"SPII"->"content_filter"
  - `parts[type:functionCall]` -> canonical `ToolCall{Name, Arguments:marshal(args)}`
  - `parts[thought:true]` -> canonical `Message.Reasoning`
  - `parts[thoughtSignature]` -> canonical `Message.ReasoningSignature`
  - `parts[thoughtSummary]` -> canonical `Message.ThoughtSummary`
- **Streaming**: SSE `data: {json}\n\n` via `streamGenerateContent?alt=sse`
  - Each chunk is a full `GenerateContentResponse` with partial `candidates` (for REST) or `content.delta` events (for interactions API)
  - Incremental: new parts appended; concat text parts across chunks. For `content.delta` events, map `thought_signature` and `thought_summary`.
  - No `[DONE]` sentinel; stream ends when connection closes
  - `usageMetadata` arrives in final chunk (or `interaction.complete` event)
  - **CRITICAL — empty-text chunks**: `thoughtSignature` may arrive in a chunk with `text == ""`. `convert.go` must NOT discard chunks solely because text is empty — always check for `thoughtSignature` (and `thoughtSummary`) before deciding to skip a chunk.
  - **Aggregator rule**: `aggregator.go` must apply **last-wins** for `ReasoningSignature` — if a later chunk carries a non-nil signature, it overwrites the accumulated value, even if that chunk's text is empty.
- **Usage**: `usageMetadata.{promptTokenCount, candidatesTokenCount, totalTokenCount, thoughtsTokenCount, cachedContentTokenCount}`
  - Map: `promptTokenCount` -> `PromptTokens`, `candidatesTokenCount` -> `CompletionTokens`
  - `thoughtsTokenCount` -> `CompletionTokensDetails.ReasoningTokens`
  - `cachedContentTokenCount` -> `PromptTokensDetails.CachedTokens`

#### Files

- `provider.go` - ProviderAdapter (Gemini is provider-only, no client adapter needed)
- `types.go` - Gemini wire types (ported from old `transformer/model/gemini.go`)
- `convert.go` - Conversion helpers

