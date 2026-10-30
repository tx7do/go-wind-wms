import 'package:go_wind_wms/src/features/asn/domain/asn_models.dart';

/// ASN（到货通知单）仓储抽象。
///
/// presentation 层仅依赖本接口；实现见 data 层 [AsnRepositoryImpl]。
abstract class AsnRepository {
  /// 拉取近期到货通知单（默认 20 条）。
  Future<List<AsnInfo>> listAsns({int limit = 20});

  /// 按单号查询（PDA 扫码/手输入口）；未命中抛 [AsnNotFoundFailure]。
  Future<AsnInfo> getByNumber(String number);

  /// 按单据 ID 查询详情（收货/上架后刷新）。
  Future<AsnInfo> getById(int id);

  /// 收货（按明细增量，防超收后端兜底）。
  /// 返回生成的入库拣货单 ID。
  Future<int> receive(int asnId, List<ReceiveLineDraft> lines);

  /// 上架（质检通过的已收数量 → 目标库位，防超上架后端兜底）。
  /// 返回生成的上架拣货单 ID。
  Future<int> putaway(int asnId, List<PutawayLineDraft> lines);

  /// 拉取库位列表（上架目标库位选择；调用方过滤 INTERNAL）。
  Future<List<LocationInfo>> listLocations({int limit = 500});
}
