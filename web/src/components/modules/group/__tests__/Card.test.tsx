import { describe, it, expect } from 'vitest';
import { buildChannelNameByModelKey, modelChannelKey } from '../utils';

describe('Group Card - Channel Name Mapping', () => {
    it('should build channel name map for regular channels', () => {
        const channels = [
            { channel_id: 1, name: 'gpt-4', channel_name: 'OpenAI Channel', enabled: true },
            { channel_id: 1, name: 'gpt-3.5-turbo', channel_name: 'OpenAI Channel', enabled: true },
            { channel_id: 2, name: 'claude-3', channel_name: 'Anthropic Channel', enabled: true },
        ];

        const map = buildChannelNameByModelKey(channels);

        expect(map.get(modelChannelKey(1, 'gpt-4'))).toBe('OpenAI Channel');
        expect(map.get(modelChannelKey(1, 'gpt-3.5-turbo'))).toBe('OpenAI Channel');
        expect(map.get(modelChannelKey(2, 'claude-3'))).toBe('Anthropic Channel');
    });

    it('should build channel name map for OAuth provider channels with negative IDs', () => {
        const channels = [
            { channel_id: -1, name: 'gpt-4', channel_name: 'OAuth Provider 1', enabled: true },
            { channel_id: -2, name: 'gpt-3.5-turbo', channel_name: 'OAuth Provider 2', enabled: true },
        ];

        const map = buildChannelNameByModelKey(channels);

        expect(map.get(modelChannelKey(-1, 'gpt-4'))).toBe('OAuth Provider 1');
        expect(map.get(modelChannelKey(-2, 'gpt-3.5-turbo'))).toBe('OAuth Provider 2');
    });

    it('should build channel name map for mixed regular and OAuth channels', () => {
        const channels = [
            { channel_id: 1, name: 'gpt-4', channel_name: 'Regular Channel', enabled: true },
            { channel_id: -1, name: 'gpt-4', channel_name: 'OAuth Provider', enabled: true },
            { channel_id: 2, name: 'claude-3', channel_name: 'Another Channel', enabled: true },
        ];

        const map = buildChannelNameByModelKey(channels);

        expect(map.get(modelChannelKey(1, 'gpt-4'))).toBe('Regular Channel');
        expect(map.get(modelChannelKey(-1, 'gpt-4'))).toBe('OAuth Provider');
        expect(map.get(modelChannelKey(2, 'claude-3'))).toBe('Another Channel');
    });

    it('should handle empty channel list', () => {
        const channels: any[] = [];
        const map = buildChannelNameByModelKey(channels);
        expect(map.size).toBe(0);
    });

    it('should use fallback channel name when not found in map', () => {
        const channels = [
            { channel_id: 1, name: 'gpt-4', channel_name: 'OpenAI Channel', enabled: true },
        ];

        const map = buildChannelNameByModelKey(channels);

        // Test fallback for non-existent channel
        const fallback = map.get(modelChannelKey(-2, 'unknown-model')) ?? 'Channel -2';
        expect(fallback).toBe('Channel -2');
    });
});
