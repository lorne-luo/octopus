# Health Monitor Integration Plan

## Overview
Move channel health monitoring from a top-level menu to integrated panels in channel create/edit forms, displaying health status and latency on channel list cards.

## Backend Changes (Minimal)

### 1. Add Latency Tracking to Health Model
**File**: `internal/model/health.go`
- Add `LatencyMs *int` field to `HealthCheck` struct
- Update database schema to include `latency_ms` column

### 2. Update Health Check Task to Record Latency
**File**: `internal/task/health.go`
- Modify `RunHealthCheck` function to measure and record latency
- Update `HealthUpdateStatus` call to include latency

### 3. Update Health API Response
**File**: `internal/op/health.go`
- Ensure `HealthCheckWithChannel` includes latency field
- No API endpoint changes needed

## Frontend Changes

### 4. Add Health Monitoring to Channel Form
**File**: `web/src/components/modules/channel/Form.tsx`

Add health monitoring accordion section:
- Add state for health check settings (model, interval, prompt)
- Add health check fetch/query to get existing settings
- Add accordion panel after advanced settings
- Fields:
  - Model selection dropdown (fetch from channel's available models)
  - Interval input (minutes, with suggestions like current implementation)
  - Prompt textarea (default to simple health check prompt)
- Add create/update health check API calls on form submit

### 5. Update Channel List Card
**File**: `web/src/components/modules/channel/Card.tsx`

Add health status display:
- Fetch health check data for the channel
- Add health status indicator (colored dot/badge)
- Add latency display in ms
- Position in existing stats section or add new row
- Show "No monitoring" when no health check configured

### 6. Create Health Monitoring Hook
**File**: `web/src/api/endpoints/health.ts` (extend existing)

Add React Query hook:
- `useHealthByChannelId(channelId)` - fetch health check for specific channel

## Cleanup

### 7. Remove Top-level Health Menu
**File**: `web/src/route/config.tsx`
- Remove health route from routes array

### 8. Remove Health Pages
- Delete `web/src/components/modules/health/index.tsx`
- Delete `web/src/components/modules/health/Card.tsx`
- Delete `web/src/components/modules/health/Create.tsx`

### 9. Remove Health Route Registration
**File**: `web/src/route/index.tsx`
- Remove health route import and registration

## UI/UX Details

### Channel Form Health Panel
- Use existing accordion pattern from advanced settings
- Position at bottom of form, above submit buttons
- Use same styling: `border rounded-xl bg-card`
- Fields:
  - Model select (required if health monitoring enabled)
  - Interval input with suggestions (like current 1h, 4h, 8h, 24h, 1week)
  - Prompt textarea (optional, defaults to "Say 'Hello'" or similar)
- Add enable/disable toggle for health monitoring

### Channel Card Health Display
- Add to existing stats section in `Card.tsx`
- Use `Badge` component for status (healthy/unhealthy/checking/unknown)
- Show latency in ms with appropriate formatting
- Use color coding: green (healthy), red (unhealthy), yellow (checking), gray (unknown/no monitoring)
- Keep consistent with existing card design patterns

## Testing Verification

1. Create new channel with health monitoring enabled
   - Verify health check created in database
   - Verify task scheduled
2. Edit existing channel to add health monitoring
   - Verify health check created
3. Edit existing channel to modify health monitoring
   - Verify health check updated
4. View channel list
   - Verify health status displayed on cards
   - Verify latency shows when available
5. Verify health check execution
   - Check that latency is recorded
   - Check that status updates correctly
6. Verify removal of top-level menu
   - Health menu item no longer in navigation
   - Health pages not accessible

## Dependencies

- All changes are internal to the codebase
- No external dependencies needed
- Reuses existing UI components (Accordion, Badge, Switch, etc.)
- Reuses existing API patterns

## Estimated Scope

- Backend: 3 files modified
- Frontend API: 1 file extended
- Frontend Components: 2 files modified, 3 files deleted
- Total: ~9 files affected