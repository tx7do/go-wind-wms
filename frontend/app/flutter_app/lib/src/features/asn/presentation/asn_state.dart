import 'package:go_wind_wms/src/features/asn/domain/asn_models.dart';

/// ASN 视图状态。
sealed class AsnState {
  const AsnState();
}

/// 初始/加载中。
final class AsnLoading extends AsnState {
  const AsnLoading();
}

/// 列表就绪（近期到货通知单）。
final class AsnListReady extends AsnState {
  final List<AsnInfo> items;

  const AsnListReady(this.items);
}

/// 详情就绪（扫码/点击进入）。
final class AsnDetailReady extends AsnState {
  final AsnInfo asn;

  const AsnDetailReady(this.asn);
}

/// 操作失败（防超收/防超上架/网络等），message 为 i18n 键。
final class AsnFailureState extends AsnState {
  final String message;

  const AsnFailureState(this.message);
}
