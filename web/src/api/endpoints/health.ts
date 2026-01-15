import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { apiClient } from '../client';

/**
 * Health check status enum
 */
export enum HealthCheckStatus {
    Unknown = 'unknown',
    Healthy = 'healthy',
    Unhealthy = 'unhealthy',
    Checking = 'checking',
}

/**
 * Check interval enum
 */
export enum CheckInterval {
    Minutely = 'minutely',
    Hourly = 'hourly',
    Daily = 'daily',
}

/**
 * Health check data structure
 */
export type HealthCheck = {
    id: number;
    channel_id: number;
    model_name: string;
    interval: CheckInterval;
    prompt: string;
    status: HealthCheckStatus;
    last_check?: string;
    next_check?: string;
    last_error?: string;
    created_at: string;
    updated_at: string;
};

/**
 * Health check with channel name for display
 */
export type HealthCheckWithChannel = HealthCheck & {
    channel_name: string;
};

/**
 * Create health check request
 */
export type CreateHealthCheckRequest = {
    channel_id: number;
    model_name: string;
    interval: CheckInterval;
    prompt: string;
};

/**
 * Update health check request
 */
export type UpdateHealthCheckRequest = {
    id: number;
    model_name?: string;
    interval?: CheckInterval;
    prompt?: string;
};

/**
 * Fetch all health checks
 */
export const useHealthList = () => {
    return useQuery({
        queryKey: ['health', 'list'],
        queryFn: async () => {
            // apiClient already unwraps the ApiResponse, so we get the data directly
            const response = await apiClient.get<HealthCheckWithChannel[]>('/api/v1/health/list');
            return response || [];
        },
    });
};

/**
 * Create a new health check
 */
export const useHealthCreate = () => {
    const queryClient = useQueryClient();
    return useMutation({
        mutationFn: async (data: CreateHealthCheckRequest) => {
            // apiClient already unwraps the ApiResponse
            return await apiClient.post<HealthCheck>('/api/v1/health/create', data);
        },
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: ['health', 'list'] });
        },
    });
};

/**
 * Update an existing health check
 */
export const useHealthUpdate = () => {
    const queryClient = useQueryClient();
    return useMutation({
        mutationFn: async (data: UpdateHealthCheckRequest) => {
            // apiClient already unwraps the ApiResponse
            return await apiClient.post<HealthCheck>('/api/v1/health/update', data);
        },
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: ['health', 'list'] });
        },
    });
};

/**
 * Delete a health check
 */
export const useHealthDelete = () => {
    const queryClient = useQueryClient();
    return useMutation({
        mutationFn: async (id: number) => {
            // apiClient already unwraps the ApiResponse
            return await apiClient.delete<null>(`/api/v1/health/delete/${id}`);
        },
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: ['health', 'list'] });
        },
    });
};

/**
 * Test a health check immediately
 */
export const useHealthTest = () => {
    const queryClient = useQueryClient();
    return useMutation({
        mutationFn: async (id: number) => {
            // apiClient already unwraps the ApiResponse
            // Send empty object to satisfy RequireJSON middleware
            return await apiClient.post<null>(`/api/v1/health/test/${id}`, {});
        },
        onSuccess: () => {
            // Invalidate after a short delay to allow the test to complete
            setTimeout(() => {
                queryClient.invalidateQueries({ queryKey: ['health', 'list'] });
            }, 1000);
        },
    });
};

/**
 * API response wrapper type (kept for reference, but apiClient auto-unwraps)
 */
export type ApiResponse<T> = {
    success: boolean;
    message?: string;
    data?: T;
};
