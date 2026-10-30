import 'package:flutter/material.dart';
import 'package:flutter_bloc/flutter_bloc.dart';

import 'package:go_wind_wms/generated/api/app/service/v1/index.dart'
    show AsnServiceV1AsnQcStatus, AsnServiceV1AsnStatus;
import 'package:go_wind_wms/generated/l10n.dart';
import 'package:go_wind_wms/src/features/asn/domain/asn_models.dart';
import 'package:go_wind_wms/src/features/asn/presentation/asn_cubit.dart';
import 'package:go_wind_wms/src/features/asn/presentation/asn_state.dart';

/// ASN 详情页：单据头 + 明细三口径 + 收货/上架作业。
///
/// 收货（ANNOUNCED/RECEIVING 可用）：按行录入本次数量、质检结果、批次，
/// 防超收由前端预检 + 后端行级条件更新双保险。
/// 上架（RECEIVING 且质检通过可用）：按行录入数量并选目标库位，
/// 库位列表取自 `/app/v1/stock-locations`（仅 INTERNAL 可选）。
class AsnDetailPage extends StatefulWidget {
  final int asnId;

  const AsnDetailPage({super.key, required this.asnId});

  @override
  State<AsnDetailPage> createState() => _AsnDetailPageState();
}

class _AsnDetailPageState extends State<AsnDetailPage> {
  @override
  void initState() {
    super.initState();
    // 进入即按 id 拉详情（复用 cubit 但绕过 lookup——直接发详情态）。
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (!mounted) return;
      context.read<AsnCubit>().refreshDetail(widget.asnId);
    });
  }

  @override
  Widget build(BuildContext context) {
    final loc = S.of(context);
    return Scaffold(
      appBar: AppBar(title: Text(loc.navInbound)),
      body: BlocConsumer<AsnCubit, AsnState>(
        listener: (context, state) {
          if (state is AsnFailureState) {
            ScaffoldMessenger.of(context)
              ..hideCurrentSnackBar()
              ..showSnackBar(
                SnackBar(
                  content: Text(
                    switch (state.message) {
                      'asnInvalidInput' => loc.asnInvalidInput,
                      'asnUnauthorized' => loc.asnUnauthorized,
                      'asnNetwork' => loc.asnNetwork,
                      _ => loc.asnUnknown,
                    },
                  ),
                ),
              );
          }
        },
        builder: (context, state) {
          if (state is AsnDetailReady) {
            return _buildDetail(context, loc, state.asn);
          }
          if (state is AsnLoading) {
            return const Center(child: CircularProgressIndicator());
          }
          return Center(child: Text(loc.asnUnknown));
        },
      ),
    );
  }

  Widget _buildDetail(BuildContext context, S loc, AsnInfo asn) {
    final canReceive = asn.status == AsnServiceV1AsnStatus.asnAnnounced ||
        asn.status == AsnServiceV1AsnStatus.asnReceiving;
    final canPutaway = asn.status == AsnServiceV1AsnStatus.asnReceiving;

    return Column(
      children: [
        _buildHeader(context, loc, asn),
        Expanded(
          child: ListView.separated(
            padding: const EdgeInsets.only(bottom: 8),
            itemCount: asn.lines.length,
            separatorBuilder: (_, _) => const Divider(height: 1),
            itemBuilder: (context, index) {
              final line = asn.lines[index];
              return _buildLineTile(context, loc, line);
            },
          ),
        ),
        SafeArea(
          child: Padding(
            padding: const EdgeInsets.all(12),
            child: Row(
              children: [
                Expanded(
                  child: FilledButton.tonal(
                    onPressed: canReceive
                        ? () => _showReceiveSheet(context, loc, asn)
                        : null,
                    child: Text(loc.asnActionReceive),
                  ),
                ),
                const SizedBox(width: 12),
                Expanded(
                  child: FilledButton(
                    onPressed:
                        canPutaway ? () => _showPutawaySheet(context, loc, asn) : null,
                    child: Text(loc.asnActionPutaway),
                  ),
                ),
              ],
            ),
          ),
        ),
      ],
    );
  }

  Widget _buildHeader(BuildContext context, S loc, AsnInfo asn) {
    return Card(
      margin: const EdgeInsets.all(12),
      child: Padding(
        padding: const EdgeInsets.all(12),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Expanded(
                  child: Text(
                    asn.asnNumber,
                    style: const TextStyle(
                      fontSize: 16,
                      fontWeight: FontWeight.w700,
                    ),
                  ),
                ),
                _statusChip(loc, asn.status),
              ],
            ),
            const SizedBox(height: 6),
            Text(
              '${loc.asnWarehouse}: ${asn.warehouseCode}　'
              '${loc.asnPartner}: ${asn.partnerCode}',
              style: Theme.of(context).textTheme.bodySmall,
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildLineTile(BuildContext context, S loc, AsnLineInfo line) {
    final loc2 = S.of(context);
    return ListTile(
      dense: true,
      title: Text(line.skuCode),
      subtitle: Text(
        '${loc2.asnQtyExpected} ${line.expectedQty} · '
        '${loc2.asnQtyReceived} ${line.receivedQty} · '
        '${loc2.asnQtyPutaway} ${line.putawayQty}',
      ),
      trailing: line.isQcPass
          ? null
          : Chip(
              label: Text(loc2.asnQcFail),
              visualDensity: VisualDensity.compact,
              backgroundColor: Colors.red.withValues(alpha: 0.1),
              labelStyle: const TextStyle(color: Colors.red, fontSize: 11),
            ),
    );
  }

  Widget _statusChip(S loc, AsnServiceV1AsnStatus? status) {
    final (label, color) = switch (status?.name) {
      'ASN_DRAFT' => (loc.asnStatusDraft, Colors.grey),
      'ASN_ANNOUNCED' => (loc.asnStatusAnnounced, Colors.blue),
      'ASN_RECEIVING' => (loc.asnStatusReceiving, Colors.orange),
      'ASN_COMPLETED' => (loc.asnStatusCompleted, Colors.green),
      'ASN_CANCELLED' => (loc.asnStatusCancelled, Colors.red),
      _ => ('—', Colors.grey),
    };
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 4),
      decoration: BoxDecoration(
        color: color.withValues(alpha: 0.12),
        borderRadius: BorderRadius.circular(12),
      ),
      child: Text(label, style: TextStyle(color: color, fontSize: 12)),
    );
  }

  // ═══════════════════════ 收货 ═══════════════════════

  void _showReceiveSheet(BuildContext context, S loc, AsnInfo asn) {
    final cubit = context.read<AsnCubit>();
    final controllers = <int, TextEditingController>{};
    final lotControllers = <int, TextEditingController>{};
    final qc = <int, AsnServiceV1AsnQcStatus>{};

    showModalBottomSheet<void>(
      context: context,
      isScrollControlled: true,
      builder: (sheetContext) {
        return Padding(
          padding: EdgeInsets.only(
            left: 16,
            right: 16,
            top: 16,
            bottom: MediaQuery.of(sheetContext).viewInsets.bottom + 16,
          ),
          child: StatefulBuilder(
            builder: (sheetContext, setSheetState) {
              return Column(
                mainAxisSize: MainAxisSize.min,
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    loc.asnReceiveTitle(asn.asnNumber),
                    style: const TextStyle(
                      fontSize: 16,
                      fontWeight: FontWeight.w700,
                    ),
                  ),
                  const SizedBox(height: 12),
                  Flexible(
                    child: ListView(
                      shrinkWrap: true,
                      children: [
                        for (final line in asn.lines)
                          if (line.remainingToReceive > 0)
                            Padding(
                              padding: const EdgeInsets.only(bottom: 10),
                              child: Column(
                                crossAxisAlignment: CrossAxisAlignment.start,
                                children: [
                                  Text(
                                    '${line.skuCode}（${loc.asnReceiveRemaining} '
                                    '${line.remainingToReceive}）',
                                    style: const TextStyle(
                                      fontWeight: FontWeight.w600,
                                    ),
                                  ),
                                  const SizedBox(height: 6),
                                  Row(
                                    children: [
                                      Expanded(
                                        child: TextFormField(
                                          controller: controllers[line.id] ??=
                                              TextEditingController(),
                                          keyboardType: TextInputType.number,
                                          decoration: InputDecoration(
                                            labelText: loc.asnReceiveQty,
                                            isDense: true,
                                            border: const OutlineInputBorder(),
                                          ),
                                        ),
                                      ),
                                      const SizedBox(width: 8),
                                      SegmentedButton<String>(
                                        segments: [
                                          ButtonSegment(
                                            value: 'PASS',
                                            label: Text(loc.asnQcPass),
                                          ),
                                          ButtonSegment(
                                            value: 'FAIL',
                                            label: Text(loc.asnQcFail),
                                          ),
                                        ],
                                        selected: {
                                          (qc[line.id] ??
                                                  AsnServiceV1AsnQcStatus.qcPass) ==
                                              AsnServiceV1AsnQcStatus.qcPass
                                              ? 'PASS'
                                              : 'FAIL'
                                        },
                                        onSelectionChanged: (selection) {
                                          setSheetState(() {
                                            qc[line.id] = selection.first ==
                                                    'PASS'
                                                ? AsnServiceV1AsnQcStatus.qcPass
                                                : AsnServiceV1AsnQcStatus.qcFail;
                                          });
                                        },
                                      ),
                                    ],
                                  ),
                                  const SizedBox(height: 6),
                                  TextFormField(
                                    controller: lotControllers[line.id] ??=
                                        TextEditingController(),
                                    decoration: InputDecoration(
                                      labelText: loc.asnLotName,
                                      isDense: true,
                                      border: const OutlineInputBorder(),
                                    ),
                                  ),
                                ],
                              ),
                            ),
                      ],
                    ),
                  ),
                  const SizedBox(height: 12),
                  Row(
                    mainAxisAlignment: MainAxisAlignment.end,
                    children: [
                      TextButton(
                        onPressed: () => Navigator.of(sheetContext).pop(),
                        child: Text(loc.asnCancel),
                      ),
                      const SizedBox(width: 8),
                      FilledButton(
                        onPressed: () async {
                          final drafts = <ReceiveLineDraft>[];
                          for (final line in asn.lines) {
                            final c = controllers[line.id];
                            if (c == null) continue;
                            final qty = int.tryParse(c.text.trim()) ?? 0;
                            if (qty <= 0) continue;
                            if (qty > line.remainingToReceive) {
                              ScaffoldMessenger.of(context)
                                ..hideCurrentSnackBar()
                                ..showSnackBar(
                                  SnackBar(content: Text(loc.asnOverReceive)),
                                );
                              return;
                            }
                            drafts.add(
                              ReceiveLineDraft(
                                asnLineId: line.id,
                                quantity: qty,
                                qcStatus: qc[line.id] ??
                                    AsnServiceV1AsnQcStatus.qcPass,
                                lotName: lotControllers[line.id]?.text.trim(),
                              ),
                            );
                          }
                          if (drafts.isEmpty) {
                            ScaffoldMessenger.of(context)
                              ..hideCurrentSnackBar()
                              ..showSnackBar(
                                SnackBar(content: Text(loc.asnNothingToReceive)),
                              );
                            return;
                          }
                          final ok = await cubit.receive(asn.id, drafts);
                          if (sheetContext.mounted) {
                            Navigator.of(sheetContext).pop();
                          }
                          if (ok && context.mounted) {
                            ScaffoldMessenger.of(context)
                              ..hideCurrentSnackBar()
                              ..showSnackBar(
                                SnackBar(content: Text(loc.asnReceiveSuccess)),
                              );
                          }
                        },
                        child: Text(loc.asnActionReceive),
                      ),
                    ],
                  ),
                ],
              );
            },
          ),
        );
      },
    );
  }

  // ═══════════════════════ 上架 ═══════════════════════

  void _showPutawaySheet(BuildContext context, S loc, AsnInfo asn) {
    final cubit = context.read<AsnCubit>();
    final controllers = <int, TextEditingController>{};
    final selectedLocation = <int, int?>{};

    showModalBottomSheet<void>(
      context: context,
      isScrollControlled: true,
      builder: (sheetContext) {
        return FutureBuilder<List<LocationInfo>>(
          future: cubit.listLocations(),
          builder: (sheetContext, snapshot) {
            final locations = (snapshot.data ?? const <LocationInfo>[])
                .where((l) => l.id > 0)
                .toList();
            return Padding(
              padding: EdgeInsets.only(
                left: 16,
                right: 16,
                top: 16,
                bottom: MediaQuery.of(sheetContext).viewInsets.bottom + 16,
              ),
              child: StatefulBuilder(
                builder: (sheetContext, setSheetState) {
                  return Column(
                    mainAxisSize: MainAxisSize.min,
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                        loc.asnPutawayTitle(asn.asnNumber),
                        style: const TextStyle(
                          fontSize: 16,
                          fontWeight: FontWeight.w700,
                        ),
                      ),
                      const SizedBox(height: 12),
                      Flexible(
                        child: ListView(
                          shrinkWrap: true,
                          children: [
                            for (final line in asn.lines)
                              if (line.isQcPass &&
                                  line.remainingToPutaway > 0)
                                Padding(
                                  padding: const EdgeInsets.only(bottom: 10),
                                  child: Column(
                                    crossAxisAlignment:
                                        CrossAxisAlignment.start,
                                    children: [
                                      Text(
                                        '${line.skuCode}（${loc.asnPutawayRemaining} '
                                        '${line.remainingToPutaway}）',
                                        style: const TextStyle(
                                          fontWeight: FontWeight.w600,
                                        ),
                                      ),
                                      const SizedBox(height: 6),
                                      Row(
                                        children: [
                                          Expanded(
                                            child: TextFormField(
                                              controller:
                                                  controllers[line.id] ??=
                                                      TextEditingController(),
                                              keyboardType:
                                                  TextInputType.number,
                                              decoration: InputDecoration(
                                                labelText: loc.asnPutawayQty,
                                                isDense: true,
                                                border:
                                                    const OutlineInputBorder(),
                                              ),
                                            ),
                                          ),
                                          const SizedBox(width: 8),
                                          Expanded(
                                            child:
                                                DropdownButtonFormField<int>(
                                              value: selectedLocation[line.id],
                                              decoration: InputDecoration(
                                                labelText: loc
                                                    .asnPutawayTargetLocation,
                                                isDense: true,
                                                border:
                                                    const OutlineInputBorder(),
                                              ),
                                              items: locations
                                                  .map(
                                                    (l) =>
                                                        DropdownMenuItem<int>(
                                                      value: l.id,
                                                      child: Text(
                                                        l.name,
                                                        overflow: TextOverflow
                                                            .ellipsis,
                                                      ),
                                                    ),
                                                  )
                                                  .toList(),
                                              onChanged: (v) =>
                                                  setSheetState(() {
                                                selectedLocation[line.id] = v;
                                              }),
                                            ),
                                          ),
                                        ],
                                      ),
                                    ],
                                  ),
                                ),
                          ],
                        ),
                      ),
                      const SizedBox(height: 12),
                      Row(
                        mainAxisAlignment: MainAxisAlignment.end,
                        children: [
                          TextButton(
                            onPressed: () => Navigator.of(sheetContext).pop(),
                            child: Text(loc.asnCancel),
                          ),
                          const SizedBox(width: 8),
                          FilledButton(
                            onPressed: () async {
                              final drafts = <PutawayLineDraft>[];
                              for (final line in asn.lines) {
                                final c = controllers[line.id];
                                if (c == null) continue;
                                final qty = int.tryParse(c.text.trim()) ?? 0;
                                if (qty <= 0) continue;
                                if (qty > line.remainingToPutaway) {
                                  ScaffoldMessenger.of(context)
                                    ..hideCurrentSnackBar()
                                    ..showSnackBar(
                                      SnackBar(
                                        content: Text(loc.asnOverPutaway),
                                      ),
                                    );
                                  return;
                                }
                                final target = selectedLocation[line.id];
                                if (target == null) {
                                  ScaffoldMessenger.of(context)
                                    ..hideCurrentSnackBar()
                                    ..showSnackBar(
                                      SnackBar(
                                        content:
                                            Text(loc.asnLocationRequired),
                                      ),
                                    );
                                  return;
                                }
                                drafts.add(
                                  PutawayLineDraft(
                                    asnLineId: line.id,
                                    quantity: qty,
                                    targetLocationId: target,
                                  ),
                                );
                              }
                              if (drafts.isEmpty) {
                                ScaffoldMessenger.of(context)
                                  ..hideCurrentSnackBar()
                                  ..showSnackBar(
                                    SnackBar(
                                      content:
                                          Text(loc.asnNothingToPutaway),
                                    ),
                                  );
                                return;
                              }
                              final ok = await cubit.putaway(asn.id, drafts);
                              if (sheetContext.mounted) {
                                Navigator.of(sheetContext).pop();
                              }
                              if (ok && context.mounted) {
                                ScaffoldMessenger.of(context)
                                  ..hideCurrentSnackBar()
                                  ..showSnackBar(
                                    SnackBar(
                                      content: Text(loc.asnPutawaySuccess),
                                    ),
                                  );
                              }
                            },
                            child: Text(loc.asnActionPutaway),
                          ),
                        ],
                      ),
                    ],
                  );
                },
              ),
            );
          },
        );
      },
    );
  }
}
