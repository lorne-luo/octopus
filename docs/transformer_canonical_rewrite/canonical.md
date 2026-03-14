# Canonical Package Design

> Package location: `internal/transformer2/canonical/`

### `canonical/request.go`

```go
type RequestKind int
const (
    KindChat RequestKind = iota
    KindEmbedding
    KindCountTokens
)

type Request struct {
    Kind    RequestKind
    Model   string

    // Chat
    Messages   []Message
    Tools      []Tool
    ToolChoice *ToolChoice

    // Generation params
    Temperature         *float64
    TopP                *float64
    TopK                *int64
    MaxTokens           *int64       // Anthropic max_tokens, OpenAI legacy max_tokens
    MaxCompletionTokens *int64       // OpenAI max_completion_tokens
    Stop                *StopSequences
    FrequencyPenalty    *float64
    PresencePenalty     *float64
    Seed                *int64
    Logprobs            *bool
    TopLogprobs         *int64
    Store               *bool
    N                   *int         // OpenAI number of completions

    // Modalities & Audio (OpenAI Chat)
    Modalities  []string      // ["text"], ["text","audio"]
    AudioConfig *AudioConfig  // audio output format + voice

    // Streaming
    Stream        bool
    StreamOptions *StreamOptions    // OpenAI stream_options.include_usage

    // Reasoning/Thinking (unified across providers)
    Reasoning *ReasoningConfig

    // Response format
    ResponseFormat *ResponseFormat  // json_schema, json_object, text
    // Anthropic output_config (output.format)
    OutputConfig   *OutputConfig

    // Embedding specific
    EmbeddingInput          *EmbeddingInput
    EmbeddingDimensions     *int64
    EmbeddingEncodingFormat *string

    // Gemini: media_resolution (per-request level)
    MediaResolution *string  // "low","medium","high","ultra_high"

    // OpenAI-specific passthrough fields
    LogitBias        map[string]int64  // token bias map
    ServiceTier      *string           // "auto", "default", etc.
    PromptCacheKey   *bool             // OpenAI prompt cache optimization
    SafetyIdentifier *string           // OpenAI safety identifier
    User             *string           // end-user identifier

    // Qwen (Alibaba) specific
    EnableThinking *bool  // Qwen models thinking/reasoning toggle

    // Provider extension escape hatch
    ExtraBody json.RawMessage  // opaque JSON merged into outbound request body

    // Passthrough metadata
    SourceFormat APIFormat
    Metadata     map[string]string
    Query        url.Values

    // Provider-specific HTTP headers extracted from the inbound request.
    // Populated by ClientAdapter.ParseRequest (which receives http.Header);
    // consumed by ProviderAdapter.BuildRequest for the outbound request.
    //
    // Key use case: Anthropic's `anthropic-beta` and `anthropic-version` headers
    // which gate server tools, extended thinking, prompt caching, etc.
    // BuildRequest may also auto-generate additional header values based on
    // request content (e.g. detecting server tools → appending beta flags).
    Headers http.Header

    // Raw inbound request body, preserved for logging/debugging.
    // Not used in transformation logic.
    RawRequest []byte

    // TransformHints stores adapter-internal hints for round-trip fidelity.
    // These fields are NOT part of the canonical semantic model — they exist
    // solely to preserve inbound request shape so that the outbound adapter
    // can reconstruct provider-specific wire format accurately.
    Hints TransformHints
}

// TransformHints carries adapter-internal metadata for round-trip fidelity.
// These are set by ClientAdapter.ParseRequest and consumed by ProviderAdapter.BuildRequest.
type TransformHints struct {
    // ResponsesArrayInput: true when OpenAI Responses API input was array format (not string).
    // Used by provider_responses.go to reconstruct the correct input shape.
    ResponsesArrayInput *bool

    // Include: additional output data requested by client.
    // e.g., "file_search_call.results", "message.input_image.image_url", "reasoning.encrypted_content"
    // Preserved from OpenAI Responses API `include` field.
    Include []string

    // AnthropicSystemArrayFormat: true when Anthropic system prompt was array of
    // [{type:"text", text, cache_control}] blocks (not a plain string).
    AnthropicSystemArrayFormat bool

    // GeminiTopK: preserved from Gemini request for round-trip.
    GeminiTopK *int

    // GeminiSafetySettings: preserved as raw JSON from Gemini request.
    GeminiSafetySettings json.RawMessage
}

// AudioConfig for OpenAI Chat audio output
type AudioConfig struct {
    Format string  // "wav","mp3","flac","opus","pcm16"
    Voice  string  // "alloy","ash","coral", etc.
}

// OutputConfig for Anthropic output configuration.
// Maps to Anthropic's two separate top-level fields:
//   output: {format: {type:"json", schema:{...}}}    → Format
//   output_config: {effort: "low"|"medium"|"high"}    → Effort (see ReasoningConfig.Effort)
// Note: output_config.effort is mapped to ReasoningConfig.Effort (not stored here)
// because it's semantically a thinking/reasoning control, not an output format control.
// This struct only carries the structured output format.
type OutputConfig struct {
    Format *OutputFormat  // {type:"json", schema:...}
}

type OutputFormat struct {
    Type   string          // "json"
    Schema json.RawMessage // JSON Schema
}

// ReasoningConfig unifies thinking/reasoning control across providers.
//
// The proxy normalizes three orthogonal concerns:
//   1. Mode (Enabled): whether thinking is on/off/adaptive
//   2. Effort: how hard the model should think
//   3. BudgetTokens: explicit token budget (legacy Anthropic, Gemini)
//
// Provider mapping:
//   OpenAI Chat: reasoning_effort ("none"|"minimal"|"low"|"medium"|"high"|"xhigh")
//     → Effort only; Enabled/BudgetTokens ignored
//
//   Anthropic (Opus 4.6, Sonnet 4.6 — adaptive mode, recommended):
//     thinking.type = "adaptive"  → Enabled = nil
//     output_config.effort        → Effort ("low"|"medium"|"high"|"max")
//     Note: "max" is Opus 4.6 only; other models return error.
//     Note: budget_tokens is DEPRECATED on 4.6 models. Use adaptive + effort instead.
//
//   Anthropic (older models — legacy manual mode):
//     thinking.type = "enabled"   → Enabled = ptr(true)
//     thinking.budget_tokens      → BudgetTokens
//     When only Effort is set (no BudgetTokens), adapter derives budget via lookup table.
//
//   Anthropic (disabled):
//     thinking.type = "disabled"  → Enabled = ptr(false)
//
//   Gemini: thinkingConfig.thinkingLevel → Effort ("minimal"|"low"|"medium"|"high")
//           thinkingConfig.thinkingBudget → BudgetTokens
type ReasoningConfig struct {
    // Effort level — unified across providers.
    //   OpenAI:    "none","minimal","low","medium","high","xhigh"
    //   Anthropic: "low","medium","high","max" (via output_config.effort)
    //   Gemini:    "minimal","low","medium","high"
    // Adapters translate between provider-specific value sets.
    Effort       *string

    // BudgetTokens — explicit token budget for thinking.
    //   Anthropic: thinking.budget_tokens (legacy manual mode only, deprecated on 4.6)
    //   Gemini:    thinkingConfig.thinkingBudget
    //   OpenAI:    ignored (no budget concept)
    BudgetTokens *int64

    // Enabled — thinking mode toggle.
    //   ptr(true):  Anthropic thinking.type="enabled" (legacy manual mode)
    //   ptr(false): Anthropic thinking.type="disabled"
    //   nil:        Anthropic thinking.type="adaptive" (recommended for 4.6+)
    //   OpenAI/Gemini: not applicable (derived from Effort presence)
    Enabled      *bool
}

type StreamOptions struct {
    IncludeUsage bool    // OpenAI stream_options.include_usage
}
```

