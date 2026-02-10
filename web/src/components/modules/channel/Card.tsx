import {
    MorphingDialog,
    MorphingDialogTrigger,
    MorphingDialogContainer,
    MorphingDialogContent,
} from '@/components/ui/morphing-dialog';
import { Cpu, MessageSquare, FileText } from 'lucide-react';
import { type StatsMetricsFormatted } from '@/api/endpoints/stats';
import { type Channel, useEnableChannel } from '@/api/endpoints/channel';
import { CardContent } from './CardContent';
import { useTranslations } from 'next-intl';
import { Tooltip, TooltipTrigger, TooltipContent } from '@/components/animate-ui/components/animate/tooltip';
import { Switch } from '@/components/ui/switch';
import { toast } from '@/components/common/Toast';
import { useChannelName } from '@/hooks/use-channel-name';

export function Card({ channel, stats }: { channel: Channel; stats: StatsMetricsFormatted }) {
    const t = useTranslations('channel.card');
    const enableChannel = useEnableChannel();
    const { getChannelName } = useChannelName();

    const channelName = getChannelName(channel);

    const handleEnableChange = (checked: boolean) => {
        enableChannel.mutate(
            { id: channel.id, enabled: checked },
            {
                onSuccess: () => {
                    toast.success(checked ? t('toast.enabled') : t('toast.disabled'));
                },
                onError: (error) => {
                    toast.error(error.message);
                },
            }
        );
    };

    return (
        <MorphingDialog>
            <MorphingDialogTrigger className="w-full">
                <article className="relative flex h-54 flex-col justify-between gap-5 rounded-3xl border border-border bg-card text-card-foreground p-4 custom-shadow transition-all duration-300 hover:scale-[1.02]">
                    <header className="relative flex items-center justify-between gap-2">
                        <Tooltip side="top" sideOffset={10} align="center">
                            <TooltipTrigger asChild>
                                <h3 className="text-lg font-bold truncate min-w-0">{channelName}</h3>
                            </TooltipTrigger>
                            <TooltipContent key={channelName}>{channelName}</TooltipContent>
                        </Tooltip>
                        <Switch
                            checked={channel.enabled}
                            onCheckedChange={handleEnableChange}
                            disabled={enableChannel.isPending}
                            onClick={(e) => e.stopPropagation()}
                        />
                    </header>

                    <dl className="relative grid grid-cols-1 gap-2">
                        <div className="flex items-center justify-between rounded-2xl border border-border/70 bg-background/80 p-1.5">
                            <div className="flex items-center gap-3">
                                <span className="flex h-9 w-9 items-center justify-center rounded-lg bg-primary/10 text-primary">
                                    <MessageSquare className="h-4.5 w-4.5" />
                                </span>
                                <dt className="text-sm text-muted-foreground">{t('requestCount')}</dt>
                            </div>
                            <dd className="text-sm font-medium pr-1">
                                {stats.request_count.formatted.value}
                                <span className="ml-1 text-xs text-muted-foreground">{stats.request_count.formatted.unit}</span>
                            </dd>
                        </div>

                        <div className="flex items-center justify-between rounded-2xl border border-border/70 bg-background/80 p-1.5">
                            <div className="flex items-center gap-3">
                                <span className="flex h-9 w-9 items-center justify-center rounded-lg bg-primary/10 text-primary">
                                    <Cpu className="h-4.5 w-4.5" />
                                </span>
                                <dt className="text-sm text-muted-foreground">{t('totalTokens')}</dt>
                            </div>
                            <dd className="text-sm font-medium pr-1">
                                {stats.total_token.formatted.value}
                                <span className="ml-1 text-xs text-muted-foreground">{stats.total_token.formatted.unit}</span>
                            </dd>
                        </div>
                    </dl>
                </article>
            </MorphingDialogTrigger>

            <MorphingDialogContainer>
                <MorphingDialogContent className="w-full md:max-w-xl bg-card text-card-foreground px-4 py-2 custom-shadow rounded-3xl max-h-[90vh] overflow-y-auto">
                    <CardContent channel={channel} stats={stats} />
                </MorphingDialogContent>
            </MorphingDialogContainer>
        </MorphingDialog>
    );
}
