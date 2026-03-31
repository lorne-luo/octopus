# TestModel Feature Development Plan

## Context

Add a "Test Model" menu to test channel configurations with predefined message types. This allows users to verify their channel setup works correctly before using it in production.

**Evolving Requirements (all sessions):**
1. **Menu Navigation:** Add `TestModel` to the top-level menu, positioned **before** `Setting`.
2. **Key Capabilities:** Send test messages containing different payloads based on the selected channel's model.
3. **Payloads Required:**
   - `text_chat.json`: Test basic text chat.
   - `vision_chat.json`: Test vision capabilities.
   - `tool_chat.json`: Test tool calling functions.
4. **Channel Handling:**
   - The test payload must be formatted according to the OpenAI request format.
   - If the channel type is Anthropic or Gemini, the request body should be appropriately converted from the OpenAI payload format.
5. **UI Specifications:**
   - The inputs for `base url`, `api key` and `model` must be **Selection Inputs** (combobox / dropdowns) populated based on the selected Channel.
   - If the channel list is not empty, the page should automatically default to selecting the **last active channel** (excluding `enabled=false` channels).
   - In the channel options list, **hide** the disabled channels (`enabled=false`).
   - The `api key` input should be a standard Dropdown (without search). The `model` list must have search capabilities (like a Combobox).
   - The `model` list includes **both** `model` and `custom_model` fields from the Channel, merged and deduplicated.
   - Base UI design for ConfigPanel should borrow and learn from the existing Channel Edit Page (e.g., using `rounded-xl`, `space-y-2`, masked keys).
   - The page layout **must** be fully responsive and display nicely on mobile devices.
6. **Result Display Specifications:**
   - Ensure the raw JSON response is presented directly as the output rather than just extracting text content.
   - The **Request Info** section must also render the request body JSON using `@uiw/react-json-view`.
   - If it's a Server-Sent Events (SSE) streaming response, collect all chunks, merge them, and output a **complete, semantically correct** JSON response object. The merged object must:
     - Have `object: "chat.completion"` (not `chat.completion.chunk`)
     - Include `finish_reason` from the last relevant chunk
     - Include `usage` from the final usage chunk
     - Properly concatenate `tool_calls[n].function.arguments` by `index` (string concatenation, not array push)
     - Include `reasoning_content` if present in deltas
   - Apply the `@uiw/react-json-view` component (the same one used in the Log Details Page) to render the merged JSON, providing formatting, collapsible nodes, and a node-level copy button.
   - While SSE is in progress, display a spinning **Loader2 icon** next to the "响应内容" label to indicate in-processing status.

---

## Architecture Overview

```
Frontend                          Backend
┌─────────────────┐              ┌──────────────────┐
│ TestModel Page  │              │ Relay Endpoint   │
│ ├ Channel Select│──POST───────>│ /v1/chat/        │
│ ├ Test Type Tab │              │ completions      │
│ ├ JSON Config   │              │                  │
│ └ JSON Response │<──SSE/JSON──│                   │
└─────────────────┘              └──────────────────┘
```

**Note:** Instead of creating a custom backend endpoint for testing, the frontend posts directly to the configured Base URL, building the payload in the frontend to respect the provider format.

---

## Files Created / Modified

### Frontend (web/src/)

| File | Purpose |
|------|---------|
| `components/modules/testmodel/index.tsx` | Main TestModel page. Orchestrates layout, SSE fetching, state management, and chunk-level JSON merging. |
| `components/modules/testmodel/ConfigPanel.tsx` | Config side panel: searchable Model combobox (model + custom_model merged), API Key dropdown, disabled-channel-filtered Channel Selector. |
| `components/modules/testmodel/ResultPanel.tsx` | Renders request body and merged response with `@uiw/react-json-view`. Shows Loader2 streaming icon. |
| `components/modules/testmodel/request-builder.ts` | Builds typed requests from OpenAI templates and converts them to Anthropic/Gemini format per ChannelType. |
| `route/config.tsx` & `nav-store.ts` | Added `testmodel` route and nav item, ordered before `setting`. |

### Configuration JSONs (Project Root)

| File | Purpose |
|------|---------|
| `text_chat.json` | Basic text chat test payload. |
| `vision_chat.json` | Vision/multimodal test payload with image_url. |
| `tool_chat.json` | Tool calling test payload with tools array. |

---

## SSE Chunk Merging Strategy (OpenAI)

The merge happens **live** during streaming, building a single result object:

1. **First chunk:** Initialize skeleton from `id`, `created`, `model`. Force `object = "chat.completion"`.
2. **Content delta:** Concatenate `delta.content` into `message.content`.
3. **Reasoning delta:** Concatenate `delta.reasoning_content` into `message.reasoning_content`.
4. **Tool calls delta:** Merge by `tool.index`. Create entry on first appearance; concatenate `function.arguments` string for subsequent entries.
5. **finish_reason:** Pick up from whichever chunk has it set.
6. **usage:** Pick up from the final usage-bearing chunk.

---

## Verification Plan

1. **Responsive Design:** Chrome DevTools device simulation — verify column-stack layout on mobile with no overflow.
2. **Channel Filtering:** Disabled channels hidden from dropdown. Last active channel auto-selected on page load.
3. **Model List:** Both `model` and `custom_model` appear, deduplicated, searchable.
4. **SSE JSON Merging (text):** Send text chat → response shows complete `chat.completion` object with `usage`.
5. **SSE JSON Merging (tool_calls):** Send tool chat → `arguments` is a single valid JSON string, `finish_reason: "tool_calls"` is present, `usage` is populated.
6. **Request Info JSON:** Click to expand Request Info → body renders as `@uiw/react-json-view` tree with copy buttons.
7. **Streaming Indicator:** Loader2 spinning icon visible next to "响应内容" label during streaming; disappears when done.
8. **Payload Formats:** Anthropic channel → request body in Anthropic Messages format. Gemini channel → Gemini `contents` format with `?alt=sse&key=...`.
