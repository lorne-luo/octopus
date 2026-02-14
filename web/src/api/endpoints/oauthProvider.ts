import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { apiClient } from '../client';
import { logger } from '@/lib/logger';

export type OAuthProvider = {
    id: number;
    name: string;
    provider_type: string;
    api_key: string;
    status: number;
    last_refresh_at: number;
    refresh_fail_count: number;
    created_at: number;
    updated_at: number;
    model: string;
    custom_model: string;
};

export type CreateOAuthProviderRequest = {
    name: string;
    provider_type: string;
    cookie: string;
    api_key?: string;
    status?: number;
    model?: string;
    custom_model?: string;
};

export type UpdateOAuthProviderRequest = {
    id: number;
    name?: string;
    provider_type?: string;
    cookie?: string;
    status?: number;
    model?: string;
    custom_model?: string;
};

export function useOAuthProviderList() {
    return useQuery({
        queryKey: ['oauth-provider', 'list'],
        queryFn: async () => {
            return apiClient.get<OAuthProvider[]>('/api/v1/oauth-provider/list');
        },
    });
}

export function useCreateOAuthProvider() {
    const queryClient = useQueryClient();

    return useMutation({
        mutationFn: async (data: CreateOAuthProviderRequest) => {
            return apiClient.post<OAuthProvider>('/api/v1/oauth-provider/create', data);
        },
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: ['oauth-provider', 'list'] });
        },
        onError: (error) => {
            logger.error('Failed to create OAuth provider:', error);
        },
    });
}

export function useUpdateOAuthProvider() {
    const queryClient = useQueryClient();

    return useMutation({
        mutationFn: async (data: UpdateOAuthProviderRequest) => {
            return apiClient.post<OAuthProvider>('/api/v1/oauth-provider/update', data);
        },
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: ['oauth-provider', 'list'] });
        },
        onError: (error) => {
            logger.error('Failed to update OAuth provider:', error);
        },
    });
}

export function useDeleteOAuthProvider() {
    const queryClient = useQueryClient();

    return useMutation({
        mutationFn: async (id: number) => {
            return apiClient.delete<null>(`/api/v1/oauth-provider/delete/${id}`);
        },
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: ['oauth-provider', 'list'] });
        },
        onError: (error) => {
            logger.error('Failed to delete OAuth provider:', error);
        },
    });
}

export function useRefreshOAuthProvider() {
    const queryClient = useQueryClient();

    return useMutation({
        mutationFn: async (id: number) => {
            return apiClient.post<OAuthProvider>('/api/v1/oauth-provider/refresh', { id });
        },
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: ['oauth-provider', 'list'] });
        },
        onError: (error) => {
            logger.error('Failed to refresh OAuth provider:', error);
        },
    });
}

export function useFetchOAuthModel() {
    return useMutation({
        mutationFn: async (id: number) => {
            return apiClient.post<string[]>('/api/v1/oauth-provider/fetch-model', { id });
        },
        onSuccess: (data) => {
            logger.log('OAuth models fetched:', data);
        },
        onError: (error) => {
            logger.error('Failed to fetch OAuth models:', error);
        },
    });
}
