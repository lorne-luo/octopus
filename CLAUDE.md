# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**Octopus** is a full-stack LLM API aggregation and load balancing service. It acts as a middleware proxy that:
- Aggregates multiple LLM provider channels (OpenAI, Anthropic, Gemini, etc.)
- Provides load balancing across channels
- Converts between different API protocols
- Offers a web-based management UI

**Tech Stack:**
- Backend: Go 1.24.4 (Gin web framework)
- Frontend: Next.js 16.0.7 + React 19.2.1 + TypeScript + Tailwind CSS + Zustand
- Database: SQLite/MySQL/PostgreSQL (via GORM)

## Development Commands

```bash
# Frontend development (runs on port 3000)
cd web && pnpm install && NEXT_PUBLIC_API_BASE_URL="http://127.0.0.1:8080" pnpm run dev

# Backend development (runs on port 8080)
go run main.go start

# Quick production build
cd web && pnpm install && pnpm run build && cd ..
mv web/out static/

# Full release build (all platforms)
./scripts/build.sh release

# Build for specific platform
./scripts/build.sh build linux x86_64
./scripts/build.sh build darwin arm64
./scripts/build.sh build windows x86_64
```

## Architecture Overview

### Request Flow

The system uses a dual-layer transformer pattern for protocol conversion:

**Non-Streaming:**
```
Client -> inbound.TransformRequest() -> outbound.TransformRequest() -> HTTP Request to Provider
       <- inbound.TransformResponse() <- outbound.TransformResponse() <- Provider Response
```

**Streaming (SSE):**
```
Client -> inbound.TransformRequest() -> outbound.TransformRequest() -> HTTP Stream
       <- inbound.TransformStream() <- outbound.TransformStream() <- SSE Chunks
```

### Core Components

| Component | Location | Purpose |
|-----------|----------|---------|
| `cmd/` | Root | CLI commands (Cobra-based) |
| `internal/conf/` | Config | Viper-based configuration loader |
| `internal/db/` | DB | GORM initialization and migrations |
| `internal/model/` | Models | Database models + business models |
| `internal/op/` | Operations | CRUD business logic layer |
| `internal/relay/relay.go` | Relay | Core request routing/proxy logic |
| `internal/relay/balancer/` | Balancer | Load balancer implementations (Round Robin, Random, Failover, Weighted) |
| `internal/transformer/` | Transformer | Protocol conversion layer |
| `internal/server/` | Server | Gin HTTP server and router |
| `web/src/` | Frontend | Next.js SPA with Zustand state |

### Transformer Layer

The transformer layer allows the system to speak multiple API protocols:

- **Inbound** (`internal/transformer/inbound/`): Client format -> Internal format
- **Outbound** (`internal/transformer/outbound/`): Internal format -> Provider format

**Supported Inbound Formats:** OpenAI Chat, OpenAI Responses, Anthropic Messages, Gemini
**Supported Outbound Formats:** OpenAI Chat, OpenAI Responses, Anthropic Messages, Gemini, Volcengine

Add new protocols by implementing the `transformer/model.Inbound` and `transformer/model.Outbound` interfaces and registering them in the respective `transformer/*/registry.go` files.

### Key Database Models

Located in `internal/model/`:
- `User` - Admin accounts
- `Channel` - LLM provider connections with auto-discovered endpoints
- `ChannelKey` - Multiple API keys per channel
- `Group` - Model groups that aggregate multiple channels
- `GroupItem` - Channel-to-group mapping with weights
- `LLMInfo` - Cached model lists from channels
- `APIKey` - Client API keys for accessing the service
- `Setting` - Runtime settings (statistics interval, etc.)
- `Stats*` - Request statistics (token count, cost tracking)

### Router System

The router uses a self-registering pattern. Handlers register routes via `init()` functions in `internal/server/router/router.go`. Add new routes by creating a package with an `init()` function that calls `router.RegisterRoutes()`.

### Background Tasks

Located in `internal/task/task.go`:
- Price syncing from [models.dev](https://github.com/sst/models.dev)
- Model syncing from upstream channels
- Statistics persistence (memory-first, batch-write at configured interval)

**Important:** Statistics are stored in memory and batch-written to avoid heavy DB writes. Properly shutdown with `Ctrl+C` or `SIGTERM` to avoid data loss. Never use `kill -9`.

## Important Notes

- Frontend build artifacts are embedded into the Go binary via `internal/server/static/static.go` using `go:embed`
- Always build the frontend before running `go run main.go start`
- Channels support multiple endpoints - the system automatically pings them and selects the one with lowest latency
- Groups use different load balancing modes: Round Robin, Random, Failover (priority-based), Weighted
- Configuration is auto-generated at `data/config.json` on first run
- Default credentials: `admin` / `admin` (change immediately)

## Adding Support for New Providers

1. Add the outbound transformer in `internal/transformer/outbound/`
2. Register it in `internal/transformer/outbound/registry.go`
3. Update `internal/model/channel.go` const with the new channel type
4. Add base URL auto-append logic in the channel creation handler if needed