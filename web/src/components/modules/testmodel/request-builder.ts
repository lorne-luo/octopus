import { ChannelType } from '@/api/endpoints/channel';

export type TestType = 'text_chat' | 'vision_chat' | 'tool_chat';

// =========================================================
// OpenAI format test templates (source of truth)
// =========================================================

const TEXT_CHAT_TEMPLATE = {
    model: '',
    temperature: 0.7,
    top_p: 1,
    messages: [{ role: 'user', content: 'Hi' }],
    stream: true,
};

const VISION_CHAT_TEMPLATE = {
    model: '',
    temperature: 0.7,
    top_p: 1,
    messages: [
        {
            role: 'user',
            content: [
                { type: 'text', text: 'What color is in this image?' },
                {
                    type: 'image_url',
                    image_url: {
                        // 1x1 red pixel PNG
                        url: 'data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8DwHwAFBQIAX8jx0gAAAABJRU5ErkJggg==',
                    },
                },
            ],
        },
    ],
    stream: true,
};

const TOOL_CHAT_TEMPLATE = {
    model: '',
    temperature: 0.7,
    top_p: 1,
    messages: [{ role: 'user', content: 'What is the weather in San Francisco?' }],
    tools: [
        {
            type: 'function',
            function: {
                name: 'get_weather',
                description: 'Get the weather',
                parameters: {
                    $schema: 'http://json-schema.org/draft-07/schema#',
                    type: 'object',
                    properties: {
                        location: { description: 'City name', type: 'string' },
                    },
                    required: ['location'],
                    additionalProperties: false,
                },
            },
        },
    ],
    tool_choice: 'auto',
    stream: true,
};

// =========================================================
// Request config
// =========================================================

export interface TestConfig {
    channelType: ChannelType;
    baseUrl: string;
    apiKey: string;
    model: string;
    testType: TestType;
}

export interface BuiltRequest {
    url: string;
    headers: Record<string, string>;
    body: string;
}

// =========================================================
// Helpers – format converters
// =========================================================

type OpenAIMessage = {
    role: string;
    content:
        | string
        | Array<{
              type: string;
              text?: string;
              image_url?: { url: string };
          }>;
};

/** Convert OpenAI messages to Anthropic messages format */
function toAnthropicMessages(messages: OpenAIMessage[]) {
    return messages.map((m) => {
        if (typeof m.content === 'string') {
            return { role: m.role, content: m.content };
        }
        // Multi-part content
        const parts = m.content.map((part) => {
            if (part.type === 'text') {
                return { type: 'text', text: part.text ?? '' };
            }
            if (part.type === 'image_url' && part.image_url?.url) {
                const url = part.image_url.url;
                if (url.startsWith('data:')) {
                    const [mime, data] = url.slice(5).split(';base64,');
                    return {
                        type: 'image',
                        source: { type: 'base64', media_type: mime, data },
                    };
                }
                return { type: 'image', source: { type: 'url', url } };
            }
            return part;
        });
        return { role: m.role, content: parts };
    });
}

/** Convert OpenAI tools to Anthropic tools format */
function toAnthropicTools(tools: typeof TOOL_CHAT_TEMPLATE.tools) {
    return tools.map((t) => ({
        name: t.function.name,
        description: t.function.description,
        input_schema: t.function.parameters,
    }));
}

/** Convert OpenAI messages to Gemini contents format */
function toGeminiContents(messages: OpenAIMessage[]) {
    return messages.map((m) => {
        const role = m.role === 'assistant' ? 'model' : 'user';
        if (typeof m.content === 'string') {
            return { role, parts: [{ text: m.content }] };
        }
        const parts = m.content.map((part) => {
            if (part.type === 'text') {
                return { text: part.text ?? '' };
            }
            if (part.type === 'image_url' && part.image_url?.url) {
                const url = part.image_url.url;
                if (url.startsWith('data:')) {
                    const [mime, data] = url.slice(5).split(';base64,');
                    return { inlineData: { mimeType: mime, data } };
                }
                return { fileData: { fileUri: url } };
            }
            return { text: '' };
        });
        return { role, parts };
    });
}