### `canonical/response.go`

```go
type Response struct {
    ID      string
    Model   string
    Created int64
    Object  string         // "chat.completion", "list", etc.

    Choices    []Choice          // chat
    Embeddings []EmbeddingObject // embedding
    Usage      *Usage
    Error      *Error

    // HTTP status code from provider response (0 if not applicable)
    StatusCode int

    // System fingerprint (OpenAI)
    SystemFingerprint string
    ServiceTier       string
}

// Error is a provider-agnostic error representation.
// Populated when the provider returns an error response (4xx/5xx)
// or when the proxy itself encounters an error.
// ClientAdapter.FormatError converts this into the client's expected format.
type Error struct {
    Code       string // machine-readable: "invalid_request_error", "rate_limit_error", etc.
    Message    string // human-readable description
    Type       string // error category: "invalid_request_error", "authentication_error", etc.
    StatusCode int    // HTTP status to return to the client (400, 401, 429, 500, etc.)
}

// Chunk represents one SSE streaming event from any provider,
// normalized to a common shape.
type Chunk struct {
    ID      string
    Model   string
    Created int64
    Deltas  []ChoiceDelta
    Usage   *Usage         // final chunk may carry usage (OpenAI include_usage)

    // Done signals stream termination. Set to true by provider adapter when:
    //   - OpenAI: receives `data: [DONE]`
    //   - Anthropic: receives `event: message_stop`
    //   - Gemini: connection closes (no sentinel)
    // Replaces the old `InternalLLMResponse.Object == "[DONE]"` pattern.
    Done    bool
}

type Choice struct {
    Index        int
    Message      Message
    FinishReason *string    // canonical values: "stop","length","tool_calls","content_filter"
    Logprobs     *LogprobsContent

    // StopSequence holds the matched custom stop sequence text when
    // FinishReason == "stop" and the stop was triggered by a user-supplied
    // stop sequence (not natural end-of-turn).
    //   Anthropic: response.stop_sequence / message_delta.delta.stop_sequence
    //   OpenAI/Gemini: no equivalent, always nil
    StopSequence *string
}

type ChoiceDelta struct {
    Index        int
    Delta        Message
    FinishReason *string
}
```

