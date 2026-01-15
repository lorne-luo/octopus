'use client';

import { useState, useCallback, useMemo } from 'react';
import { Trash2, X, Pencil, Clock, AlertCircle, Play } from 'lucide-react';
import { motion, AnimatePresence } from 'motion/react';
import {
    type HealthCheckWithChannel,
    HealthCheckStatus,
    CheckInterval,
    useHealthDelete,
    useHealthUpdate,
    useHealthTest,
    type UpdateHealthCheckRequest,
} from '@/api/endpoints/health';
import { useChannelList } from '@/api/endpoints/channel';
import { cn } from '@/lib/utils';
import { toast } from '@/components/common/Toast';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/animate-ui/components/animate/tooltip';
import {
    MorphingDialog,
    MorphingDialogClose,
    MorphingDialogContainer,
    MorphingDialogContent,
    MorphingDialogDescription,
    MorphingDialogTitle,
    MorphingDialogTrigger,
    useMorphingDialog,
} from '@/components/ui/morphing-dialog';
import { Field, FieldLabel, FieldGroup } from '@/components/ui/field';
import { Input } from '@/components/ui/input';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { useTranslations } from 'next-intl';

interface HealthCardProps {
    healthCheck: HealthCheckWithChannel;
}

function StatusIndicator({ status }: { status: HealthCheckStatus }) {
    const t = useTranslations('health.card.status');
    const statusConfig = {
        [HealthCheckStatus.Healthy]: {
            color: 'bg-green-500',
            ring: 'ring-green-500/30',
            label: t('healthy'),
        },
        [HealthCheckStatus.Unhealthy]: {
            color: 'bg-red-500',
            ring: 'ring-red-500/30',
            label: t('unhealthy'),
        },
        [HealthCheckStatus.Unknown]: {
            color: 'bg-gray-400',
            ring: 'ring-gray-400/30',
            label: t('unknown'),
        },
        [HealthCheckStatus.Checking]: {
            color: 'bg-blue-500 animate-pulse',
            ring: 'ring-blue-500/30',
            label: t('checking'),
        },
    };

    const config = statusConfig[status] || statusConfig[HealthCheckStatus.Unknown];

    return (
        <Tooltip>
            <TooltipTrigger>
                <div className={cn(
                    'size-3 rounded-full ring-4',
                    config.color,
                    config.ring
                )} />
            </TooltipTrigger>
            <TooltipContent>{config.label}</TooltipContent>
        </Tooltip>
    );
}

function formatDateTime(dateStr?: string): string {
    if (!dateStr) return '-';
    const date = new Date(dateStr);
    return date.toLocaleString();
}

function EditDialogContent({ healthCheck }: { healthCheck: HealthCheckWithChannel }) {
    const { setIsOpen } = useMorphingDialog();
    const updateHealth = useHealthUpdate();
    const t = useTranslations('health');
    const { data: channels } = useChannelList();
    const [modelName, setModelName] = useState(healthCheck.model_name);
    const [interval, setInterval] = useState(healthCheck.interval);
    const [prompt, setPrompt] = useState(healthCheck.prompt);

    // Get models for the current channel
    const availableModels = useMemo(() => {
        if (!channels) return [];
        const channel = channels.find(c => c.raw.id === healthCheck.channel_id);
        if (!channel) return [];

        const models: string[] = [];
        // Parse models from the model field (comma-separated)
        if (channel.raw.model) {
            models.push(...channel.raw.model.split(',').map(m => m.trim()).filter(Boolean));
        }
        // Parse models from the custom_model field (comma-separated)
        if (channel.raw.custom_model) {
            models.push(...channel.raw.custom_model.split(',').map(m => m.trim()).filter(Boolean));
        }
        return [...new Set(models)]; // Remove duplicates
    }, [channels, healthCheck.channel_id]);

    const handleSubmit = useCallback((e: React.FormEvent) => {
        e.preventDefault();
        const data: UpdateHealthCheckRequest = {
            id: healthCheck.id,
            model_name: modelName,
            interval: interval,
            prompt: prompt,
        };
        updateHealth.mutate(data, {
            onSuccess: () => {
                toast.success(t('toast.updated'));
                setIsOpen(false);
            },
            onError: (error) => {
                toast.error(t('toast.updateFailed'), { description: error.message });
            },
        });
    }, [healthCheck.id, modelName, interval, prompt, updateHealth, setIsOpen, t]);

    return (
        <>
            <MorphingDialogTitle className="shrink-0">
                <header className="mb-3 flex items-center justify-between">
                    <h2 className="text-2xl font-bold text-card-foreground">
                        {t('edit.title')}
                    </h2>
                    <MorphingDialogClose className="relative right-0 top-0" />
                </header>
            </MorphingDialogTitle>
            <MorphingDialogDescription className="flex-1 min-h-0 overflow-hidden">
                <form onSubmit={handleSubmit} className="flex flex-col gap-4">
                    <FieldGroup>
                        <Field>
                            <FieldLabel>{t('form.channel')}</FieldLabel>
                            <Input value={healthCheck.channel_name} disabled className="bg-muted" />
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
                                    placeholder="e.g., gpt-4"
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
                                    <SelectItem value={CheckInterval.Minutely}>{t('interval.minutely')}</SelectItem>
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
                            disabled={updateHealth.isPending}
                            className="px-4 py-2 rounded-lg bg-primary text-primary-foreground hover:bg-primary/90 transition-colors disabled:opacity-50"
                        >
                            {updateHealth.isPending ? t('form.saving') : t('form.save')}
                        </button>
                    </div>
                </form>
            </MorphingDialogDescription>
        </>
    );
}

