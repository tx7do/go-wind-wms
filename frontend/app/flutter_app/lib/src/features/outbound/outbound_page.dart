import 'package:flutter/material.dart';

import 'package:go_router/go_router.dart';
import 'package:get_it/get_it.dart' show GetIt;

import 'package:go_wind_wms/generated/api/app/service/v1/index.dart'
    show AsnServiceV1AsnStatus;
import 'package:go_wind_wms/generated/l10n.dart';
import 'package:go_wind_wms/src/core/constants/router_paths.dart';
import 'package:go_wind_wms/src/features/outbound/outbound_repository.dart';

/// PDA 出库扫码页（M2b）：扫/输拣货单号 → 查单 → 确认 → 校验落账。
///
/// 状态机派生态由服务端聚合：DRAFT 可确认；CONFIRMED 可校验（FEFO 扣减）；
/// DONE 为终态。与 ASN 收货页（features/asn）对称。
class OutboundPage extends StatefulWidget {
  const OutboundPage({super.key});

  @override
  State<OutboundPage> createState() => _OutboundPageState();
}

class _OutboundPageState extends State<OutboundPage> {
  final _numberController = TextEditingController();
  final _repo = createOutboundRepository();

  dynamic _picking;
  String? _message;
  bool _busy = false;
  bool _notFound = false;

  @override
  void dispose() {
    _numberController.dispose();
    super.dispose();
  }

  Future<void> _lookup() async {
    final number = _numberController.text.trim();
    if (number.isEmpty || _busy) return;
    FocusScope.of(context).unfocus();
    setState(() {
      _busy = true;
      _message = null;
      _notFound = false;
    });
    try {
      final dto = await _repo.getByNumber(number);
      setState(() {
        _picking = dto;
        _numberController.clear();
      });
    } catch (key) {
      setState(() {
        _picking = null;
        if (key == 'outboundNotFound') _notFound = true;
        _message = key as String?;
      });
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _act(String action) async {
    if (_busy || _picking == null) return;
    setState(() => _busy = true);
    try {
      if (action == 'confirm') {
        await _repo.confirm(_picking!.id ?? 0);
      } else {
        await _repo.validate(_picking!.id ?? 0);
      }
      final dto = await _repo.getByNumber(_picking!.pickingNumber ?? '');
      if (mounted) {
        setState(() {
          _picking = dto;
          _message = action == 'confirm' ? 'outboundConfirmed' : 'outboundValidated';
        });
      }
    } catch (key) {
      if (mounted) setState(() => _message = key as String?);
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final loc = S.of(context);
    return Scaffold(
      appBar: AppBar(title: Text(loc.navOutbound)),
      body: _busy
          ? const Center(child: CircularProgressIndicator())
          : ListView(
              padding: const EdgeInsets.all(12),
              children: [
                TextField(
                  controller: _numberController,
                  decoration: InputDecoration(
                    labelText: loc.outboundScanHint,
                    prefixIcon: const Icon(Icons.qr_code_scanner),
                    suffixIcon: IconButton(
                      icon: const Icon(Icons.search),
                      onPressed: _lookup,
                    ),
                    border: const OutlineInputBorder(),
                  ),
                  onSubmitted: (_) => _lookup(),
                ),
                if (_notFound)
                  Padding(
                    padding: const EdgeInsets.only(top: 12),
                    child: Text(loc.outboundNotFound,
                        style: const TextStyle(color: Colors.red)),
                  ),
                if (_message != null && !_notFound)
                  Padding(
                    padding: const EdgeInsets.only(top: 12),
                    child: Text(_messageText(loc, _message!),
                        style: const TextStyle(color: Colors.blue)),
                  ),
                if (_picking != null) ...[
                  const SizedBox(height: 12),
                  _buildPickingCard(context, loc),
                ],
              ],
            ),
    );
  }

  Widget _buildPickingCard(BuildContext context, S loc) {
    final p = _picking!;
    final state = p.derivedState?.name ?? 'DRAFT';
    final canConfirm = state == 'DRAFT';
    final canValidate = state == 'CONFIRMED';

    return Card(
      child: Padding(
        padding: const EdgeInsets.all(12),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Expanded(
                  child: Text(p.pickingNumber ?? '',
                      style: const TextStyle(
                          fontSize: 16, fontWeight: FontWeight.w700)),
                ),
                Text(state,
                    style: TextStyle(
                        color: state == 'DONE' ? Colors.green : Colors.orange)),
              ],
            ),
            const SizedBox(height: 6),
            Text('${loc.outboundConsignee}: ${p.partnerCode ?? '—'}',
                style: Theme.of(context).textTheme.bodySmall),
            const Divider(height: 20),
            for (final m in p.moves ?? const [])
              ListTile(
                dense: true,
                contentPadding: EdgeInsets.zero,
                title: Text(m.productCode ?? ''),
                trailing: Text('× ${m.plannedQuantity ?? 0}'),
              ),
            const SizedBox(height: 8),
            Row(
              children: [
                Expanded(
                  child: FilledButton.tonal(
                    onPressed: canConfirm ? () => _act('confirm') : null,
                    child: Text(loc.outboundConfirm),
                  ),
                ),
                const SizedBox(width: 12),
                Expanded(
                  child: FilledButton(
                    onPressed: canValidate ? () => _act('validate') : null,
                    child: Text(loc.outboundValidate),
                  ),
                ),
              ],
            ),
          ],
        ),
      ),
    );
  }

  String _messageText(S loc, String key) {
    return switch (key) {
      'outboundNotFound' => loc.outboundNotFound,
      'outboundRejected' => loc.outboundRejected,
      'outboundUnauthorized' => loc.outboundUnauthorized,
      'outboundNetwork' => loc.outboundNetwork,
      'outboundConfirmed' => loc.outboundConfirmed,
      'outboundValidated' => loc.outboundValidated,
      _ => loc.outboundUnknown,
    };
  }
}
