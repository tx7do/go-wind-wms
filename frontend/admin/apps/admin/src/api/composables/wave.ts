import type {
  waveservicev1_CancelWaveRequest,
  waveservicev1_CreateWaveRequest,
  waveservicev1_ListWaveResponse,
  waveservicev1_RunWaveRequest,
  waveservicev1_RunWaveResponse,
} from '#/api/generated/admin/service/v1';

import {
  useMutation,
  type UseMutationOptions,
  useQuery,
  type UseQueryOptions,
} from '@tanstack/vue-query';

import { apiClient } from '#/api/client';
import { queryClient } from '#/plugins/vue-query';
import { type PaginationQuery } from '#/transport/rest';

// ==============================
// 波次（Wave，M2b 出库合并执行）
// ==============================

export function useListWaves(
  query: PaginationQuery,
  options?: UseQueryOptions<waveservicev1_ListWaveResponse, Error>,
) {
  return useQuery({
    queryKey: ['listWaves', query],
    queryFn: () => apiClient.waveService.List(query.toRawParams()),
    ...options,
  });
}

export async function fetchListWaves(params: PaginationQuery) {
  return queryClient.fetchQuery({
    queryKey: ['listWaves', params],
    queryFn: () => apiClient.waveService.List(params.toRawParams()),
    staleTime: 0,
    retry: 0,
  });
}

export function useGetWave(
  id: number,
  options?: UseQueryOptions<any, Error>,
) {
  return useQuery({
    queryKey: ['getWave', id],
    queryFn: () => apiClient.waveService.Get({ id }),
    ...options,
  });
}

export function useCreateWave(
  options?: UseMutationOptions<object, Error, Record<string, any>>,
) {
  return useMutation({
    mutationFn: (values) =>
      apiClient.waveService.Create({
        ...values,
      } as waveservicev1_CreateWaveRequest),
    ...options,
  });
}

export function useRunWave(
  options?: UseMutationOptions<
    waveservicev1_RunWaveResponse,
    Error,
    waveservicev1_RunWaveRequest
  >,
) {
  return useMutation({
    mutationFn: (req) => apiClient.waveService.Run(req),
    ...options,
  });
}

export function useCancelWave(
  options?: UseMutationOptions<
    object,
    Error,
    waveservicev1_CancelWaveRequest
  >,
) {
  return useMutation({
    mutationFn: (req) => apiClient.waveService.Cancel(req),
    ...options,
  });
}
