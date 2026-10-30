import 'package:go_wind_wms/generated/api/app/service/v1/index.dart'
    show
        AsnServiceV1Asn,
        AsnServiceV1AsnLine,
        AsnServiceV1AsnStatus,
        AsnServiceV1AsnQcStatus,
        InventoryServiceV1StockLocation;

/// 到货通知单领域模型。
///
/// 纯 Dart 值对象；由 data 层从 `/app/v1/asns` 响应映射而来。
/// 数量口径：expected（预期）/ received（已收）/ putaway（已上架）。
class AsnInfo {
  final int id;
  final String asnNumber;
  final String warehouseCode;
  final String partnerCode;
  final AsnServiceV1AsnStatus? status;
  final String? expectedDate;
  final String? remark;
  final List<AsnLineInfo> lines;

  const AsnInfo({
    required this.id,
    required this.asnNumber,
    required this.warehouseCode,
    required this.partnerCode,
    this.status,
    this.expectedDate,
    this.remark,
    required this.lines,
  });

  factory AsnInfo.fromDto(AsnServiceV1Asn dto) {
    return AsnInfo(
      id: dto.id ?? 0,
      asnNumber: dto.asnNumber ?? '',
      warehouseCode: dto.warehouseCode ?? '',
      partnerCode: dto.partnerCode ?? '',
      status: dto.status,
      expectedDate: dto.expectedDate,
      remark: dto.remark,
      lines: (dto.lines ?? const [])
          .map(AsnLineInfo.fromDto)
          .toList(growable: false),
    );
  }

  /// 剩余可收数量（全行合计，防超收口径）。
  int get totalRemainingToReceive => lines.fold(
        0,
        (sum, l) => sum + (l.expectedQty - l.receivedQty).clamp(0, 1 << 62),
      );
}

/// 到货明细。
class AsnLineInfo {
  final int id;
  final String skuCode;
  final int expectedQty;
  final int receivedQty;
  final int putawayQty;
  final AsnServiceV1AsnQcStatus? qcStatus;
  final String? lotName;

  const AsnLineInfo({
    required this.id,
    required this.skuCode,
    required this.expectedQty,
    required this.receivedQty,
    required this.putawayQty,
    this.qcStatus,
    this.lotName,
  });

  factory AsnLineInfo.fromDto(AsnServiceV1AsnLine dto) {
    return AsnLineInfo(
      id: dto.id ?? 0,
      skuCode: dto.skuCode ?? '',
      expectedQty: dto.expectedQty ?? 0,
      receivedQty: dto.receivedQty ?? 0,
      putawayQty: dto.putawayQty ?? 0,
      qcStatus: dto.qcStatus,
      lotName: dto.lotName,
    );
  }

  /// 剩余可收。
  int get remainingToReceive =>
      (expectedQty - receivedQty).clamp(0, 1 << 62);

  /// 剩余可上架（质检通过才有意义）。
  int get remainingToPutaway =>
      (receivedQty - putawayQty).clamp(0, 1 << 62);

  /// 质检通过才可上架。
  bool get isQcPass => qcStatus == AsnServiceV1AsnQcStatus.qcPass;
}

/// 收货行草稿（按明细增量）。
class ReceiveLineDraft {
  final int asnLineId;
  final int quantity;
  final AsnServiceV1AsnQcStatus qcStatus;
  final String? lotName;

  const ReceiveLineDraft({
    required this.asnLineId,
    required this.quantity,
    this.qcStatus = AsnServiceV1AsnQcStatus.qcPass,
    this.lotName,
  });
}

/// 上架行草稿（按明细增量，指定目标库位）。
class PutawayLineDraft {
  final int asnLineId;
  final int quantity;
  final int targetLocationId;

  const PutawayLineDraft({
    required this.asnLineId,
    required this.quantity,
    required this.targetLocationId,
  });
}

/// 库存位置（上架目标库位选择；仅 INTERNAL 可作目标）。
class LocationInfo {
  final int id;
  final String name;

  const LocationInfo({required this.id, required this.name});

  factory LocationInfo.fromDto(InventoryServiceV1StockLocation dto) {
    return LocationInfo(id: dto.id ?? 0, name: dto.name ?? '');
  }
}
