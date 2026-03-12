'use client';

import { useState, useCallback } from 'react';
import { useTranslations } from 'next-intl';
import { ConfigPanel } from './ConfigPanel';
import { ResultPanel } from './ResultPanel';
import { buildRequest, type TestType, type BuiltRequest } from './request-builder';

export interface ResultData {
    request: BuiltRequest | null;
    rawContent: string;     // merged plain text or full response string
    mergedJson: unknown | null; // assembled json object if possible
    error: string | null;
    elapsed: number | null;
    streaming: boolean;
}

const EMPTY_RESULT: ResultData = {
    request: null,
    rawContent: '',
    mergedJson: null,
    error: null,
    elapsed: null,
    streaming: false,
};

export function TestModel() {
    const t = useTranslations('testmodel');
    const [sending, setSending] = useState(false);
    const [result, setResult] = useState<ResultData>(EMPTY_RESULT);

    const handleSend = useCallback(
        async (cfg: {
            channelId: number;
            keyIndex: number;
            baseUrlIndex: number;
            model: string;
            testType: TestType;
        }) => {
            const built = buildRequest(cfg);
            setSending(true);
            setResult({
                request: built,
                rawContent: '',
                mergedJson: null,
                error: null,
                elapsed: null,
                streaming: true,
            });

            const start = Date.now();

            try {
                const response = await fetch(built.url, {
                    method: 'POST',
                    headers: built.headers,
                    body: built.body,
                });

                if (!response.ok) {
                    const text = await response.text();
                    setResult((prev) => ({
                        ...prev,
                        error: `HTTP ${response.status}: ${text}`,
                        elapsed: Date.now() - start,
                        streaming: false,
                    }));
                    return;
                }

                const body = response.body;
                if (!body) {
                    const text = await response.text();
                    setResult((prev) => ({
                        ...prev,
                        rawContent: text,
                        elapsed: Date.now() - start,
                        streaming: false,
                    }));
                    return;
                }

                const reader = body.getReader();
                const decoder = new TextDecoder();
                let buffer = '';

                // eslint-disable-next-line @typescript-eslint/no-explicit-any
                let mergedObj: Record<string, any> | null = null;
                let mergedText = '';

                while (true) {
                    const { done, value } = await reader.read();
                    if (done) break;

                    buffer += decoder.decode(value, { stream: true });
                    const lines = buffer.split('\n');
                    buffer = lines.pop() ?? '';

                    for (const line of lines) {
                        const trimmed = line.trim();
                        if (!trimmed || trimmed === 'data: [DONE]') continue;

                        if (trimmed.startsWith('data: ')) {
                            const jsonStr = trimmed.slice(6);
                            try {
                                // eslint-disable-next-line @typescript-eslint/no-explicit-any
                                const parsed = JSON.parse(jsonStr) as Record<string, any>;

                                // Backend always outputs OpenAI-compatible SSE format
                                if (!mergedObj) {
                                    mergedObj = {
                                        id: parsed.id,
                                        object: 'chat.completion',
                                        created: parsed.created,
                                        model: parsed.model,
                                        choices: [{
                                            index: 0,
                                            message: {
                                                role: 'assistant',
                                                content: '',
                                            },
                                        }],
                                    };
                                    if (parsed.choices?.[0]?.delta?.tool_calls) {
                                        mergedObj.choices[0].message.tool_calls = JSON.parse(JSON.stringify(parsed.choices[0].delta.tool_calls));
                                    }
                                }

                                // Merge content
                                const deltaText = parsed.choices?.[0]?.delta?.content || '';
                                if (deltaText) {
                                    mergedText += deltaText;
                                    mergedObj.choices[0].message.content = mergedText;
                                }

                                // Merge reasoning_content
                                const deltaReasoning = parsed.choices?.[0]?.delta?.reasoning_content || '';
                                if (deltaReasoning) {
                                    mergedObj.choices[0].message.reasoning_content =
                                        (mergedObj.choices[0].message.reasoning_content || '') + deltaReasoning;
                                }

                                // Merge tool_calls
                                const deltaToolCalls = parsed.choices?.[0]?.delta?.tool_calls;
                                if (deltaToolCalls) {
                                    mergedObj.choices[0].message.tool_calls = mergedObj.choices[0].message.tool_calls || [];
                                    for (const tool of deltaToolCalls) {
                                        const idx = tool.index;
                                        if (!mergedObj.choices[0].message.tool_calls[idx]) {
                                            mergedObj.choices[0].message.tool_calls[idx] = tool;
                                        } else {
                                            if (tool.function?.arguments) {
                                                mergedObj.choices[0].message.tool_calls[idx].function.arguments += tool.function.arguments;
                                            }
                                        }
                                    }
                                }

                                // Merge finish_reason
                                const finishReason = parsed.choices?.[0]?.finish_reason;
                                if (finishReason) {
                                    mergedObj.choices[0].finish_reason = finishReason;
                                }

                                // Merge usage (usually in the last chunk)
                                if (parsed.usage) {
                                    mergedObj.usage = parsed.usage;
                                }

                                setResult((prev) => ({
                                    ...prev,
                                    mergedJson: mergedObj ? structuredClone(mergedObj) : null,
                                    rawContent: mergedText
                                }));
                            } catch {
                                mergedText += trimmed + '\n';
                                setResult((prev) => ({
                                    ...prev,
                                    rawContent: mergedText,
                                    mergedJson: mergedObj || { raw: mergedText }
                                }));
                            }
                        }
                    }
                }

                setResult((prev) => ({
                    ...prev,
                    elapsed: Date.now() - start,
                    streaming: false,
                }));
            } catch (err) {
                setResult((prev) => ({
                    ...prev,
                    error: err instanceof Error ? err.message : String(err),
                    elapsed: Date.now() - start,
                    streaming: false,
                }));
            } finally {
                setSending(false);
            }
        },
        []
    );

    const handleClear = useCallback(() => {
        setResult(EMPTY_RESULT);
    }, []);

    return (
        <div className="flex flex-col md:grid md:grid-cols-[300px_1fr] md:h-full gap-4 min-h-0">
            {/* Left: Config panel */}
            <div className="flex-none md:flex-auto border rounded-xl p-4 bg-card md:overflow-y-auto">
                <ConfigPanel onSend={handleSend} onClear={handleClear} sending={sending} />
            </div>

            {/* Right: Result panel */}
            <div className="flex-1 border rounded-xl p-4 bg-card md:min-h-0 min-h-[500px] md:overflow-hidden flex flex-col">
                <div className="text-sm font-medium mb-3 flex-none">{t('result.title')}</div>
                <div className="flex-1 min-h-0">
                    <ResultPanel result={result} />
                </div>
            </div>
        </div>
    );
}
