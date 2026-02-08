"use client";

import { useEffect, useMemo, useRef, useState, useCallback } from "react";
import { AnimatePresence, motion } from "motion/react";
import { useChannelList } from "@/api/endpoints/channel";
import { Card } from "./Card";
import { useSearchStore } from "@/components/modules/toolbar";
import { EASING } from "@/lib/animations/fluid-transitions";

/** Channel card height: h-54 = 216px */
const INITIAL_VISIBLE_COUNT = 16;
const LOAD_BATCH_SIZE = 16;

export function Channel() {
  const { data: channelsData } = useChannelList();
  const pageKey = "channel" as const;
  const searchTerm = useSearchStore((s) => s.getSearchTerm(pageKey));
  const [visibleCount, setVisibleCount] = useState(INITIAL_VISIBLE_COUNT);
  const observerRef = useRef<IntersectionObserver | null>(null);
  const sentinelRef = useRef<HTMLDivElement>(null);

  // Reset visible count when search term changes
  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setVisibleCount(INITIAL_VISIBLE_COUNT);
  }, [searchTerm]);

  // Intersection Observer for lazy loading
  useEffect(() => {
    sentinelRef.current = document.createElement("div");

    const observer = new IntersectionObserver(
      (entries) => {
        if (entries[0].isIntersecting) {
          setVisibleCount((prev) => prev + LOAD_BATCH_SIZE);
        }
      },
      { rootMargin: "200px" },
    );

    observerRef.current = observer;

    // We'll attach the observer in the component's useEffect after render

    return () => {
      observer.disconnect();
    };
  }, []);

  const filteredChannels = useMemo(() => {
    if (!channelsData) return [];
    const sorted = [...channelsData].sort((a, b) => {
      const countA = a.raw.stats.request_success + a.raw.stats.request_failed;
      const countB = b.raw.stats.request_success + b.raw.stats.request_failed;
      if (countA === countB) {
        return a.raw.id - b.raw.id;
      }
      return countB - countA; // 总请求降序
    });
    if (!searchTerm.trim()) return sorted;
    const term = searchTerm.toLowerCase();
    return sorted.filter((c) => c.raw.name.toLowerCase().includes(term));
  }, [channelsData, searchTerm]);

  // Attach/detach sentinel observer based on whether we should load more
  useEffect(() => {
    const sentinel = document.getElementById("channel-sentinel");
    if (!sentinel || !observerRef.current) return;

    const shouldLoadMore = visibleCount < filteredChannels.length;

    if (shouldLoadMore) {
      observerRef.current.observe(sentinel);
    } else {
      observerRef.current.unobserve(sentinel);
    }

    return () => {
      if (sentinel) observerRef.current?.unobserve(sentinel);
    };
  }, [filteredChannels.length, visibleCount]);

  const visibleChannels = useMemo(() => {
    return filteredChannels.slice(0, visibleCount);
  }, [filteredChannels, visibleCount]);

  return (
    <motion.div
      initial={{ opacity: 0 }}
      animate={{ opacity: 1 }}
      exit={{ opacity: 0 }}
      transition={{ duration: 0.25, ease: EASING.easeOutExpo }}
    >
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">
        <AnimatePresence mode="popLayout">
          {visibleChannels.map((channel, index) => (
            <motion.div
              key={"channel-" + channel.raw.id}
              initial={{ opacity: 0, y: 20 }}
              animate={{ opacity: 1, y: 0 }}
              exit={{
                opacity: 0,
                scale: 0.95,
                transition: { duration: 0.2 },
              }}
              transition={{
                duration: 0.45,
                ease: EASING.easeOutExpo,
                delay:
                  index === 0 ? 0 : Math.min(0.08 * Math.log2(index + 1), 0.4),
              }}
              layout={!searchTerm.trim()}
            >
              <Card channel={channel.raw} stats={channel.formatted} />
            </motion.div>
          ))}
        </AnimatePresence>
      </div>
      {/* Sentinel for lazy loading */}
      <div id="channel-sentinel" className="h-4" />
    </motion.div>
  );
}
