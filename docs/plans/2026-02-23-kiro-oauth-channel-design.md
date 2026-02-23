# Kiro OAuth Channel Design

**Date:** 2026-02-23
**Status:** Draft
**Reference:** `/code/lorne/kiro-go-proxy`

## Overview

Add Kiro OAuth channel support to octopus, following the existing iFlow pattern but with extended capabilities for request/response transformation.

## Architecture

### Components

| Component | Purpose |
|-----------|---------|
| `internal/oauth/kiro/auth.go` | Token refresh logic (Kiro Desktop Auth) |
| `internal/oauth/kiro/token.go` | Token structures |
| `internal/oauth/kiro/parser.go` | AWS Event Stream parsing |
| `internal/transformer/kiro/*.go` | Request/response transformation |
| `internal/model/oauth_provider.go` | Update GetBaseURL() for Kiro |
| `internal/oauth/manager.go` | Add Kiro refresh case |
| `internal/relay/*.go` | Kiro-specific relay handling |

### Key Differences from iFlow

- Kiro uses refresh token → access token flow (not API key rotation)
- Requires transformer for request format conversion
- Response is AWS Event Stream binary, needs parsing

### Default Base URL

`https://q.us-east-1.amazonaws.com`

## Authentication Flow

### Kiro Desktop Auth

```
Refresh Token (stored in AuthJson)
    |
    v
POST https://prod.{region}.auth.desktop.kiro.dev/refreshToken
    {
      "refreshToken": "...",
      "clientId": "..."
    }
    |
    v
Response:
    {
      "accessToken": "...",
      "expiresIn": 3600,
      "tokenType": "Bearer"
    }
```

### AuthJson Content Format

```json
{
  "RefreshToken": "xxx...",
  "Region": "us-east-1"
}
```

- `RefreshToken` - Required, the refresh token from Kiro Desktop
- `Region` - Optional, defaults to `us-east-1`

### Token Storage

- `APIKey` → stores the access token
- `APIKeyExpireAt` → stores expiration timestamp
- Auto-refresh when within 10 minutes of expiration

### Manager Refresh Logic

1. Get active AuthJson (RefreshToken)
2. Call Kiro Desktop Auth endpoint
3. Update provider with new access token
4. Mark AuthJson status code (200/429/etc.)

## Request Transformer (OpenAI → Kiro)

### Kiro Payload Structure

```json
{
  "conversationState": {
    "chatTriggerType": "MANUAL",
    "conversationId": "uuid",
    "currentMessage": {
      "userInputMessage": {
        "content": "...",
        "modelId": "...",
        "origin": "AI_EDITOR",
        "images": [],
        "userInputMessageContext": {
          "tools": [],
          "toolResults": []
        }
      }
    },
    "history": []
  },
  "profileArn": "..."
}
```

### Field Mapping

| OpenAI Field | Kiro Field | Notes |
|--------------|------------|-------|
| `messages` | `conversationState.history` + `currentMessage` | Last message → current, rest → history |
| `model` | `currentMessage.userInputMessage.modelId` | Model name mapping |
| `tools` | `userInputMessageContext.tools` | Tool definitions |
| `stream` | N/A | Handled at response level |

### Message Processing

1. `StripAllToolContent` - Remove tool content when no tools
2. `MergeAdjacentMessages` - Merge same-role messages
3. `EnsureFirstMessageIsUser` - Prepend synthetic user if needed
4. `EnsureAlternatingRoles` - Insert empty assistant messages

### Model Mapping

- Support aliases (e.g., `claude-sonnet-4-5` → `claude-sonnet-4.5`)
- Pass through unknown models to Kiro API

## Response Transformer (Kiro → OpenAI)

### Kiro Response Format

AWS Event Stream (binary SSE), each event is a JSON object:

```json
{"type": "content", "content": "Hello"}
{"type": "tool_start", "name": "func", "toolUseId": "xxx"}
{"type": "tool_input", "input": "{...}"}
{"type": "tool_stop"}
{"type": "usage", "inputTokens": 100, "outputTokens": 50}
```

### Event Types

| Kiro Event | OpenAI Equivalent | Notes |
|------------|-------------------|-------|
| `content` | `delta.content` | Text content |
| `thinking` | `delta.reasoning_content` | Extended thinking |
| `tool_start` | `delta.tool_calls[].function.name` | Tool call begins |
| `tool_input` | `delta.tool_calls[].function.arguments` | Tool arguments |
| `tool_stop` | Close tool call | End of tool |
| `usage` | `usage` field | Token counts |

### Parser Logic

- Buffer incomplete JSON across chunks
- `FindMatchingBrace()` to locate complete JSON objects
- Handle bracket-format tool calls: `[Called func_name with args: {...}]`

### Streaming Response

- Convert to OpenAI SSE format
- Send `data: {...}\n\n` for each event
- Final `data: [DONE]`

### Non-Streaming Response

- Buffer all events
- Aggregate content into single response

## Relay Integration

### Flow

```
Client Request (OpenAI format)
    |
    v
Relay detects UseOAuth=true + ProviderType=Kiro
    |
    v
OAuth Manager: Get/Refresh Access Token
    |
    v
Kiro Transformer: Convert request to Kiro format
    |
    v
POST {baseURL}/generateAssistantResponse
    Headers: Authorization: Bearer {accessToken}
    |
    v
Kiro Parser: Parse AWS Event Stream
    |
    v
Kiro Transformer: Convert response to OpenAI format
    |
    v
Client Response
```

### Channel Configuration

- `UseOAuth: true`
- `OAuthProviderID: <kiro_provider_id>`
- `BaseURL: https://q.us-east-1.amazonaws.com` (or custom)

### Error Detection

```go
func isKiroError(body []byte) bool {
    bodyStr := string(body)
    return strings.Contains(bodyStr, "UnauthorizedException") ||
           strings.Contains(bodyStr, "AccessDenied")
}
```

### Token Refresh on Failure

- On 401, auto-refresh token and retry once
- On 429, mark AuthJson as rate-limited, try next

## File Structure

### New Files

```
internal/oauth/kiro/
├── auth.go          # Token refresh logic
├── token.go         # Token structures
├── parser.go        # AWS Event Stream parser
└── auth_test.go     # Unit tests

internal/transformer/kiro/
├── request.go       # OpenAI → Kiro request transformer
├── response.go      # Kiro → OpenAI response transformer
├── message.go       # Message processing utilities
├── model.go         # Model name mapping
└── transformer_test.go
```

### Files to Modify

| File | Changes |
|------|---------|
| `internal/model/oauth_provider.go` | Update `GetBaseURL()` for Kiro |
| `internal/oauth/manager.go` | Add `refreshKiroToken()` case |
| `internal/server/handlers/oauth_provider.go` | Add Kiro AuthJson validation |
| `internal/relay/relay.go` | Add Kiro transformer routing |
| `internal/relay/passthrough.go` | Add Kiro error detection |

## Implementation Order

1. **OAuth Layer** - `internal/oauth/kiro/auth.go`, `token.go`
2. **Parser** - `internal/oauth/kiro/parser.go`
3. **Transformer** - `internal/transformer/kiro/*.go`
4. **Manager Integration** - Update `manager.go`
5. **Relay Integration** - Update relay files
6. **Handler Updates** - AuthJson validation
7. **Tests** - Unit tests for each component
