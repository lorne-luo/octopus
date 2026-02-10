'use client';

import { useEffect, useMemo, useState, useRef } from 'react';
import { AnimatePresence, motion } from 'motion/react';
import { GroupCard } from './Card';
import { useGroupList } from '@/api/endpoints/group';
import { useSearchStore } from '@/components/modules/toolbar';
import { EASING } from '@/lib/animations/fluid-transitions';
import { Loader2 } from 'lucide-react';

const INITIAL_COUNT = 9;
const INCREMENT_COUNT = 9;

export function Group() {
    const { data: groups } = useGroupList();
    const pageKey = 'group' as const;
    const searchTerm = useSearchStore((s) => s.getSearchTerm(pageKey));

    const [displayCount, setDisplayCount] = useState(INITIAL_COUNT);
    const sentinelRef = useRef<HTMLDivElement>(null);

    const filteredGroups = useMemo(() => {
        if (!groups) return [];
        const sorted = [...groups].sort((a, b) => a.id! - b.id!);
        if (!searchTerm.trim()) return sorted;
        const term = searchTerm.toLowerCase();
        return sorted.filter((g) => g.name.toLowerCase().includes(term));
    }, [groups, searchTerm]);

    // Reset display count when search term changes
    useEffect(() => {
        setDisplayCount(INITIAL_COUNT);
    }, [searchTerm]);

    const displayGroups = useMemo(() => {
        return filteredGroups.slice(0, displayCount);
    }, [filteredGroups, displayCount]);

    const hasMore = displayCount < filteredGroups.length;

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
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
                <AnimatePresence mode="popLayout">
                    {displayGroups.map((group, index) => (
                        <motion.div
                            key={group.id}
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
                            <GroupCard group={group} />
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
