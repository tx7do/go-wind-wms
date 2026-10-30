import 'package:dio/dio.dart' show DioException;
import 'package:get_it/get_it.dart' show GetIt;

import 'package:go_wind_wms/generated/api/app/service/v1/index.dart'
    show
        ApiClient,
        AsnServiceV1AsnStatus,
        InventoryServiceV1ConfirmStockPickingRequest,
        InventoryServiceV1GetStockPickingByNumberRequest,
        InventoryServiceV1GetStockPickingRequest,
        InventoryServiceV1ValidateStockPickingRequest,
        PaginationPagingRequest;
import 'package:go_wind_wms/src/core/transport/http/api_exception.dart'
    show ApiException, ApiExceptionCategory;

/// 出库扫码（M2b PDA）的数据访问：查单 → 确认 → 校验落账。
///
/// 精简分层：页内 StatefulWidget 直接持有本类（场景单一，无列表/跨页状态）；
/// 失败映射与 asn/wms 仓储同口径。
class OutboundRepository {
  final ApiClient _api;

  OutboundRepository(this._api);

  /// 按拣货单号查单（含 moves）；未命中抛 [StateError]（页面按未找到提示）。
  Future<dynamic> getByNumber(String number) async {
    try {
      return await _api.stockPickingService.getStockPickingByNumber(
        InventoryServiceV1GetStockPickingByNumberRequest(number: number),
      );
    } on DioException catch (e) {
      throw _toMessage(e);
    }
  }

  /// 确认拣货单（DRAFT moves → CONFIRMED）。
  Future<void> confirm(int id) async {
    try {
      await _api.stockPickingService.confirm(
        InventoryServiceV1ConfirmStockPickingRequest(id: id),
      );
    } on DioException catch (e) {
      throw _toMessage(e);
    }
  }

  /// 校验拣货单（事务内 FEFO 扣减落账；CONFIRMED moves → DONE）。
  Future<void> validate(int id) async {
    try {
      await _api.stockPickingService.validate(
        InventoryServiceV1ValidateStockPickingRequest(id: id),
      );
    } on DioException catch (e) {
      throw _toMessage(e);
    }
  }

  /// 拉取近期出库单（OUTGOING，供列表选择）。
  Future<List<dynamic>> listOutbound({int limit = 20}) async {
    try {
      final resp = await _api.stockPickingService.list(
        PaginationPagingRequest(pageSize: limit),
      );
      return (resp.items ?? const [])
          .where((p) => p.pickingType?.value == 'OUTGOING')
          .toList(growable: false);
    } on DioException catch (e) {
      throw _toMessage(e);
    }
  }

  String _toMessage(DioException e) {
    final api = ApiException.fromDioError(e);
    switch (api.category) {
      case ApiExceptionCategory.auth:
        return 'outboundUnauthorized';
      case ApiExceptionCategory.business:
        if (e.response?.statusCode == 404) return 'outboundNotFound';
        return 'outboundRejected';
      case ApiExceptionCategory.server:
      case ApiExceptionCategory.network:
        return 'outboundNetwork';
      case ApiExceptionCategory.unknown:
        return 'outboundUnknown';
    }
  }
}

/// 供 [init.dart]（如需 DI）注册时构造。
OutboundRepository createOutboundRepository() {
  return OutboundRepository(GetIt.instance<ApiClient>());
}
