'use client';

import { useState, useEffect, useRef } from 'react';
import { useTranslations } from 'next-intl';
import { ChevronDown, Search, Check } from 'lucide-react';
import { useChannelList, ChannelType } from '@/api/endpoints/channel';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { cn } from '@/lib/utils';
import {
    Select,
    SelectContent,
    SelectItem,
    SelectTrigger,
    SelectValue,
} from '@/components/ui/select';
import type { TestType } from './request-builder';

interface ConfigPanelProps {
    onSend: (cfg: {
        channelType: ChannelType;
        baseUrl: string;
        apiKey: string;
        model: string;
        testType: TestType;
    }) => void;
    onClear: () => void;
    sending: boolean;
}

const TEST_TYPES: TestType[] = ['text_chat', 'vision_chat', 'tool_chat'];

// ─── Searchable combobox for Model ─────────────────────────────────────────

function ModelCombobox({
    options,
    value,
    onChange,
    placeholder,
}: {
    options: string[];
    value: string;
    onChange: (v: string) => void;
    placeholder: string;
}) {
    const [open, setOpen] = useState(false);
    const [search, setSearch] = useState('');
    const ref = useRef<HTMLDivElement>(null);

    const filtered = options.filter((o) => o.toLowerCase().includes(search.toLowerCase()));
    
    // Close on outside click
    useEffect(() => {
        const handler = (e: MouseEvent) => {
            if (ref.current && !ref.current.contains(e.target as Node)) {
                setOpen(false);
            }
        };
        document.addEventListener('mousedown', handler);
        return () => document.removeEventListener('mousedown', handler);
    }, []);

    return (
        <div ref={ref} className="relative">
            <button
                type="button"
                onClick={() => { setOpen((o) => !o); setSearch(''); }}
                className={cn(
                    'flex w-full items-center justify-between',
                    'rounded-xl border border-border bg-background px-3 py-2 text-sm',
                    'hover:bg-muted/30 transition-colors',
                    'focus:outline-none focus:ring-2 focus:ring-ring',
                    !value && 'text-muted-foreground'
                )}
            >
                <span className="font-mono truncate">
                    {value || placeholder}
                </span>
                <ChevronDown className={cn('h-4 w-4 shrink-0 text-muted-foreground transition-transform', open && 'rotate-180')} />
            </button>

            {open && (
                <div className="absolute z-50 mt-1 w-full rounded-xl border border-border bg-popover shadow-lg overflow-hidden">
                    {/* Search input */}
                    <div className="flex items-center gap-2 px-3 py-2 border-b border-border">
                        <Search className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                        <input
                            autoFocus
                            value={search}
                            onChange={(e) => setSearch(e.target.value)}
                            placeholder="Search..."
                            className="flex-1 bg-transparent text-sm outline-none placeholder:text-muted-foreground"
                        />
                    </div>

                    {/* Options list */}
                    <div className="max-h-[50vh] overflow-y-auto py-1">
                        {filtered.length === 0 ? (
                            <div className="px-3 py-2 text-sm text-muted-foreground text-center">No results</div>
                        ) : (
                            filtered.map((o) => (
                                <button
                                    key={o}
                                    type="button"
                                    onClick={() => { onChange(o); setOpen(false); }}
                                    className={cn(
                                        'w-full flex items-center gap-2 px-3 py-2 text-sm text-left',
                                        'hover:bg-muted/50 transition-colors',
                                        o === value && 'bg-muted/30'
                                    )}
                                >
                                    <Check className={cn('h-3.5 w-3.5 shrink-0', o === value ? 'opacity-100' : 'opacity-0')} />
                                    <span className="font-mono">{o}</span>
                                </button>
                            ))
                        )}
                    </div>
                </div>
            )}
        </div>
    );
}

// ─── Searchable combobox for Channel ───────────────────────────────────────