### `canonical/message.go`

```go
type Role string
const (
    RoleSystem    Role = "system"
    RoleDeveloper Role = "developer"  // OpenAI developer role
    RoleUser      Role = "user"
    RoleAssistant Role = "assistant"
    RoleTool      Role = "tool"
)

type ContentType string
const (
    ContentText     ContentType = "text"
    ContentImage    ContentType = "image"
    ContentAudio    ContentType = "audio"
    ContentFile     ContentType = "file"
    ContentDocument ContentType = "document"  // Anthropic document block (PDF)
    ContentThinking ContentType = "thinking"  // Anthropic thinking block (interleaved thinking)
)

// Proxy Degradation Policy:
// Provider adapters (e.g. OpenAI parsing canonical Request) must fast-fail 
// with an HTTP 400 error if they encounter an unsupported ContentType 
// (e.g. ContentDocument for OpenAI) to prevent silent drops or black-box errors.

type Message struct {
    Role    Role
    Content []ContentBlock
    Name    *string
    Refusal string

    // Tool call (assistant requesting tool execution)
    ToolCalls  []ToolCall
    // Tool result (tool responding)
    ToolCallID *string

    // Tool call helper fields (not serialized, used by adapters)
    MessageIndex *int
    ToolCallName *string
    ToolCallIsError *bool

    // Reasoning/Thinking output
    Reasoning          *string
    ReasoningSignature *string    // Gemini thoughtSignature, Anthropic signature
    ThoughtSummary     *string    // Gemini thought_summary
    RedactedThinking   *string    // Anthropic redacted_thinking.data (opaque passthrough)

    // Provider-specific passthrough
    CacheControl *CacheControl

    // Server tool content blocks (Anthropic: web_search_tool_result,
    // web_fetch_tool_result, code_execution_tool_result, etc.)
    // These are opaque JSON passthrough — proxy preserves them as-is
    ServerToolBlocks []json.RawMessage

    // Image generation results (merged into Content during processing)
    Images []ContentBlock
}

type ContentBlock struct {
    Type  ContentType
    Text  string          // when Type == ContentText
    Media *MediaContent   // when Type == ContentImage/Audio/File/Document

    // Thinking content (Anthropic extended thinking, OpenAI reasoning)
    // Present when Type == ContentThinking
    Thinking  string  // the thinking/reasoning text
    Signature string  // Anthropic thinking block signature (required for multi-turn)

    // Provider-specific passthrough
    CacheControl *CacheControl
}
```

