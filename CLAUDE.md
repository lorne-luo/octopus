# Claude Code Guide

## Project Overview
Octopus is an "all API in one place" application, likely a unified API gateway or management platform for LLMs.
It is built with Go (Backend) and React/Next.js (Frontend).

## Tech Stack
- **Backend**: Go, Gin, GORM
- **Frontend**: React, Next.js, Tailwind CSS
- **Database**: SQLite, MySQL, PostgreSQL, remember OAuthProvider model's table name is o_auth_providers, Channel.OAuthProvider column name is o_auth_provider_id
- **Container**: Docker

## Development Commands

### Backend
- Run: `go run main.go start`
- Build: `go build -o octopus main.go`
- Test: `go test ./...`

### Frontend
- Directory: `web/`
- Install: `cd web && pnpm install`
- Dev: `cd web && pnpm dev`
- Build: `cd web && pnpm build`

### Database
- Auto-migration is enabled on startup.

## Core Components
- **Relay**: Handles API request routing and transformation (`internal/relay`).
- **Transformer**: Converts requests between different provider formats (`internal/transformer`).
- **Model**: Database models (`internal/model`).
- **Task**: Background tasks (`internal/task`).

## Request Flow
1.  **Server**: Receives request (`internal/server`).
2.  **Relay**: Routes request (`internal/relay`).
3.  **Balancer**: Selects channel/key (`internal/relay/balancer`).
4.  **Transformer**: Transforms request to provider format (`internal/transformer`).
5.  **Outbound**: Sends request to provider.
6.  **Inbound**: Transforms response back to client format.

## Key Database Models
- `User`
- `Channel`
- `ChannelKey`
- `Group`
- `RelayLog`
- `Stats*`

## Adding a New Provider
1.  Add provider constant in `internal/conf/const.go` (if needed).
2.  Implement transformer in `internal/transformer`.
3.  Update relay logic to handle the new provider type.

## Test Data
SQLite DB at ./data/data.db will be used for development, use command `sqlite3 data/data.db` to check the data.