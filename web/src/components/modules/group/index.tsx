'use client';

import { useEffect, useMemo, useRef, useState } from 'react';
import { AnimatePresence, motion } from 'motion/react';
import { GroupCard } from './Card';
import { useGroupList } from '@/api/endpoints/group';
import { useSearchStore } from '@/components/modules/toolbar';
import { EASING } from '@/lib/animations/fluid-transitions';

/** Group card approximate height (variable due to content) */
const INITIAL_VISIBLE_COUNT = 9;
const LOAD_BATCH_SIZE = 9;

export function Group() {
    const { data: groups } = useGroupList();
    const pageKey = 'group' as const;
    const searchTerm = useSearchStore((s) => s.getSearchTerm(pageKey));
    const [visibleCount, setVisibleCount] = useState(INITIAL_VISIBLE_COUNT);
    const observerRef = useRef<IntersectionObserver | null>(null);

    // Reset visible count when search term changes
    useEffect(() => {
        setVisibleCount(INITIAL_VISIBLE_COUNT);
    }, [searchTerm]);

    // Intersection Observer for lazy loading
    useEffect(() => {
        const observer = new IntersectionObserver(
            (entries) => {
                if (entries[0].isIntersecting) {
                    setVisibleCount((prev) => prev + LOAD_BATCH_SIZE);
                }
            },
            { rootMargin: '200px' }
        );

        observerRef.current = observer;

        return () => {
            observer.disconnect();
        };
    }, []);

    const filteredGroups = useMemo(() => {
        if (!groups) return [];
        const sorted = [...groups].sort((a, b) => a.id! - b.id!);
        if (!searchTerm.trim()) return sorted;
        const term = searchTerm.toLowerCase();
        return sorted.filter((g) => g.name.toLowerCase().includes(term));
    }, [groups, searchTerm]);

    // Attach/detach sentinel observer based on whether we should load more
    useEffect(() => {
        const sentinel = document.getElementById('group-sentinel');
        if (!sentinel || !observerRef.current) return;

        const shouldLoadMore = visibleCount < filteredGroups.length;

        if (shouldLoadMore) {
            observerRef.current.observe(sentinel);
        } else {
            observerRef.current.unobserve(sentinel);
        }

        return () => {
            if (sentinel) observerRef.current?.unobserve(sentinel);
        };
    }, [filteredGroups.length, visibleCount]);

    const visibleGroups = useMemo(() => {
        return filteredGroups.slice(0, visibleCount);
    }, [filteredGroups, visibleCount]);

    return (
        <motion.div
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
            transition={{ duration: 0.25, ease: EASING.easeOutExpo }}
        >
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
                <AnimatePresence mode="popLayout">
                    {visibleGroups.map((group, index) => (
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
                                delay: index === 0 ? 0 : Math.min(0.08 * Math.log2(index + 1), 0.4),
                            }}
                            layout={!searchTerm.trim()}
                        >
                            <GroupCard group={group} />
                        </motion.div>
                    ))}
                </AnimatePresence>
            </div>
            {/* Sentinel for lazy loading */}
            <div id="group-sentinel" className="h-4" />
        </motion.div>
    );
}
