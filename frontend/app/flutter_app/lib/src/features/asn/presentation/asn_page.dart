import 'package:flutter/material.dart';
import 'package:flutter_bloc/flutter_bloc.dart';
import 'package:go_router/go_router.dart';

import 'package:go_wind_wms/generated/l10n.dart';
import 'package:go_wind_wms/src/core/constants/router_paths.dart';
import 'package:go_wind_wms/src/features/asn/presentation/asn_cubit.dart';
import 'package:go_wind_wms/src/features/asn/presentation/asn_state.dart';
import 'package:go_wind_wms/src/features/asn/domain/asn_models.dart';

/// ASN 到货通知页（PDA 入库链入口）。
///
/// 顶部单号输入框：扫码枪录入或手输 ASN 单号 → 查单 → 进详情收货/上架；
/// 下方为近期到货通知单列表，点击同样进入详情。
class AsnPage extends StatefulWidget {
  const AsnPage({super.key});

  @override
  State<AsnPage> createState() => _AsnPageState();
}

class _AsnPageState extends State<AsnPage> {
  final _numberController = TextEditingController();
  final _focusNode = FocusNode();

  @override
  void dispose() {
    _numberController.dispose();
    _focusNode.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final loc = S.of(context);
    return Scaffold(
      appBar: AppBar(title: Text(loc.navInbound)),
      body: BlocConsumer<AsnCubit, AsnState>(
        listener: (context, state) {
          if (state is AsnDetailReady) {
            // 查单成功 → 进详情（详情页自己 push，携带 id）；
            // 返回后刷新列表。
            final id = state.asn.id;
            _numberController.clear();
            _focusNode.requestFocus();
            context
                .push('${AppRoutePath.asnDetail}/$id')
                .whenComplete(() {
              if (context.mounted) {
                context.read<AsnCubit>().load();
              }
            });
          } else if (state is AsnFailureState) {
            ScaffoldMessenger.of(context)
              ..hideCurrentSnackBar()
              ..showSnackBar(
                SnackBar(content: Text(_failureText(loc, state.message))),
              );
          }
        },
        builder: (context, state) {
          return Column(
            children: [
              Padding(
                padding: const EdgeInsets.all(12),
                child: TextField(
                  controller: _numberController,
                  focusNode: _focusNode,
                  decoration: InputDecoration(
                    labelText: loc.asnScanHint,
                    prefixIcon: const Icon(Icons.qr_code_scanner),
                    suffixIcon: IconButton(
                      icon: const Icon(Icons.search),
                      onPressed: () => _lookup(context),
                    ),
                    border: const OutlineInputBorder(),
                  ),
                  onSubmitted: (_) => _lookup(context),
                ),
              ),
              Expanded(child: _buildBody(context, loc, state)),
            ],
          );
        },
      ),
    );
  }

  void _lookup(BuildContext context) {
    final number = _numberController.text.trim();
    if (number.isEmpty) return;
    FocusScope.of(context).unfocus();
    context.read<AsnCubit>().lookupByNumber(number);
  }

  Widget _buildBody(BuildContext context, S loc, AsnState state) {
    if (state is AsnLoading) {
      return const Center(child: CircularProgressIndicator());
    }
    if (state is AsnFailureState) {
      return Center(child: Text(_failureText(loc, state.message)));
    }
    if (state is AsnListReady) {
      if (state.items.isEmpty) {
        return Center(child: Text(loc.asnEmpty));
      }
      return RefreshIndicator(
        onRefresh: () => context.read<AsnCubit>().load(),
        child: ListView.separated(
          itemCount: state.items.length,
          separatorBuilder: (_, _) => const Divider(height: 1),
          itemBuilder: (context, index) {
            final asn = state.items[index];
            return ListTile(
              title: Text(
                asn.asnNumber,
                style: const TextStyle(fontWeight: FontWeight.w600),
              ),
              subtitle: Text(
                '${asn.warehouseCode} · ${asn.partnerCode}',
              ),
              trailing: _StatusChip(status: asn.status),
              onTap: () {
                context
                    .push('${AppRoutePath.asnDetail}/${asn.id}')
                    .whenComplete(() {
                  if (context.mounted) {
                    context.read<AsnCubit>().load();
                  }
                });
              },
            );
          },
        ),
      );
    }
    return const SizedBox.shrink();
  }

  String _failureText(S loc, String key) {
    return switch (key) {
      'asnNotFound' => loc.asnNotFound,
      'asnInvalidInput' => loc.asnInvalidInput,
      'asnUnauthorized' => loc.asnUnauthorized,
      'asnNetwork' => loc.asnNetwork,
      _ => loc.asnUnknown,
    };
  }
}

/// 单据状态角标。
class _StatusChip extends StatelessWidget {
  final dynamic status;

  const _StatusChip({required this.status});

  @override
  Widget build(BuildContext context) {
    final loc = S.of(context);
    final (label, color) = switch (status?.name) {
      'ASN_DRAFT' => (loc.asnStatusDraft, Colors.grey),
      'ASN_ANNOUNCED' => (loc.asnStatusAnnounced, Colors.blue),
      'ASN_RECEIVING' => (loc.asnStatusReceiving, Colors.orange),
      'ASN_COMPLETED' => (loc.asnStatusCompleted, Colors.green),
      'ASN_CANCELLED' => (loc.asnStatusCancelled, Colors.red),
      _ => ('—', Colors.grey),
    };
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 3),
      decoration: BoxDecoration(
        color: color.withValues(alpha: 0.12),
        borderRadius: BorderRadius.circular(10),
      ),
      child: Text(
        label,
        style: TextStyle(color: color, fontSize: 12),
      ),
    );
  }
}
