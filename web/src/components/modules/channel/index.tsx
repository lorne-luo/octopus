'use client';

import { useEffect, useMemo, useState, useRef } from 'react';
import { AnimatePresence, motion } from 'motion/react';
import { useChannelListRegular } from '@/api/endpoints/channel';
import { Card } from './Card';
import { useSearchStore } from '@/components/modules/toolbar';
import { EASING } from '@/lib/animations/fluid-transitions';
import { useChannelName } from '@/hooks/use-channel-name';
import { Loader2 } from 'lucide-react';

const INITIAL_COUNT = 16;
const INCREMENT_COUNT = 16;

export function Channel() {
    const { data: channelsData } = useChannelListRegular();
    const { getChannelName } = useChannelName();
    const pageKey = 'channel' as const;
    const searchTerm = useSearchStore((s) => s.getSearchTerm(pageKey));

    const [displayCount, setDisplayCount] = useState(INITIAL_COUNT);
    const sentinelRef = useRef<HTMLDivElement>(null);

    const filteredChannels = useMemo(() => {
        if (!channelsData) return [];
        const sorted = [...channelsData].sort((a, b) => {
            const countA = a.raw.stats.request_success + a.raw.stats.request_failed;
            const countB = b.raw.stats.request_success + b.raw.stats.request_failed;
            if (countA !== countB) return countB - countA;
            return a.raw.id - b.raw.id;
        });
        if (!searchTerm.trim()) return sorted;
        const term = searchTerm.toLowerCase();
        return sorted.filter((c) => {
            const name = getChannelName(c.raw).toLowerCase();
            return name.includes(term);
        });
    }, [channelsData, searchTerm, getChannelName]);

    // Reset display count when search term changes
    useEffect(() => {
        setDisplayCount(INITIAL_COUNT);
    }, [searchTerm]);

    const displayChannels = useMemo(() => {
        return filteredChannels.slice(0, displayCount);
    }, [filteredChannels, displayCount]);

    const hasMore = displayCount < filteredChannels.length;

    useEffect(() => {
        if (!hasMore) return;

        const observer = new IntersectionObserver(
            (entries) => {
                if (entries[0].isIntersecting) {
                    setDisplayCount((prev) => prev + INCREMENT_COUNT);
                }
            },
            { rootMargin: '200px' }
        );

        if (sentinelRef.current) {
            observer.observe(sentinelRef.current);
        }

        return () => observer.disconnect();
    }, [hasMore]);

    return (
        <div className="space-y-4">
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">
                <AnimatePresence mode="popLayout">
                    {displayChannels.map((channel, index) => (
                        <motion.div
                            key={"channel-" + channel.raw.id}
                            initial={{ opacity: 0, y: 20 }}
                            animate={{ opacity: 1, y: 0 }}
                            exit={{
                                opacity: 0,
                                scale: 0.95,
                                transition: { duration: 0.2 }
                            }}
                            transition={{
                                duration: 0.45,
                                ease: EASING.easeOutExpo,
                                delay: Math.min(0.05 * (index % INITIAL_COUNT), 0.3),
                            }}
                            layout={!searchTerm.trim()}
                        >
                            <Card channel={channel.raw} stats={channel.formatted} />
                        </motion.div>
                    ))}
                </AnimatePresence>
            </div>

            {hasMore && (
                <div ref={sentinelRef} className="flex justify-center py-8">
                    <Loader2 className="size-6 text-muted-foreground animate-spin" />
                </div>
            )}
        </div>
    );
}