### `canonical/tool.go`

```go
// Tool unifies:
//   OpenAI Chat: {"type":"function","function":{"name","description","parameters","strict"}}
//   Anthropic: {"name","description","input_schema","cache_control"}
//   Gemini: {"functionDeclarations":[{"name","description","parameters"}]}
type Tool struct {
    Type            string           // "function", "image_generation"
    Name            string
    Description     string
    Parameters      json.RawMessage  // JSON Schema
    Strict          *bool            // OpenAI & Anthropic strict mode (schema validation)
    ImageGeneration *ImageGenerationConfig
    CacheControl    *CacheControl

    // Server/built-in tools (passthrough as opaque JSON)
    // For Anthropic server tools (web_search, code_execution, etc.)
    // and OpenAI built-in tools (web_search_preview, file_search, etc.)
    // When set, the tool is forwarded as-is without canonical conversion
    RawJSON json.RawMessage
}

// ToolCall unifies:
//   OpenAI Chat: choice.message.tool_calls[].{id, type, function.{name, arguments}}
//   Anthropic: content[].{type:"tool_use", id, name, input(json)}
//   Gemini: parts[].functionCall.{name, args(map)}
type ToolCall struct {
    ID           string
    Type         string    // "function"
    Name         string
    Arguments    string    // JSON string (Gemini: marshal map[string]any to JSON string)
    Index        int
    CacheControl *CacheControl

    // Gemini thoughtSignature for function call round-trip.
    // Gemini 3 requires thoughtSignature on functionCall parts; for parallel
    // function calls only the first part carries it. Stored here so the
    // proxy can accurately reconstruct the signature→functionCall mapping
    // when converting back to Gemini wire format.
    // Non-Gemini adapters ignore this field.
    Signature    *string
}

// ImageGenerationConfig carries parameters for image_generation tool type.
// Used by OpenAI Responses API for in-line image generation.
type ImageGenerationConfig struct {
    Background        string  // "opaque", "transparent"
    OutputFormat      string  // "png", "jpeg", "webp"
    Quality           string  // "standard", "hd"
    Size              string  // "1024x1024", "1792x1024", etc.
    OutputCompression *int64  // compression level
}

// ToolChoice unifies:
//   OpenAI: "none"|"auto"|"required"|{"type":"function","function":{"name":"..."}}
//          + top-level parallel_tool_calls (mapped here as DisableParallelToolUse)
//   Anthropic: {"type":"auto"|"none"|"tool"|"any", "name":"...",
//              "disable_parallel_tool_use":true} -> "required" maps to "any"
//   Gemini: toolConfig.functionCallingConfig.mode ("AUTO"|"ANY"|"NONE") -> "required" maps to "ANY"
type ToolChoice struct {
    Mode     string   // "auto", "none", "required", "tool"
    Function *string  // specific function name (when Mode == "tool")

    // Unified parallel tool call control.
    //   OpenAI:    parallel_tool_calls: false  → DisableParallelToolUse = ptr(true)
    //   Anthropic: tool_choice.disable_parallel_tool_use: true → same
    //   Gemini:    no equivalent, ignored
    DisableParallelToolUse *bool
}
```

### `canonical/media.go`

```go
// MediaContent unifies multimodal content across providers:
//   OpenAI Chat: image_url{url,detail}, input_audio{data,format}, file{file_data,file_id,filename}
//   Anthropic: image{source:{type,media_type,data,url}}, document{source:{type,media_type,data}}
//   Gemini: inlineData{mimeType,data}, fileData{mimeType,fileUri}
type MediaContent struct {
    // Image
    URL      string   // URL-based image (OpenAI image_url.url, Anthropic url source, Gemini fileUri)
    Base64   string   // base64-encoded image data
    MimeType string   // MIME type (image/jpeg, image/png, etc.)
    Detail   *string  // OpenAI detail level: "auto"|"low"|"high"

    // Audio
    AudioFormat string  // "wav"|"mp3"
    AudioData   string  // base64-encoded audio

    // File / Document
    FileName string
    FileData string   // base64-encoded file content
    FileID   string   // provider-specific file ID reference
    FileURL  string   // URL to file
}
```

