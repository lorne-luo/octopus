import { describe, it, expect } from 'vitest';

describe('Group Editor - OAuth Provider Channel List', () => {
    it('should handle empty oauth provider channels', () => {
        const displayChannels: any[] = [];
        const byId = new Map<number, { id: number; name: string; models: any[] }>();
        
        displayChannels.forEach((mc) => {
            if (!mc || typeof mc.channel_id !== 'number' || !mc.name || !mc.channel_name) {
                return;
            }
            const existing = byId.get(mc.channel_id);
            if (existing) existing.models.push(mc);
            else byId.set(mc.channel_id, { id: mc.channel_id, name: mc.channel_name, models: [mc] });
        });

        const channels = Array.from(byId.values());
        expect(channels).toEqual([]);
    });

    it('should handle oauth provider channels with negative channel_id', () => {
        const displayChannels = [
            {
                name: 'gpt-4',
                enabled: true,
                channel_id: -1,
                channel_name: 'OAuth Provider 1',
            },
            {
                name: 'gpt-3.5-turbo',
                enabled: true,
                channel_id: -1,
                channel_name: 'OAuth Provider 1',
            },
        ];

        const byId = new Map<number, { id: number; name: string; models: any[] }>();
        
        displayChannels.forEach((mc) => {
            if (!mc || typeof mc.channel_id !== 'number' || !mc.name || !mc.channel_name) {
                return;
            }
            const existing = byId.get(mc.channel_id);
            if (existing) existing.models.push(mc);
            else byId.set(mc.channel_id, { id: mc.channel_id, name: mc.channel_name, models: [mc] });
        });

        const channels = Array.from(byId.values());
        expect(channels).toHaveLength(1);
        expect(channels[0].id).toBe(-1);
        expect(channels[0].models).toHaveLength(2);
    });

    it('should handle invalid channel data gracefully', () => {
        const displayChannels = [
            null,
            undefined,
            { name: 'gpt-4' }, // missing channel_id
            { channel_id: 1 }, // missing name
            {
                name: 'gpt-4',
                enabled: true,
                channel_id: 1,
                channel_name: 'Valid Channel',
            },
        ] as any[];

        const byId = new Map<number, { id: number; name: string; models: any[] }>();
        
        displayChannels.forEach((mc) => {
            if (!mc || typeof mc.channel_id !== 'number' || !mc.name || !mc.channel_name) {
                return;
            }
            const existing = byId.get(mc.channel_id);
            if (existing) existing.models.push(mc);
            else byId.set(mc.channel_id, { id: mc.channel_id, name: mc.channel_name, models: [mc] });
        });

        const channels = Array.from(byId.values());
        expect(channels).toHaveLength(1);
        expect(channels[0].name).toBe('Valid Channel');
    });
});
