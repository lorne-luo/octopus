'use client';

import { useState, useCallback } from 'react';
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
import { Input } from '@/components/ui/input';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';

export function CreateDialogContent() {
    const { setIsOpen } = useMorphingDialog();
    const createHealth = useHealthCreate();
    const { data: channels } = useChannelList();

    const [channelId, setChannelId] = useState<string>('');
    const [modelName, setModelName] = useState('');
    const [interval, setInterval] = useState<CheckInterval>(CheckInterval.Hourly);
    const [prompt, setPrompt] = useState('hi');

    const handleSubmit = useCallback((e: React.FormEvent) => {
        e.preventDefault();

        if (!channelId) {
            toast.error('Please select a channel');
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
                toast.success('Health check created');
                setIsOpen(false);
            },
            onError: (error) => {
                toast.error('Failed to create health check', { description: error.message });
            },
        });
    }, [channelId, modelName, interval, prompt, createHealth, setIsOpen]);

    return (
        <div className="w-screen max-w-full md:max-w-lg min-h-0 flex flex-col">
            <MorphingDialogTitle className="shrink-0">
                <header className="mb-5 flex items-center justify-between">
                    <h2 className="text-2xl font-bold text-card-foreground">
                        New Health Check
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
                            <FieldLabel>Channel</FieldLabel>
                            <Select value={channelId} onValueChange={setChannelId}>
                                <SelectTrigger className="w-full">
                                    <SelectValue placeholder="Select a channel" />
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
                            <FieldLabel>Model Name</FieldLabel>
                            <Input
                                value={modelName}
                                onChange={(e) => setModelName(e.target.value)}
                                placeholder="e.g., gpt-4, claude-3-opus"
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
                            disabled={createHealth.isPending}
                            className="px-4 py-2 rounded-lg bg-primary text-primary-foreground hover:bg-primary/90 transition-colors disabled:opacity-50"
                        >
                            {createHealth.isPending ? 'Creating...' : 'Create'}
                        </button>
                    </div>
                </form>
            </MorphingDialogDescription>
        </div>
    );
}
