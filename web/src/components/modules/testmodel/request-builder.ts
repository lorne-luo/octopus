import { API_BASE_URL } from '@/api/client';
import { useAuthStore } from '@/api/endpoints/user';

export type TestType = 'text_chat' | 'vision_chat' | 'tool_chat';

// =========================================================
// Request config
// =========================================================

export interface TestConfig {
    channelId: number;
    keyIndex: number;
    baseUrlIndex: number;
    model: string;
    testType: TestType;
}

export interface BuiltRequest {
    url: string;
    headers: Record<string, string>;
    body: string;
}

// =========================================================
// Main builder
// =========================================================

export function buildRequest(config: TestConfig): BuiltRequest {
    const { channelId, keyIndex, baseUrlIndex, model, testType } = config;

    // Resolve the base API URL
    const baseApiUrl = (API_BASE_URL && API_BASE_URL !== '.')
        ? API_BASE_URL.replace(/\/$/, '')
        : (typeof window !== 'undefined' ? window.location.origin : '');

    // Get JWT token from auth store
    const token = useAuthStore.getState().token ?? '';

    return {
        url: `${baseApiUrl}/api/v1/channel/test`,
        headers: {
            'Content-Type': 'application/json',
            Authorization: `Bearer ${token}`,
        },
        body: JSON.stringify({
            channel_id: channelId,
            key_index: keyIndex,
            base_url_index: baseUrlIndex,
            model,
            test_type: testType,
        }, null, 2),
    };
}
