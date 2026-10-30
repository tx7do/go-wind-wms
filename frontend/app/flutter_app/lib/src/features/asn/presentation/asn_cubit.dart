import 'package:flutter_bloc/flutter_bloc.dart';

import 'package:go_wind_wms/src/features/asn/domain/asn_failure.dart';
import 'package:go_wind_wms/src/features/asn/domain/asn_models.dart';
import 'package:go_wind_wms/src/features/asn/domain/asn_repository.dart';
import 'package:go_wind_wms/src/features/asn/presentation/asn_state.dart';

/// ASN 视图模型。
///
/// [load] 拉取近期单据列表；[lookupByNumber] 扫码/手输查单（成功发详情态，
/// 由页面负责导航到详情页）；[receive] / [putaway] 提交作业并刷新详情。
/// 失败不抛异常，统一转为 [AsnFailureState]（message 为 i18n 键）。
class AsnCubit extends Cubit<AsnState> {
  final AsnRepository _repository;

  AsnCubit(this._repository) : super(const AsnLoading());

  Future<void> load() async {
    emit(const AsnLoading());
    try {
      final items = await _repository.listAsns();
      if (!isClosed) emit(AsnListReady(items));
    } on AsnFailure catch (e) {
      if (!isClosed) emit(AsnFailureState(_messageOf(e)));
    }
  }

  /// 按单号查询；成功发 [AsnDetailReady]，返回 true 供页面导航。
  Future<bool> lookupByNumber(String number) async {
    if (number.isEmpty) return false;
    try {
      final asn = await _repository.getByNumber(number);
      if (!isClosed) emit(AsnDetailReady(asn));
      return true;
    } on AsnFailure catch (e) {
      if (!isClosed) emit(AsnFailureState(_messageOf(e)));
      return false;
    }
  }

  /// 按 id 直接拉详情（详情页进入/刷新用）。
  Future<void> refreshDetail(int asnId) async {
    emit(const AsnLoading());
    try {
      final asn = await _repository.getById(asnId);
      if (!isClosed) emit(AsnDetailReady(asn));
    } on AsnFailure catch (e) {
      if (!isClosed) emit(AsnFailureState(_messageOf(e)));
    }
  }

  /// 收货（按明细增量）。成功后以服务端最新数据刷新详情。
  Future<bool> receive(int asnId, List<ReceiveLineDraft> lines) async {
    try {
      await _repository.receive(asnId, lines);
      final asn = await _reloadDetail(asnId);
      if (!isClosed) emit(AsnDetailReady(asn));
      return true;
    } on AsnFailure catch (e) {
      if (!isClosed) emit(AsnFailureState(_messageOf(e)));
      return false;
    }
  }

  /// 上架（质检通过数量 → 目标库位）。成功后刷新详情。
  Future<bool> putaway(int asnId, List<PutawayLineDraft> lines) async {
    try {
      await _repository.putaway(asnId, lines);
      final asn = await _reloadDetail(asnId);
      if (!isClosed) emit(AsnDetailReady(asn));
      return true;
    } on AsnFailure catch (e) {
      if (!isClosed) emit(AsnFailureState(_messageOf(e)));
      return false;
    }
  }

  /// 拉取库位列表（上架目标选择，仅 INTERNAL 由调用方过滤）。
  Future<List<LocationInfo>> listLocations() {
    return _repository.listLocations();
  }

  Future<AsnInfo> _reloadDetail(int asnId) {
    return _repository.getById(asnId);
  }

  String _messageOf(AsnFailure e) {
    if (e is AsnNotFoundFailure) return 'asnNotFound';
    if (e is AsnInvalidInputFailure) return 'asnInvalidInput';
    if (e is AsnUnauthorizedFailure) return 'asnUnauthorized';
    if (e is AsnNetworkFailure) return 'asnNetwork';
    return 'asnUnknown';
  }
}
