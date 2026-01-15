'use client';

import { useState, useCallback } from 'react';
import { Trash2, X, Pencil, Clock, AlertCircle } from 'lucide-react';
import { motion, AnimatePresence } from 'motion/react';
import {
    type HealthCheckWithChannel,
    HealthCheckStatus,
    CheckInterval,
    useHealthDelete,
    useHealthUpdate,
    type UpdateHealthCheckRequest,
} from '@/api/endpoints/health';
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

interface HealthCardProps {
    healthCheck: HealthCheckWithChannel;
}

function StatusIndicator({ status }: { status: HealthCheckStatus }) {
    const statusConfig = {
        [HealthCheckStatus.Healthy]: {
            color: 'bg-green-500',
            ring: 'ring-green-500/30',
            label: 'Healthy',
        },
        [HealthCheckStatus.Unhealthy]: {
            color: 'bg-red-500',
            ring: 'ring-red-500/30',
            label: 'Unhealthy',
        },
        [HealthCheckStatus.Unknown]: {
            color: 'bg-gray-400',
            ring: 'ring-gray-400/30',
            label: 'Unknown',
        },
        [HealthCheckStatus.Checking]: {
            color: 'bg-blue-500 animate-pulse',
            ring: 'ring-blue-500/30',
            label: 'Checking',
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
    const [modelName, setModelName] = useState(healthCheck.model_name);
    const [interval, setInterval] = useState(healthCheck.interval);
    const [prompt, setPrompt] = useState(healthCheck.prompt);

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
                toast.success('Health check updated');
                setIsOpen(false);
            },
            onError: (error) => {
                toast.error('Update failed', { description: error.message });
            },
        });
    }, [healthCheck.id, modelName, interval, prompt, updateHealth, setIsOpen]);

    return (
        <>
            <MorphingDialogTitle className="shrink-0">
                <header className="mb-3 flex items-center justify-between">
                    <h2 className="text-2xl font-bold text-card-foreground">
                        Edit Health Check
                    </h2>
                    <MorphingDialogClose className="relative right-0 top-0" />
                </header>
            </MorphingDialogTitle>
            <MorphingDialogDescription className="flex-1 min-h-0 overflow-hidden">
                <form onSubmit={handleSubmit} className="flex flex-col gap-4">
                    <FieldGroup>
                        <Field>
                            <FieldLabel>Channel</FieldLabel>
                            <Input value={healthCheck.channel_name} disabled className="bg-muted" />
                        </Field>
                        <Field>
                            <FieldLabel>Model Name</FieldLabel>
                            <Input
                                value={modelName}
                                onChange={(e) => setModelName(e.target.value)}
                                placeholder="e.g., gpt-4"
                                required
                            />
                        </Field>
                        <Field>
                            <FieldLabel>Check Interval</FieldLabel>
                            <Select value={interval} onValueChange={(v) => setInterval(v as CheckInterval)}>
                                <SelectTrigger className="w-full">
                                    <SelectValue />
                                </SelectTrigger>
                                <SelectContent>
                                    <SelectItem value={CheckInterval.Hourly}>Hourly</SelectItem>
                                    <SelectItem value={CheckInterval.Daily}>Daily</SelectItem>
                                </SelectContent>
                            </Select>
                        </Field>
                        <Field>
                            <FieldLabel>Prompt</FieldLabel>
                            <Input
                                value={prompt}
                                onChange={(e) => setPrompt(e.target.value)}
                                placeholder="e.g., hi"
                                required
                            />
                        </Field>
                    </FieldGroup>
                    <div className="flex justify-end gap-2 pt-4">
                        <button
                            type="button"
                            onClick={() => setIsOpen(false)}
                            className="px-4 py-2 rounded-lg bg-muted hover:bg-muted/80 transition-colors"
                        >
                            Cancel
                        </button>
                        <button
                            type="submit"
                            disabled={updateHealth.isPending}
                            className="px-4 py-2 rounded-lg bg-primary text-primary-foreground hover:bg-primary/90 transition-colors disabled:opacity-50"
                        >
                            {updateHealth.isPending ? 'Saving...' : 'Save'}
                        </button>
                    </div>
                </form>
            </MorphingDialogDescription>
        </>
    );
}

export function HealthCard({ healthCheck }: HealthCardProps) {
    const deleteHealth = useHealthDelete();
    const [confirmDelete, setConfirmDelete] = useState(false);

    const intervalLabel = healthCheck.interval === CheckInterval.Hourly ? 'Hourly' : 'Daily';

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
                    <MorphingDialog>
                        <MorphingDialogTrigger className="p-1.5 rounded-lg transition-colors hover:bg-muted text-muted-foreground hover:text-foreground">
                            <Tooltip>
                                <TooltipTrigger asChild>
                                    <Pencil className="size-4" />
                                </TooltipTrigger>
                                <TooltipContent>Edit</TooltipContent>
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
                            <TooltipContent>Delete</TooltipContent>
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
                                        onSuccess: () => toast.success('Health check deleted'),
                                    })
                                }
                                disabled={deleteHealth.isPending}
                                className="flex-1 h-7 flex items-center justify-center gap-2 rounded-lg bg-destructive-foreground text-destructive text-sm font-semibold transition-all hover:bg-destructive-foreground/90 active:scale-[0.98] disabled:opacity-50 disabled:cursor-not-allowed"
                            >
                                <Trash2 className="size-3.5" />
                                Confirm Delete
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
                    <span className="text-muted-foreground">Prompt</span>
                    <span className="font-mono truncate max-w-[60%]">{healthCheck.prompt}</span>
                </div>
                <div className="flex justify-between">
                    <span className="text-muted-foreground">Last Check</span>
                    <span>{formatDateTime(healthCheck.last_check)}</span>
                </div>
                <div className="flex justify-between">
                    <span className="text-muted-foreground">Next Check</span>
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
