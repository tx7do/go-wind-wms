import type {
  asnservicev1_CloseAsnRequest,
  asnservicev1_CreateAsnRequest,
  asnservicev1_DeleteAsnRequest,
  asnservicev1_ListAsnResponse,
  asnservicev1_PutawayAsnRequest,
  asnservicev1_ReceiveAsnRequest,
  asnservicev1_UpdateAsnRequest,
} from '#/api/generated/admin/service/v1';

import {
  useMutation,
  type UseMutationOptions,
  useQuery,
  type UseQueryOptions,
} from '@tanstack/vue-query';

import { apiClient } from '#/api/client';
import { queryClient } from '#/plugins/vue-query';
import { makeUpdateMask, type PaginationQuery } from '#/transport/rest';

// ==============================
// 到货通知单（ASN）
// ==============================

export function useListAsns(
  query: PaginationQuery,
  options?: UseQueryOptions<asnservicev1_ListAsnResponse, Error>,
) {
  return useQuery({
    queryKey: ['listAsns', query],
    queryFn: () => apiClient.asnService.List(query.toRawParams()),
    ...options,
  });
}

export async function fetchListAsns(params: PaginationQuery) {
  return queryClient.fetchQuery({
    queryKey: ['listAsns', params],
    queryFn: () => apiClient.asnService.List(params.toRawParams()),
    staleTime: 0,
    retry: 0,
  });
}

export function useGetAsn(
  id: number,
  options?: UseQueryOptions<any, Error>,
) {
  return useQuery({
    queryKey: ['getAsn', id],
    queryFn: () => apiClient.asnService.Get({ id }),
    ...options,
  });
}

export function useCreateAsn(
  options?: UseMutationOptions<object, Error, Record<string, any>>,
) {
  return useMutation({
    mutationFn: (values) =>
      apiClient.asnService.Create({
        data: { ...values },
      } as asnservicev1_CreateAsnRequest),
    ...options,
  });
}

export function useUpdateAsn(
  options?: UseMutationOptions<
    object,
    Error,
    { id: number; values: Record<string, any> }
  >,
) {
  return useMutation({
    mutationFn: ({ id, values }: { id: number; values: Record<string, any> }) =>
      apiClient.asnService.Update({
        id,
        data: { ...values },
        updateMask: makeUpdateMask(Object.keys(values ?? {})),
      } as asnservicev1_UpdateAsnRequest),
    ...options,
  });
}

export function useDeleteAsn(
  options?: UseMutationOptions<
    object,
    Error,
    asnservicev1_DeleteAsnRequest
  >,
) {
  return useMutation({
    mutationFn: (req) => apiClient.asnService.Delete(req),
    ...options,
  });
}

export function useAnnounceAsn(
  options?: UseMutationOptions<object, Error, { id: number }>,
) {
  return useMutation({
    mutationFn: (req) => apiClient.asnService.Announce(req),
    ...options,
  });
}

export function useCancelAsn(
  options?: UseMutationOptions<object, Error, { id: number }>,
) {
  return useMutation({
    mutationFn: (req) => apiClient.asnService.Cancel(req),
    ...options,
  });
}

export function useCloseAsn(
  options?: UseMutationOptions<
    object,
    Error,
    asnservicev1_CloseAsnRequest
  >,
) {
  return useMutation({
    mutationFn: (req) => apiClient.asnService.Close(req),
    ...options,
  });
}

export function useReceiveAsn(
  options?: UseMutationOptions<
    object,
    Error,
    asnservicev1_ReceiveAsnRequest
  >,
) {
  return useMutation({
    mutationFn: (req) => apiClient.asnService.Receive(req),
    ...options,
  });
}

export function usePutawayAsn(
  options?: UseMutationOptions<
    object,
    Error,
    asnservicev1_PutawayAsnRequest
  >,
) {
  return useMutation({
    mutationFn: (req) => apiClient.asnService.Putaway(req),
    ...options,
  });
}