function ChannelCombobox({
    options,
    value,
    onChange,
    placeholder,
}: {
    options: { id: number | ''; name: string }[];
    value: number | '';
    onChange: (v: number | '') => void;
    placeholder: string;
}) {
    const [open, setOpen] = useState(false);
    const [search, setSearch] = useState('');
    const ref = useRef<HTMLDivElement>(null);

    const filtered = options.filter((o) => o.name.toLowerCase().includes(search.toLowerCase()) || (o.id !== '' && String(o.id).includes(search)));
    
    // Close on outside click
    useEffect(() => {
        const handler = (e: MouseEvent) => {
            if (ref.current && !ref.current.contains(e.target as Node)) {
                setOpen(false);
            }
        };
        document.addEventListener('mousedown', handler);
        return () => document.removeEventListener('mousedown', handler);
    }, []);

    const selectedOption = options.find(o => o.id === value);

    return (
        <div ref={ref} className="relative">
            <button
                type="button"
                onClick={() => { setOpen((o) => !o); setSearch(''); }}
                className={cn(
                    'flex w-full items-center justify-between',
                    'rounded-xl border border-border bg-background px-3 py-2 text-sm',
                    'hover:bg-muted/30 transition-colors',
                    'focus:outline-none focus:ring-2 focus:ring-ring',
                    value === '' && 'text-muted-foreground'
                )}
            >
                <span className="truncate">
                    {selectedOption ? selectedOption.name : placeholder}
                </span>
                <ChevronDown className={cn('h-4 w-4 shrink-0 text-muted-foreground transition-transform', open && 'rotate-180')} />
            </button>

            {open && (
                <div className="absolute z-50 mt-1 w-full rounded-xl border border-border bg-popover shadow-lg overflow-hidden">
                    {/* Search input */}
                    <div className="flex items-center gap-2 px-3 py-2 border-b border-border">
                        <Search className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                        <input
                            autoFocus
                            value={search}
                            onChange={(e) => setSearch(e.target.value)}
                            placeholder="Search..."
                            className="flex-1 bg-transparent text-sm outline-none placeholder:text-muted-foreground"
                        />
                    </div>

                    {/* Options list */}
                    <div className="max-h-[60vh] overflow-y-auto py-1">
                        {filtered.length === 0 ? (
                            <div className="px-3 py-2 text-sm text-muted-foreground text-center">No results</div>
                        ) : (
                            filtered.map((o) => (
                                <button
                                    key={o.id}
                                    type="button"
                                    onClick={() => { onChange(o.id); setOpen(false); }}
                                    className={cn(
                                        'w-full flex items-center gap-2 px-3 py-2 text-sm text-left',
                                        'hover:bg-muted/50 transition-colors',
                                        o.id === value && 'bg-muted/30'
                                    )}
                                >
                                    <Check className={cn('h-3.5 w-3.5 shrink-0', o.id === value ? 'opacity-100' : 'opacity-0')} />
                                    <span>{o.name}</span>
                                </button>
                            ))
                        )}
                    </div>
                </div>
            )}
        </div>
    );
}

// ─── Main ConfigPanel ─────────────────────────────────────────────────────────