export function HealthCard({ healthCheck }: HealthCardProps) {
    const deleteHealth = useHealthDelete();
    const testHealth = useHealthTest();
    const t = useTranslations('health');
    const [confirmDelete, setConfirmDelete] = useState(false);

    const intervalLabel = healthCheck.interval === CheckInterval.Minutely
        ? t('interval.minutely')
        : healthCheck.interval === CheckInterval.Hourly
            ? t('interval.hourly')
            : t('interval.daily');
    const isChecking = healthCheck.status === HealthCheckStatus.Checking || testHealth.isPending;

    const handleTest = useCallback(() => {
        testHealth.mutate(healthCheck.id, {
            onSuccess: () => {
                toast.success(t('toast.testing'));
            },
            onError: (error) => {
                toast.error(t('toast.testFailed'), { description: error.message });
            },
        });
    }, [healthCheck.id, testHealth, t]);

    return (
        <article className="flex flex-col rounded-3xl border border-border bg-card text-card-foreground p-4 custom-shadow">
            <header className="flex items-start justify-between mb-3 relative overflow-visible rounded-xl -mx-1 px-1 -my-1 py-1">
                <div className="flex items-center gap-3 flex-1 min-w-0">
                    <StatusIndicator status={healthCheck.status} />
                    <div className="min-w-0">
                        <Tooltip>
                            <TooltipTrigger asChild>
                                <h3 className="text-lg font-bold truncate">{healthCheck.channel_name}</h3>
                            </TooltipTrigger>
                            <TooltipContent>{healthCheck.channel_name}</TooltipContent>
                        </Tooltip>
                        <p className="text-sm text-muted-foreground truncate">{healthCheck.model_name}</p>
                    </div>
                </div>

                <div className="flex items-center gap-1 shrink-0">
                    <Tooltip>
                        <TooltipTrigger asChild>
                            <button
                                type="button"
                                onClick={handleTest}
                                disabled={isChecking}
                                className="p-1.5 rounded-lg transition-colors hover:bg-primary/10 text-muted-foreground hover:text-primary disabled:opacity-50 disabled:cursor-not-allowed"
                            >
                                <Play className={cn("size-4", isChecking && "animate-pulse")} />
                            </button>
                        </TooltipTrigger>
                        <TooltipContent>{t('card.actions.test')}</TooltipContent>
                    </Tooltip>

                    <MorphingDialog>
                        <MorphingDialogTrigger className="p-1.5 rounded-lg transition-colors hover:bg-muted text-muted-foreground hover:text-foreground">
                            <Tooltip>
                                <TooltipTrigger asChild>
                                    <Pencil className="size-4" />
                                </TooltipTrigger>
                                <TooltipContent>{t('card.actions.edit')}</TooltipContent>
                            </Tooltip>
                        </MorphingDialogTrigger>

                        <MorphingDialogContainer>
                            <MorphingDialogContent className="relative w-screen max-w-full md:max-w-lg bg-card text-card-foreground px-6 py-4 rounded-3xl custom-shadow">
                                <EditDialogContent healthCheck={healthCheck} />
                            </MorphingDialogContent>
                        </MorphingDialogContainer>
                    </MorphingDialog>

                    {!confirmDelete && (
                        <Tooltip>
                            <TooltipTrigger>
                                <motion.button
                                    layoutId={`delete-btn-health-${healthCheck.id}`}
                                    type="button"
                                    onClick={() => setConfirmDelete(true)}
                                    className="p-1.5 rounded-lg hover:bg-destructive/10 text-muted-foreground hover:text-destructive transition-colors"
                                >
                                    <Trash2 className="size-4" />
                                </motion.button>
                            </TooltipTrigger>
                            <TooltipContent>{t('card.actions.delete')}</TooltipContent>
                        </Tooltip>
                    )}
                </div>

                <AnimatePresence>
                    {confirmDelete && (
                        <motion.div
                            layoutId={`delete-btn-health-${healthCheck.id}`}
                            className="absolute inset-0 flex items-center justify-center gap-2 bg-destructive p-2 rounded-xl"
                            transition={{ type: 'spring', stiffness: 400, damping: 30 }}
                        >
                            <button
                                type="button"
                                onClick={() => setConfirmDelete(false)}
                                className="flex h-7 w-7 items-center justify-center rounded-lg bg-destructive-foreground/20 text-destructive-foreground transition-all hover:bg-destructive-foreground/30 active:scale-95"
                            >
                                <X className="size-4" />
                            </button>
                            <button
                                type="button"
                                onClick={() =>
                                    deleteHealth.mutate(healthCheck.id, {
                                        onSuccess: () => toast.success(t('toast.deleted')),
                                    })
                                }
                                disabled={deleteHealth.isPending}
                                className="flex-1 h-7 flex items-center justify-center gap-2 rounded-lg bg-destructive-foreground text-destructive text-sm font-semibold transition-all hover:bg-destructive-foreground/90 active:scale-[0.98] disabled:opacity-50 disabled:cursor-not-allowed"
                            >
                                <Trash2 className="size-3.5" />
                                {t('card.actions.confirmDelete')}
                            </button>
                        </motion.div>
                    )}
                </AnimatePresence>
            </header>

            {/* Interval Badge */}
            <div className="flex gap-2 mb-3">
                <span className="px-2 py-1 text-xs rounded-lg bg-muted flex items-center gap-1">
                    <Clock className="size-3" />
                    {intervalLabel}
                </span>
            </div>

            {/* Info Section */}
            <section className="rounded-xl border border-border/50 bg-muted/30 p-3 space-y-2 text-sm">
                <div className="flex justify-between">
                    <span className="text-muted-foreground">{t('card.prompt')}</span>
                    <Tooltip>
                        <TooltipTrigger asChild>
                            <span className="font-mono truncate max-w-[60%]">
                                {healthCheck.prompt.length > 40
                                    ? healthCheck.prompt.slice(0, 40) + '...'
                                    : healthCheck.prompt}
                            </span>
                        </TooltipTrigger>
                        {healthCheck.prompt.length > 40 && (
                            <TooltipContent className="max-w-xs break-all">
                                {healthCheck.prompt}
                            </TooltipContent>
                        )}
                    </Tooltip>
                </div>
                <div className="flex justify-between">
                    <span className="text-muted-foreground">{t('card.lastCheck')}</span>
                    <span>{formatDateTime(healthCheck.last_check)}</span>
                </div>
                <div className="flex justify-between">
                    <span className="text-muted-foreground">{t('card.nextCheck')}</span>
                    <span>{formatDateTime(healthCheck.next_check)}</span>
                </div>

                {healthCheck.last_error && (
                    <div className="flex items-start gap-2 text-destructive bg-destructive/10 rounded-lg p-2 mt-2">
                        <AlertCircle className="size-4 shrink-0 mt-0.5" />
                        <span className="text-xs break-all">{healthCheck.last_error}</span>
                    </div>
                )}
            </section>
        </article>
    );
}