/** Convert OpenAI tools to Gemini function declarations */
function toGeminiFunctionDeclarations(tools: typeof TOOL_CHAT_TEMPLATE.tools) {
    return tools.map((t) => ({
        name: t.function.name,
        description: t.function.description,
        parameters: t.function.parameters,
    }));
}

// =========================================================
// Main builder
// =========================================================

export function buildRequest(config: TestConfig): BuiltRequest {
    const { channelType, baseUrl, apiKey, model, testType } = config;

    // Strip trailing slash and any common API version suffix (e.g. /v1, /v1beta)
    // so that channels stored as "https://api.example.com/v1" don't produce double /v1
    const baseUrl_ = baseUrl
        .replace(/\/$/, '')
        .replace(/\/v\d+(?:beta)?$/, '');


    let template: typeof TEXT_CHAT_TEMPLATE | typeof VISION_CHAT_TEMPLATE | typeof TOOL_CHAT_TEMPLATE;
    if (testType === 'text_chat') template = TEXT_CHAT_TEMPLATE;
    else if (testType === 'vision_chat') template = VISION_CHAT_TEMPLATE;
    else template = TOOL_CHAT_TEMPLATE;

    const templateWithModel = { ...template, model };

    // ── OpenAI-compatible channels ──────────────────────────
    if (
        channelType === ChannelType.OpenAIChat ||
        channelType === ChannelType.OpenAIResponse ||
        channelType === ChannelType.OpenAIEmbedding ||
        channelType === ChannelType.Volcengine
    ) {
        return {
            url: `${baseUrl_}/v1/chat/completions`,
            headers: {
                'Content-Type': 'application/json',
                Authorization: `Bearer ${apiKey}`,
            },
            body: JSON.stringify(templateWithModel, null, 2),
        };
    }

    // ── Anthropic ────────────────────────────────────────────
    if (channelType === ChannelType.Anthropic) {
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        const base = templateWithModel as any;
        // eslint-disable-next-line @typescript-eslint/no-unused-vars
        const { stream: _stream, tool_choice, tools, ...rest } = base;

        const body: Record<string, unknown> = {
            ...rest,
            messages: toAnthropicMessages(base.messages),
            max_tokens: 2048,
            stream: true,
        };

        if (tools) {
            body.tools = toAnthropicTools(tools);
        }
        if (tool_choice) {
            body.tool_choice = { type: tool_choice === 'auto' ? 'auto' : 'any' };
        }

        return {
            url: `${baseUrl_}/v1/messages`,
            headers: {
                'Content-Type': 'application/json',
                'x-api-key': apiKey,
                'anthropic-version': '2023-06-01',
            },
            body: JSON.stringify(body, null, 2),
        };
    }

    // ── Gemini ───────────────────────────────────────────────
    if (channelType === ChannelType.Gemini) {
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        const base = templateWithModel as any;
        const body: Record<string, unknown> = {
            contents: toGeminiContents(base.messages),
            generationConfig: {
                temperature: base.temperature,
                topP: base.top_p,
            },
        };

        if (base.tools) {
            body.tools = [{ functionDeclarations: toGeminiFunctionDeclarations(base.tools) }];
        }

        // Gemini uses API key as query param
        return {
            url: `${baseUrl_}/v1beta/models/${model}:streamGenerateContent?alt=sse&key=${apiKey}`,
            headers: {
                'Content-Type': 'application/json',
            },
            body: JSON.stringify(body, null, 2),
        };
    }

    // Fallback: treat as OpenAI
    return {
        url: `${baseUrl_}/v1/chat/completions`,
        headers: {
            'Content-Type': 'application/json',
            Authorization: `Bearer ${apiKey}`,
        },
        body: JSON.stringify(templateWithModel, null, 2),
    };
}
