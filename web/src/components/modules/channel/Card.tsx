import {
    MorphingDialog,
    MorphingDialogTrigger,
    MorphingDialogContainer,
    MorphingDialogContent,
} from '@/components/ui/morphing-dialog';
import { DollarSign, MessageSquare, Activity } from 'lucide-react';
import { type StatsMetricsFormatted } from '@/api/endpoints/stats';
import { type Channel, useEnableChannel } from '@/api/endpoints/channel';
import { useHealthByChannelId, HealthCheckStatus } from '@/api/endpoints/health';
import { CardContent } from './CardContent';
import { useTranslations } from 'next-intl';
import { Tooltip, TooltipTrigger, TooltipContent } from '@/components/animate-ui/components/animate/tooltip';
import { Switch } from '@/components/ui/switch';
import { Badge } from '@/components/ui/badge';
import { toast } from '@/components/common/Toast';

export function Card({ channel, stats }: { channel: Channel; stats: StatsMetricsFormatted }) {
    const t = useTranslations('channel.card');
    const enableChannel = useEnableChannel();
    const { data: healthChecks } = useHealthByChannelId(channel.id);

    const healthCheck = healthChecks && healthChecks.length > 0 ? healthChecks[0] : null;

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

    const getHealthStatusColor = (status: HealthCheckStatus) => {
        switch (status) {
            case HealthCheckStatus.Healthy:
                return 'bg-green-500/10 text-green-500 border-green-500/20';
            case HealthCheckStatus.Unhealthy:
                return 'bg-red-500/10 text-red-500 border-red-500/20';
            case HealthCheckStatus.Checking:
                return 'bg-yellow-500/10 text-yellow-500 border-yellow-500/20';
            default:
                return 'bg-gray-500/10 text-gray-500 border-gray-500/20';
        }
    };

    const getHealthStatusLabel = (status: HealthCheckStatus | undefined) => {
        if (!status || status === HealthCheckStatus.Unknown) {
            return t('healthStatusNone');
        }
        return t(`healthStatus${status.charAt(0).toUpperCase() + status.slice(1)}`);
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
                                    <DollarSign className="h-5 w-5" />
                                </span>
                                <dt className="text-sm text-muted-foreground">{t('totalCost')}</dt>
                            </div>
                            <dd className="text-base">
                                {stats.total_cost.formatted.value}
                                <span className="ml-1 text-xs text-muted-foreground">{stats.total_cost.formatted.unit}</span>
                            </dd>
                        </div>

                        {/* Health monitoring status */}
                        <div className="flex items-center justify-between rounded-2xl border border-border/70 bg-background/80 p-2">
                            <div className="flex items-center gap-3">
                                <span className={`flex h-10 w-10 items-center justify-center rounded-lg ${healthCheck ? getHealthStatusColor(healthCheck.status) : 'bg-muted/10 text-muted-foreground'}`}>
                                    <Activity className="h-5 w-5" />
                                </span>
                                <div className="flex flex-col">
                                    <dt className="text-sm text-muted-foreground">{t('healthStatus')}</dt>
                                    <dd className="text-xs text-muted-foreground/70">
                                        {healthCheck ? (
                                            <>
                                                {getHealthStatusLabel(healthCheck.status)}
                                                {healthCheck.latency_ms !== undefined && healthCheck.latency_ms !== null && (
                                                    <span className="ml-2">
                                                        • {healthCheck.latency_ms}ms
                                                    </span>
                                                )}
                                            </>
                                        ) : (
                                            t('healthStatusNone')
                                        )}
                                    </dd>
                                </div>
                            </div>
                            {healthCheck && (
                                <Badge variant="outline" className={`rounded-lg border ${getHealthStatusColor(healthCheck.status)}`}>
                                    {getHealthStatusLabel(healthCheck.status)}
                                    {healthCheck.latency_ms !== undefined && healthCheck.latency_ms !== null && (
                                        <span className="ml-1 opacity-70">• {healthCheck.latency_ms}ms</span>
                                    )}
                                </Badge>
                            )}
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