export function ConfigPanel({ onSend, onClear, sending }: ConfigPanelProps) {
    const t = useTranslations('testmodel.configPanel');
    const { data: channels } = useChannelList();

    const [selectedChannelId, setSelectedChannelId] = useState<number | ''>('');
    const [channelType, setChannelType] = useState<ChannelType>(ChannelType.OpenAIChat);
    const [baseUrl, setBaseUrl] = useState('');
    const [apiKey, setApiKey] = useState('');
    const [model, setModel] = useState('');
    const [testType, setTestType] = useState<TestType>('text_chat');

    const activeChannels = [...(channels?.filter(c => c.raw.enabled) ?? [])].sort((a, b) => b.raw.id - a.raw.id);

    // Default to last active channel when channels load
    useEffect(() => {
        if (activeChannels.length > 0 && selectedChannelId === '') {
            setSelectedChannelId(activeChannels[activeChannels.length - 1].raw.id);
        }
    }, [channels]); // eslint-disable-line react-hooks/exhaustive-deps

    // Derived options from selected channel
    const selectedChannel = channels?.find((c) => c.raw.id === selectedChannelId)?.raw ?? null;
    const baseUrlOptions = selectedChannel?.base_urls ?? [];
    const keyOptions = selectedChannel?.keys ?? [];
    const modelOptions: string[] = selectedChannel
        ? [
              ...selectedChannel.model.split(',').map((m) => m.trim()).filter(Boolean),
              ...(selectedChannel.custom_model ? selectedChannel.custom_model.split(',').map((m) => m.trim()).filter(Boolean) : []),
          ].filter((v, i, arr) => arr.indexOf(v) === i) // deduplicate
        : [];

    // Auto-fill when channel changes
    useEffect(() => {
        if (!selectedChannel) return;
        setChannelType(selectedChannel.type);
        setBaseUrl(selectedChannel.base_urls?.[0]?.url ?? '');
        const firstKey = selectedChannel.keys?.find((k) => k.enabled)?.channel_key
            ?? selectedChannel.keys?.[0]?.channel_key ?? '';
        setApiKey(firstKey);
        
        // Compute modelOptions instantly to auto-fill the first available model
        const instantModelOptions = [
              ...selectedChannel.model.split(',').map((m) => m.trim()).filter(Boolean),
              ...(selectedChannel.custom_model ? selectedChannel.custom_model.split(',').map((m) => m.trim()).filter(Boolean) : []),
        ].filter((v, i, arr) => arr.indexOf(v) === i);
        
        setModel(instantModelOptions[0] ?? '');
    }, [selectedChannelId]); // eslint-disable-line react-hooks/exhaustive-deps

    const hasChannel = selectedChannelId !== '';
    const canSend = !sending && !!baseUrl && !!apiKey && !!model;

    const testTypeLabels: Record<TestType, string> = {
        text_chat: t('textChat'),
        vision_chat: t('visionChat'),
        tool_chat: t('toolChat'),
    };

    return (
        <div className="space-y-4 px-1">

            {/* Channel */}
            <div className="space-y-2">
                <label className="text-sm font-medium text-card-foreground">{t('selectChannel')}</label>
                <ChannelCombobox
                    options={[
                        { id: '', name: t('orManual') },
                        ...activeChannels.map(c => ({ id: c.raw.id, name: c.raw.name }))
                    ]}
                    value={selectedChannelId}
                    onChange={(val) => {
                        if (val === '') {
                            setSelectedChannelId('');
                            setBaseUrl(''); setApiKey(''); setModel('');
                        } else {
                            setSelectedChannelId(val);
                        }
                        onClear(); // clear results on channel switch
                    }}
                    placeholder={t('selectChannel')}
                />
            </div>

            {/* Base URL */}
            <div className="space-y-2">
                <label className="text-sm font-medium text-card-foreground">{t('baseUrl')}</label>
                {hasChannel && baseUrlOptions.length > 0 ? (
                    <Select value={baseUrl} onValueChange={setBaseUrl}>
                        <SelectTrigger className="rounded-xl w-full border border-border px-4 py-2 text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring font-mono text-xs">
                            <SelectValue />
                        </SelectTrigger>
                        <SelectContent className="rounded-xl">
                            {baseUrlOptions.map((u, i) => (
                                <SelectItem key={i} className="rounded-xl font-mono text-xs" value={u.url}>
                                    {u.url}{u.delay > 0 ? ` (${u.delay}ms)` : ''}
                                </SelectItem>
                            ))}
                        </SelectContent>
                    </Select>
                ) : (
                    <Input
                        value={baseUrl}
                        onChange={(e) => setBaseUrl(e.target.value)}
                        placeholder="https://api.openai.com"
                        className="rounded-xl font-mono text-xs"
                    />
                )}
            </div>

            {/* API Key */}
            <div className="space-y-2">
                <label className="text-sm font-medium text-card-foreground">{t('apiKey')}</label>
                {hasChannel && keyOptions.length > 0 ? (
                    <Select value={apiKey} onValueChange={setApiKey}>
                        <SelectTrigger className="rounded-xl w-full border border-border px-4 py-2 text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring font-mono text-xs">
                            <SelectValue />
                        </SelectTrigger>
                        <SelectContent className="rounded-xl">
                            {keyOptions.map((k) => (
                                <SelectItem key={k.channel_key} className="rounded-xl font-mono text-xs" value={k.channel_key}>
                                    <span className="flex items-center gap-2">
                                        <span>{maskKey(k.channel_key)}</span>
                                        {k.remark && <span className="text-muted-foreground">— {k.remark}</span>}
                                        {!k.enabled && <span className="text-destructive/70">(disabled)</span>}
                                    </span>
                                </SelectItem>
                            ))}
                        </SelectContent>
                    </Select>
                ) : (
                    <Input
                        type="password"
                        value={apiKey}
                        onChange={(e) => setApiKey(e.target.value)}
                        placeholder="sk-..."
                        className="rounded-xl font-mono text-xs"
                    />
                )}
            </div>

            {/* Model — searchable combobox */}
            <div className="space-y-2">
                <label className="text-sm font-medium text-card-foreground">{t('model')}</label>
                {hasChannel && modelOptions.length > 0 ? (
                    <ModelCombobox
                        options={modelOptions}
                        value={model}
                        onChange={setModel}
                        placeholder={t('model')}
                    />
                ) : (
                    <Input
                        value={model}
                        onChange={(e) => setModel(e.target.value)}
                        placeholder="gpt-4o"
                        className="rounded-xl font-mono text-xs"
                    />
                )}
            </div>

            {/* Test type */}
            <div className="space-y-2">
                <label className="text-sm font-medium text-card-foreground">{t('testType')}</label>
                <div className="flex flex-col gap-1.5">
                    {TEST_TYPES.map((type) => (
                        <button
                            key={type}
                            type="button"
                            onClick={() => setTestType(type)}
                            className={cn(
                                'w-full text-left px-3 py-2 rounded-xl text-sm transition-colors border',
                                testType === type
                                    ? 'bg-primary text-primary-foreground border-primary'
                                    : 'bg-background hover:bg-muted border-border text-card-foreground'
                            )}
                        >
                            {testTypeLabels[type]}
                        </button>
                    ))}
                </div>
            </div>

            {/* Actions */}
            <div className={`flex flex-col gap-3 pt-2 sm:flex-row`}>
                <Button
                    type="button"
                    variant="secondary"
                    onClick={onClear}
                    disabled={sending}
                    className="w-full sm:flex-1 rounded-2xl h-12"
                >
                    {t('clear')}
                </Button>
                <Button
                    type="button"
                    onClick={() => {
                        if (!canSend) return;
                        onSend({ channelType, baseUrl, apiKey, model, testType });
                    }}
                    disabled={!canSend}
                    className="w-full sm:flex-1 rounded-2xl h-12"
                >
                    {sending ? t('sending') : t('send')}
                </Button>
            </div>
        </div>
    );
}

function maskKey(key: string): string {
    if (key.length <= 8) return '***';
    return key.slice(0, 4) + '···' + key.slice(-4);
}
