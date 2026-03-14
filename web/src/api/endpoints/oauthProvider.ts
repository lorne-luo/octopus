import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiClient } from "../client";
import { logger } from "@/lib/logger";
import { toast } from "@/components/common/Toast";

export type OAuthProviderChannel = {
  id: number;
  model: string;
  custom_model: string;
  match_regex?: string;
  enabled: boolean;
};

export type AuthJson = {
  id: number;
  oauth_provider_id: number;
  content: string;
  enabled: boolean;
  status_code: number;
  last_use_time_stamp: number;
  total_token: number;
  remark: string;
};

export type OAuthProvider = {
  id: number;
  name: string;
  provider_type: string;
  auth_jsons?: AuthJson[];
  api_key: string;
  status: number;
  last_refresh_at: number;
  refresh_fail_count: number;
  created_at: number;
  updated_at: number;
  base_url?: string;
  channel?: OAuthProviderChannel;
};

export type AuthJsonAddRequest = {
  enabled: boolean;
  content: string;
  remark?: string;
};

export type AuthJsonUpdateRequest = {
  id: number;
  enabled?: boolean;
  content?: string;
  remark?: string;
};

export type CreateOAuthProviderRequest = {
  name: string;
  provider_type: string;
  api_key?: string;
  status?: number;
  model?: string;
  custom_model?: string;
  match_regex?: string;
  auth_jsons?: AuthJsonAddRequest[];
};

export type UpdateOAuthProviderRequest = {
  id: number;
  name?: string;
  provider_type?: string;
  status?: number;
  model?: string;
  custom_model?: string;
  match_regex?: string;
  base_url?: string;
  auth_jsons_to_add?: AuthJsonAddRequest[];
  auth_jsons_to_update?: AuthJsonUpdateRequest[];
  auth_jsons_to_delete?: number[];
};

export function useOAuthProviderList() {
  return useQuery({
    queryKey: ["oauth-provider", "list"],
    queryFn: async () => {
      return apiClient.get<OAuthProvider[]>("/api/v1/oauth-provider/list");
    },
  });
}

export function useCreateOAuthProvider() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (data: CreateOAuthProviderRequest) => {
      return apiClient.post<OAuthProvider>(
        "/api/v1/oauth-provider/create",
        data,
      );
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["oauth-provider", "list"] });
      toast.success("OAuth provider created successfully");
    },
    onError: (error: any) => {
      logger.error("Failed to create OAuth provider:", error);
      const errorMessage = error?.message || "Failed to create OAuth provider";
      toast.error(errorMessage);
    },
  });
}

export function useUpdateOAuthProvider() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (data: UpdateOAuthProviderRequest) => {
      return apiClient.post<OAuthProvider>(
        "/api/v1/oauth-provider/update",
        data,
      );
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["oauth-provider", "list"] });
      toast.success("OAuth provider updated successfully");
    },
    onError: (error: any) => {
      logger.error("Failed to update OAuth provider:", error);
      const errorMessage = error?.message || "Failed to update OAuth provider";
      toast.error(errorMessage);
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
      queryClient.invalidateQueries({ queryKey: ["oauth-provider", "list"] });
      toast.success("OAuth provider deleted successfully");
    },
    onError: (error: any) => {
      logger.error("Failed to delete OAuth provider:", error);
      const errorMessage = error?.message || "Failed to delete OAuth provider";
      toast.error(errorMessage);
    },
  });
}

export function useRefreshOAuthProvider() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (id: number) => {
      return apiClient.post<OAuthProvider>("/api/v1/oauth-provider/refresh", {
        id,
      });
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["oauth-provider", "list"] });
      toast.success("OAuth provider refreshed successfully");
    },
    onError: (error: any) => {
      logger.error("Failed to refresh OAuth provider:", error);
      const errorMessage = error?.message || "Failed to refresh OAuth provider";
      toast.error(errorMessage);
    },
  });
}

export function useFetchOAuthModel() {
  return useMutation({
    mutationFn: async (id: number) => {
      return apiClient.post<string[]>("/api/v1/oauth-provider/fetch-model", {
        id,
      });
    },
    onSuccess: (data) => {
      logger.log("OAuth models fetched:", data);
    },
    onError: (error) => {
      logger.error("Failed to fetch OAuth models:", error);
    },
  });
}
