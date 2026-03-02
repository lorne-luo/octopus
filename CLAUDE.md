# Claude Code Guide

## Project Overview
Octopus is an "all API in one place" application, likely a unified API gateway or management platform for LLMs.
It is built with Go (Backend) and React/Next.js (Frontend).

## Tech Stack
- **Backend**: Go, Gin, GORM
- **Frontend**: React, Next.js, Tailwind CSS
- **Database**: SQLite, MySQL, PostgreSQL
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


### Debug with real channel
I will add some real channel in data/data.db
You can get schema by 
```
sqlite3 data/data.db ".schema channels"
```
and get data from tables
sqlite3 data/data.db "SELECT * FROM channels where name='Xunfei';"
And design curl to verify the channel works, if `localhost:9100` is not reachable you can ask me to start the server
```
curl -N http://localhost:9100/v1/messages \
  --header "x-api-key: sk-octopus-Qx8dHErG0OtVXKI1za3TNhEi9aPo79xpEgu3PqAAk0R2diHu" \
  --header "anthropic-version: 2023-06-01" \
  --header "content-type: application/json" \
  --data '{
    "model": "xunf",
    "max_tokens": 1024,
    "stream": true,
    "messages": [
      {"role": "user", "content": "Hello, generate 30 tokens sentence"}
    ]
  }'
```