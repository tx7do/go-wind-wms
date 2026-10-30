import 'package:dio/dio.dart' show DioException;
import 'package:get_it/get_it.dart' show GetIt;

import 'package:go_wind_wms/generated/api/app/service/v1/index.dart'
    show
        ApiClient,
        AsnServiceV1AsnQcStatus,
        AsnServiceV1GetAsnByNumberRequest,
        AsnServiceV1GetAsnRequest,
        PaginationPagingRequest,
        AsnServiceV1PutawayAsnLine,
        AsnServiceV1PutawayAsnRequest,
        AsnServiceV1ReceiveAsnLine,
        AsnServiceV1ReceiveAsnRequest;
import 'package:go_wind_wms/src/core/transport/http/api_exception.dart'
    show ApiException, ApiExceptionCategory;
import 'package:go_wind_wms/src/features/asn/domain/asn_failure.dart';
import 'package:go_wind_wms/src/features/asn/domain/asn_models.dart';
import 'package:go_wind_wms/src/features/asn/domain/asn_repository.dart';

/// [AsnRepository] 的 data 层实现。
///
/// 职责：调用生成的 `AsnServiceClient` / `LocationServiceClient`，将响应
/// 映射为领域模型；将传输层 [DioException]（已由统一拦截器封装为
/// [ApiException]）映射为 [AsnFailure] 子类抛出。
class AsnRepositoryImpl implements AsnRepository {
  final ApiClient _api;

  AsnRepositoryImpl(this._api);

  @override
  Future<List<AsnInfo>> listAsns({int limit = 20}) async {
    try {
      final resp = await _api.asnService.list(
        PaginationPagingRequest(pageSize: limit),
      );
      return (resp.items ?? const [])
          .map(AsnInfo.fromDto)
          .toList(growable: false);
    } on DioException catch (e) {
      throw _toFailure(e);
    }
  }

  @override
  Future<AsnInfo> getByNumber(String number) async {
    try {
      final dto = await _api.asnService.getByNumber(
        AsnServiceV1GetAsnByNumberRequest(number: number),
      );
      return AsnInfo.fromDto(dto);
    } on DioException catch (e) {
      throw _toFailure(e);
    }
  }

  @override
  Future<AsnInfo> getById(int id) async {
    try {
      final dto = await _api.asnService.get(
        AsnServiceV1GetAsnRequest(id: id),
      );
      return AsnInfo.fromDto(dto);
    } on DioException catch (e) {
      throw _toFailure(e);
    }
  }

  @override
  Future<int> receive(int asnId, List<ReceiveLineDraft> lines) async {
    try {
      final resp = await _api.asnService.receive(
        AsnServiceV1ReceiveAsnRequest(
          asnId: asnId,
          lines: lines
              .map(
                (l) => AsnServiceV1ReceiveAsnLine(
                  asnLineId: l.asnLineId,
                  quantity: l.quantity,
                  qcStatus: l.qcStatus,
                  lotName: l.lotName,
                  expiryDate:
                      l.expiry == null ? null : l.expiry!.toIso8601String(),
                ),
              )
              .toList(),
        ),
      );
      return resp.pickingId ?? 0;
    } on DioException catch (e) {
      throw _toFailure(e);
    }
  }

  @override
  Future<int> putaway(int asnId, List<PutawayLineDraft> lines) async {
    try {
      final resp = await _api.asnService.putaway(
        AsnServiceV1PutawayAsnRequest(
          asnId: asnId,
          lines: lines
              .map(
                (l) => AsnServiceV1PutawayAsnLine(
                  asnLineId: l.asnLineId,
                  quantity: l.quantity,
                  targetLocationId: l.targetLocationId,
                ),
              )
              .toList(),
        ),
      );
      return resp.pickingId ?? 0;
    } on DioException catch (e) {
      throw _toFailure(e);
    }
  }

  @override
  Future<List<LocationInfo>> listLocations({int limit = 500}) async {
    try {
      final resp = await _api.locationService.list(
        PaginationPagingRequest(pageSize: limit),
      );
      return (resp.items ?? const [])
          .map(LocationInfo.fromDto)
          .toList(growable: false);
    } on DioException catch (e) {
      throw _toFailure(e);
    }
  }

  /// 将传输层 [DioException]（经统一拦截器封装）映射为领域 [AsnFailure]。
  AsnFailure _toFailure(DioException e) {
    final api = ApiException.fromDioError(e);
    switch (api.category) {
      case ApiExceptionCategory.auth:
        return const AsnUnauthorizedFailure();
      case ApiExceptionCategory.business:
        // 404（单号不存在）与 409/400（防超收/防超上架/状态机）同属业务类，
        // 以 HTTP 状态码细分，便于展示层给出准确提示。
        final status = e.response?.statusCode;
        if (status == 404) {
          return const AsnNotFoundFailure();
        }
        return const AsnInvalidInputFailure();
      case ApiExceptionCategory.server:
      case ApiExceptionCategory.network:
        return const AsnNetworkFailure();
      case ApiExceptionCategory.unknown:
        return const AsnUnknownFailure();
    }
  }
}

/// 供 [init.dart] 注册时构造 [AsnRepositoryImpl]。
AsnRepositoryImpl createAsnRepositoryImpl() {
  return AsnRepositoryImpl(GetIt.instance<ApiClient>());
}
