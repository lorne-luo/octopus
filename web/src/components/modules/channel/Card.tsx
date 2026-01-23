import {
    MorphingDialog,
    MorphingDialogTrigger,
    MorphingDialogContainer,
    MorphingDialogContent,
} from '@/components/ui/morphing-dialog';
import { MessageSquare, Activity } from 'lucide-react';
import { type StatsMetricsFormatted } from '@/api/endpoints/stats';
import { type Channel, useEnableChannel } from '@/api/endpoints/channel';
import { type HealthCheck, HealthCheckStatus } from '@/api/endpoints/health';
import { CardContent } from './CardContent';
import { useTranslations } from 'next-intl';
import { Tooltip, TooltipTrigger, TooltipContent } from '@/components/animate-ui/components/animate/tooltip';
import { Switch } from '@/components/ui/switch';
import { toast } from '@/components/common/Toast';

export function Card({ channel, stats, health }: { channel: Channel; stats: StatsMetricsFormatted; health?: HealthCheck }) {
    const t = useTranslations('channel.card');
    const enableChannel = useEnableChannel();

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

    const getHealthIcon = () => {
        if (!health) {
            return <Activity className="h-5 w-5 text-gray-400" />;
        }
        switch (health.status) {
            case HealthCheckStatus.Healthy:
                return <Activity className="h-5 w-5 text-green-500" />;
            case HealthCheckStatus.Unhealthy:
                return <Activity className="h-5 w-5 text-red-500" />;
            case HealthCheckStatus.Checking:
                return <Activity className="h-5 w-5 text-yellow-500 animate-pulse" />;
            default:
                return <Activity className="h-5 w-5 text-gray-400" />;
        }
    };

    const getHealthText = () => {
        if (!health) return 'No health check';
        switch (health.status) {
            case HealthCheckStatus.Healthy:
                return 'Healthy';
            case HealthCheckStatus.Unhealthy:
                return 'Unhealthy';
            case HealthCheckStatus.Checking:
                return 'Checking...';
            default:
                return 'Unknown';
        }
    };

    const formatLatency = () => {
        if (!health || health.latency_ms === undefined) return '-';
        return `${health.latency_ms}ms`;
    };

    return (
        <MorphingDialog>
            <MorphingDialogTrigger className="w-full">
                <article className="relative flex h-54 flex-col justify-between gap-5 rounded-3xl border border-border bg-card text-card-foreground p-4 custom-shadow transition-all duration-300 hover:scale-[1.02]">
                    <header className="relative flex items-center justify-between gap-2">
                        <Tooltip side="top" sideOffset={10} align="center">
                            <TooltipTrigger asChild>
                                <h3 className="text-lg font-bold truncate min-w-0">{channel.name}</h3>
                            </TooltipTrigger>
                            <TooltipContent key={channel.name}>{channel.name}</TooltipContent>
                        </Tooltip>
                        <Switch
                            checked={channel.enabled}
                            onCheckedChange={handleEnableChange}
                            disabled={enableChannel.isPending}
                            onClick={(e) => e.stopPropagation()}
                        />
                    </header>

                    <dl className="relative grid grid-cols-1 gap-3">
                        <div className="flex items-center justify-between rounded-2xl border border-border/70 bg-background/80 p-2">
                            <div className="flex items-center gap-3">
                                <span className="flex h-10 w-10 items-center justify-center rounded-lg bg-primary/10 text-primary">
                                    <MessageSquare className="h-5 w-5" />
                                </span>
                                <dt className="text-sm text-muted-foreground">{t('requestCount')}</dt>
                            </div>
                            <dd className="text-base">
                                {stats.request_count.formatted.value}
                                <span className="ml-1 text-xs text-muted-foreground">{stats.request_count.formatted.unit}</span>
                            </dd>
                        </div>

                        <div className="flex items-center justify-between rounded-2xl border border-border/70 bg-background/80 p-2">
                            <div className="flex items-center gap-3">
                                <span className="flex h-10 w-10 items-center justify-center rounded-lg bg-primary/10 text-primary">
                                    {getHealthIcon()}
                                </span>
                                <dt className="text-sm text-muted-foreground">{getHealthText()}</dt>
                            </div>
                            <dd className="text-base">
                                {formatLatency()}
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
