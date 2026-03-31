'use client';

import { useRef, useEffect, useMemo } from 'react';
import { useTranslations } from 'next-intl';
import { useTheme } from 'next-themes';
import { Loader2 } from 'lucide-react';
import JsonView from '@uiw/react-json-view';
import { githubDarkTheme } from '@uiw/react-json-view/githubDark';
import { githubLightTheme } from '@uiw/react-json-view/githubLight';
import { cn } from '@/lib/utils';
import type { BuiltRequest } from './request-builder';
import type { ResultData } from './index';

interface ResultPanelProps {
    result: ResultData;
}

export function ResultPanel({ result }: ResultPanelProps) {
    const t = useTranslations('testmodel.result');
    const { resolvedTheme } = useTheme();
    const bottomRef = useRef<HTMLDivElement>(null);

    // Auto-scroll to bottom while streaming
    useEffect(() => {
        if (result.streaming && bottomRef.current) {
            bottomRef.current.scrollIntoView({ behavior: 'smooth' });
        }
    }, [result.rawContent, result.streaming]);

    const isEmpty = !result.request && !result.rawContent && !result.mergedJson && !result.error;

    const jsonStyle = useMemo(() => ({
        ...(resolvedTheme === 'dark' ? githubDarkTheme : githubLightTheme),
        fontSize: '12px',
        fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace',
        backgroundColor: 'transparent',
    }), [resolvedTheme]);

    return (
        <div className="flex flex-col h-full gap-3 min-h-0">
            {/* Request Info */}
            {result.request && (
                <details className="group flex-none">
                    <summary className="cursor-pointer select-none text-sm font-medium text-muted-foreground hover:text-foreground transition-colors flex items-center gap-1.5 py-1">
                        <span className="transition-transform group-open:rotate-90">▶</span>
                        {t('requestInfo')}
                        {result.elapsed !== null && (
                            <span className="ml-auto text-xs text-muted-foreground">
                                {t('elapsed')}: {result.elapsed}ms
                            </span>
                        )}
                    </summary>
                    <div className="mt-2 rounded-lg border bg-muted/30 overflow-hidden text-xs">
                        {/* URL */}
                        <div className="px-3 py-2 border-b bg-muted/50">
                            <span className="font-semibold text-primary">POST </span>
                            <span className="break-all font-mono">{result.request.url}</span>
                        </div>
                        {/* Headers */}
                        <div className="px-3 py-2 border-b">
                            <div className="font-semibold mb-1 text-muted-foreground">Headers</div>
                            {Object.entries(result.request.headers).map(([k, v]) => (
                                <div key={k} className="font-mono flex gap-2 flex-wrap">
                                    <span className="text-blue-500 dark:text-blue-400">{k}:</span>
                                    <span className="break-all">
                                        {k.toLowerCase().includes('key') || k.toLowerCase().includes('authorization')
                                            ? maskSecret(v)
                                            : v}
                                    </span>
                                </div>
                            ))}
                        </div>
                        {/* Body */}
                        <div className="px-3 py-2 overflow-x-auto max-h-48 text-xs font-mono">
                            {(() => {
                                try {
                                    if (!result.request.body) return null;
                                    const parsedBody = JSON.parse(result.request.body) as object;
                                    return (
                                        <div className="-ml-2">
                                            <JsonView
                                                value={parsedBody}
                                                style={jsonStyle}
                                                displayDataTypes={false}
                                                displayObjectSize={false}
                                                collapsed={false}
                                            />
                                        </div>
                                    );
                                } catch {
                                    return (
                                        <pre className="whitespace-pre-wrap break-all">
                                            {result.request.body}
                                        </pre>
                                    );
                                }
                            })()}
                        </div>
                    </div>
                </details>
            )}

            {/* Response */}
            <div className="flex-1 flex flex-col min-h-0">
                <div className="flex items-center justify-between mb-2 flex-none">
                    <div className="flex items-center gap-2">
                        <span className="text-sm font-medium text-muted-foreground">{t('responseInfo')}</span>
                        {result.streaming && (
                            <Loader2 className="size-3.5 text-primary animate-spin" />
                        )}
                    </div>
                    {result.streaming && (
                        <span className="text-xs text-primary animate-pulse">{t('streaming')}</span>
                    )}
                </div>

                <div
                    className={cn(
                        'flex-1 rounded-lg border overflow-auto min-h-0',
                        result.error ? 'border-destructive/50 bg-destructive/5' : 'bg-muted/20'
                    )}
                >
                    {isEmpty ? (
                        <div className="h-full flex items-center justify-center text-muted-foreground text-sm">
                            {t('empty')}
                        </div>
                    ) : result.error ? (
                        <div className="p-4">
                            <div className="text-sm font-semibold text-destructive mb-1">{t('error')}</div>
                            <pre className="text-xs text-destructive/80 whitespace-pre-wrap break-all font-mono">
                                {result.error}
                            </pre>
                        </div>
                    ) : (
                        <div className="p-4 flex flex-col gap-4">
                            {result.mergedJson ? (
                                <JsonView
                                    value={result.mergedJson as object}
                                    style={jsonStyle}
                                    displayDataTypes={false}
                                    displayObjectSize={false}
                                    collapsed={false}
                                />
                            ) : (
                                <pre className="text-xs font-mono text-muted-foreground whitespace-pre-wrap break-all">
                                    {result.rawContent}
                                </pre>
                            )}
                            <div ref={bottomRef} />
                        </div>
                    )}
                </div>
            </div>
        </div>
    );
}

function maskSecret(value: string): string {
    if (value.length <= 12) return '***';
    const prefix = value.startsWith('Bearer ') ? 'Bearer ' : '';
    const raw = prefix ? value.slice(7) : value;
    if (raw.length <= 8) return prefix + '***';
    return prefix + raw.slice(0, 4) + '***' + raw.slice(-4);
}