### `canonical/usage.go`

```go
// Usage unifies token counts from all providers:
//   OpenAI Chat: prompt_tokens, completion_tokens, total_tokens,
//                prompt_tokens_details.{cached_tokens, audio_tokens},
//                completion_tokens_details.{reasoning_tokens, audio_tokens, ...}
//   Anthropic: input_tokens, output_tokens,
//              cache_creation_input_tokens, cache_read_input_tokens
//   Gemini: usageMetadata.{promptTokenCount, candidatesTokenCount, totalTokenCount,
//           thoughtsTokenCount, cachedContentTokenCount}
//
// IMPORTANT for Anthropic caching:
// When cache_control is used, Anthropic's input_tokens only counts tokens
// AFTER the last cache breakpoint. The true total prompt tokens is:
//   PromptTokens = cache_read_input_tokens + cache_creation_input_tokens + input_tokens
// The InputTokensAfterBreakpoint field stores the original input_tokens value.
//
// See: https://platform.claude.com/docs/en/docs/build-with-claude/prompt-caching#tracking-cache-performance
type Usage struct {
    PromptTokens     int64  // Total prompt tokens (calculated correctly for caching)
    CompletionTokens int64
    TotalTokens      int64

    PromptTokensDetails     *PromptTokensDetails
    CompletionTokensDetails *CompletionTokensDetails

    // Anthropic cache-specific (totals)
    CacheCreationInputTokens int64
    CacheReadInputTokens     int64

    // Anthropic cache creation breakdown by TTL.
    // Present when mixing 5m and 1h cache TTLs in the same request.
    // See: https://docs.anthropic.com/en/docs/build-with-claude/prompt-caching#1-hour-cache-duration
    //
    // API response example:
    //   "cache_creation": {
    //     "ephemeral_5m_input_tokens": 456,
    //     "ephemeral_1h_input_tokens": 100
    //   }
    // Note: CacheCreationInputTokens == Ephemeral5mInputTokens + Ephemeral1hInputTokens
    //
    // This matters for accurate cost calculation because 5m writes are 1.25x
    // and 1h writes are 2x base input token price.
    CacheCreation *CacheCreationDetails

    // Anthropic-specific: original input_tokens from API response.
    // When caching is enabled, this only represents tokens AFTER the last
    // cache breakpoint, NOT total input tokens. Use PromptTokens for the correct total.
    // Preserved for accurate logging and debugging of cache efficiency.
    InputTokensAfterBreakpoint int64

    // Anthropic server tool usage
    ServerToolUsage *ServerToolUsage

    // Internal flag for adapter-specific behavior
    IsAnthropicUsage bool
}

// CacheCreationDetails breaks down cache creation tokens by TTL duration.
// Only populated when the Anthropic response includes the nested cache_creation object
// (i.e., when mixing 5m and 1h cache TTLs).
type CacheCreationDetails struct {
    Ephemeral5mInputTokens int64  // tokens written to 5-minute cache
    Ephemeral1hInputTokens int64  // tokens written to 1-hour cache
}

// ServerToolUsage tracks Anthropic server-side tool invocations
type ServerToolUsage struct {
    WebSearchRequests int64
    WebFetchRequests  int64
}

type PromptTokensDetails struct {
    CachedTokens int64  // OpenAI cached_tokens, Gemini cachedContentTokenCount
    AudioTokens  int64  // OpenAI audio_tokens
}

type CompletionTokensDetails struct {
    ReasoningTokens          int64  // OpenAI reasoning_tokens, Gemini thoughtsTokenCount
    AudioTokens              int64
    AcceptedPredictionTokens int64
    RejectedPredictionTokens int64
}
```

