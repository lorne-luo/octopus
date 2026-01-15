'use client';

import { useState, useCallback, useMemo } from 'react';
import {
    MorphingDialogClose,
    MorphingDialogTitle,
    MorphingDialogDescription,
    useMorphingDialog,
} from '@/components/ui/morphing-dialog';
import {
    useHealthCreate,
    CheckInterval,
    type CreateHealthCheckRequest,
} from '@/api/endpoints/health';
import { useChannelList } from '@/api/endpoints/channel';
import { toast } from '@/components/common/Toast';
import { Field, FieldLabel, FieldGroup } from '@/components/ui/field';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Input } from '@/components/ui/input';
import { useTranslations } from 'next-intl';

export function CreateDialogContent() {
    const { setIsOpen } = useMorphingDialog();
    const createHealth = useHealthCreate();
    const t = useTranslations('health');
    const { data: channels } = useChannelList();

    const [channelId, setChannelId] = useState<string>('');
    const [modelName, setModelName] = useState('');
    const [interval, setInterval] = useState<CheckInterval>(CheckInterval.Hourly);
    const [prompt, setPrompt] = useState('hi');

    // Get models for the selected channel
    const availableModels = useMemo(() => {
        if (!channelId || !channels) return [];
        const selectedChannel = channels.find(c => c.raw.id.toString() === channelId);
        if (!selectedChannel) return [];

        const models: string[] = [];
        // Parse models from the model field (comma-separated)
        if (selectedChannel.raw.model) {
            models.push(...selectedChannel.raw.model.split(',').map(m => m.trim()).filter(Boolean));
        }
        // Parse models from the custom_model field (comma-separated)
        if (selectedChannel.raw.custom_model) {
            models.push(...selectedChannel.raw.custom_model.split(',').map(m => m.trim()).filter(Boolean));
        }
        return [...new Set(models)]; // Remove duplicates
    }, [channelId, channels]);

    // Handle channel change - reset model selection
    const handleChannelChange = useCallback((value: string) => {
        setChannelId(value);
        setModelName(''); // Reset model when channel changes
    }, []);

    const handleSubmit = useCallback((e: React.FormEvent) => {
        e.preventDefault();

        if (!channelId) {
            toast.error(t('form.channel') + '?');
            return;
        }

        const data: CreateHealthCheckRequest = {
            channel_id: parseInt(channelId, 10),
            model_name: modelName,
            interval: interval,
            prompt: prompt,
        };

        createHealth.mutate(data, {
            onSuccess: () => {
                toast.success(t('toast.created'));
                setIsOpen(false);
            },
            onError: (error) => {
                toast.error(t('toast.createFailed'), { description: error.message });
            },
        });
    }, [channelId, modelName, interval, prompt, createHealth, setIsOpen, t]);

    return (
        <div className="w-screen max-w-full md:max-w-lg min-h-0 flex flex-col">
            <MorphingDialogTitle className="shrink-0">
                <header className="mb-5 flex items-center justify-between">
                    <h2 className="text-2xl font-bold text-card-foreground">
                        {t('create.title')}
                    </h2>
                    <MorphingDialogClose
                        className="relative right-0 top-0"
                        variants={{
                            initial: { opacity: 0, scale: 0.8 },
                            animate: { opacity: 1, scale: 1 },
                            exit: { opacity: 0, scale: 0.8 },
                        }}
                    />
                </header>
            </MorphingDialogTitle>
            <MorphingDialogDescription className="flex-1 min-h-0 overflow-hidden">
                <form onSubmit={handleSubmit} className="flex flex-col gap-4">
                    <FieldGroup>
                        <Field>
                            <FieldLabel>{t('form.channel')}</FieldLabel>
                            <Select value={channelId} onValueChange={handleChannelChange}>
                                <SelectTrigger className="w-full">
                                    <SelectValue placeholder={t('form.channel')} />
                                </SelectTrigger>
                                <SelectContent>
                                    {channels?.map((channel) => (
                                        <SelectItem
                                            key={channel.raw.id}
                                            value={channel.raw.id.toString()}
                                        >
                                            {channel.raw.name}
                                        </SelectItem>
                                    ))}
                                </SelectContent>
                            </Select>
                        </Field>
                        <Field>
                            <FieldLabel>{t('form.modelName')}</FieldLabel>
                            {availableModels.length > 0 ? (
                                <Select value={modelName} onValueChange={setModelName}>
                                    <SelectTrigger className="w-full">
                                        <SelectValue placeholder={t('form.modelName')} />
                                    </SelectTrigger>
                                    <SelectContent>
                                        {availableModels.map((model) => (
                                            <SelectItem key={model} value={model}>
                                                {model}
                                            </SelectItem>
                                        ))}
                                    </SelectContent>
                                </Select>
                            ) : (
                                <Input
                                    value={modelName}
                                    onChange={(e) => setModelName(e.target.value)}
                                    placeholder="e.g., gpt-4, claude-3-opus"
                                    required
                                />
                            )}
                        </Field>
                        <Field>
                            <FieldLabel>{t('form.interval')}</FieldLabel>
                            <Select value={interval} onValueChange={(v) => setInterval(v as CheckInterval)}>
                                <SelectTrigger className="w-full">
                                    <SelectValue />
                                </SelectTrigger>
                                <SelectContent>
                                    <SelectItem value={CheckInterval.Hourly}>{t('interval.hourly')}</SelectItem>
                                    <SelectItem value={CheckInterval.Daily}>{t('interval.daily')}</SelectItem>
                                </SelectContent>
                            </Select>
                        </Field>
                        <Field>
                            <FieldLabel>{t('form.prompt')}</FieldLabel>
                            <textarea
                                value={prompt}
                                onChange={(e) => setPrompt(e.target.value)}
                                placeholder="e.g., hi"
                                required
                                rows={3}
                                className="placeholder:text-muted-foreground selection:bg-primary selection:text-primary-foreground dark:bg-input/30 border-input w-full min-w-0 rounded-md border bg-transparent px-3 py-2 text-base shadow-xs transition-[color,box-shadow] outline-none disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-50 md:text-sm focus-visible:border-ring focus-visible:ring-ring/50 focus-visible:ring-[3px] resize-none"
                            />
                        </Field>
                    </FieldGroup>
                    <div className="flex justify-end gap-2 pt-4">
                        <button
                            type="button"
                            onClick={() => setIsOpen(false)}
                            className="px-4 py-2 rounded-lg bg-muted hover:bg-muted/80 transition-colors"
                        >
                            {t('form.cancel')}
                        </button>
                        <button
                            type="submit"
                            disabled={createHealth.isPending}
                            className="px-4 py-2 rounded-lg bg-primary text-primary-foreground hover:bg-primary/90 transition-colors disabled:opacity-50"
                        >
                            {createHealth.isPending ? t('form.creating') : t('form.create')}
                        </button>
                    </div>
                </form>
            </MorphingDialogDescription>
        </div>
    );
}
