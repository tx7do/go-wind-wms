/** 拣货类型 → 单据标题与类型标签。 */
const PICKING_DOC_META: Record<string, { title: string; type: string }> = {
  INCOMING: { title: '入 库 单', type: '入库' },
  OUTGOING: { title: '出 库 单', type: '出库' },
  INTERNAL: { title: '调 拨 单', type: '调拨' },
  INVENTORY_ADJUSTMENT: { title: '盘 点 单', type: '盘点' },
};

const PICKING_STATE_LABELS: Record<string, string> = {
  DRAFT: '草稿',
  CONFIRMED: '已确认',
  DONE: '已完成',
  CANCELLED: '已取消',
};

/**
 * 拣货单/盘点单打印文档构建。
 *
 * 打印内容为 A4 竖版单据：标题 + 单号/打印时间、主信息区、明细表、
 * 合计（含人民币大写）、备注与签署栏。品名/规格/单位等主数据在打印时
 * 从对应档案接口解析（列表行只带编码）。
 */

import { $t } from '@vben/locales';

import {
  apiClient,
  fetchListWarehouses,
  PaginationQuery,
} from '#/api';

import { escapeHtml, printHtml } from './print';

/** 人民币金额大写（精确到分），如 1234.56 → 壹仟贰佰叁拾肆元伍角陆分。 */

function formatDateTime(iso: null | string | undefined): string {
  if (!iso) return '—';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  const p = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`;
}



async function loadRefMap<T extends { code?: string }>(
  fetch: (
    params: PaginationQuery,
  ) => Promise<{ items?: T[] } | undefined | null>,
): Promise<Map<string, T>> {
  const result = await fetch(
    new PaginationQuery({ paging: { page: 1, pageSize: 500 } }),
  );
  return new Map((result?.items ?? []).map((it) => [it.code ?? '', it]));
}

/** 打印采购单（order 需带明细，通常来自 Get 接口）。 */
export async function printStockPickingById(id: number): Promise<void> {
  const picking = await apiClient.stockPickingService.Get({ id });
  if (!picking?.id) return;

  const warehouses = await loadRefMap(fetchListWarehouses);
  const meta = PICKING_DOC_META[picking.pickingType ?? ''] ?? {
    title: '拣 货 单',
    type: picking.pickingType ?? '—',
  };
  const state = PICKING_STATE_LABELS[picking.derivedState ?? ''] ?? '—';

  const whLabel = (code?: string) => {
    if (!code) return '—';
    const w = warehouses.get(code);
    return w?.name ? `${code} — ${w.name}` : code;
  };

  const body = (picking.moves ?? [])
    .map((m) => {
      const cells = [m.productCode ?? '', String(m.plannedQuantity ?? 0)];
      return `<tr>${cells
        .map((c, i) => `<td${i === 1 ? ' class="num"' : ''}>${escapeHtml(c)}</td>`)
        .join('')}</tr>`;
    })
    .join('');

  const html = `
<div class="doc">
  <h1 class="doc-title">${escapeHtml(meta.title)}</h1>
  <div class="doc-sub">${escapeHtml($t('page.doc.docNumber'))}：${escapeHtml(picking.pickingNumber ?? '')}　·　${escapeHtml($t('page.doc.printTime'))}：${formatDateTime(new Date().toISOString())}</div>
  <table class="doc-info">
    <tr>
      <td>${escapeHtml($t('page.doc.pickingType'))}：${escapeHtml(meta.type)}</td>
      <td>${escapeHtml($t('page.doc.status'))}：${escapeHtml(state)}</td>
      <td>${escapeHtml($t('page.doc.partner'))}：${escapeHtml(picking.partnerCode ?? '—')}</td>
    </tr>
    <tr>
      <td colspan="2">${escapeHtml($t('page.doc.fromWarehouse'))}：${escapeHtml(whLabel(picking.fromWarehouseCode))}</td>
      <td>${escapeHtml($t('page.doc.toWarehouse'))}：${escapeHtml(whLabel(picking.toWarehouseCode))}</td>
    </tr>
  </table>
  <table class="doc-items">
    <thead>
      <tr>
        <th>${escapeHtml($t('page.doc.sku'))}</th>
        <th>${escapeHtml($t('page.doc.quantity'))}</th>
      </tr>
    </thead>
    <tbody>${body}</tbody>
  </table>
  ${picking.remark ? `<div class="doc-remark">${escapeHtml($t('page.doc.remark'))}：${escapeHtml(picking.remark)}</div>` : ''}
  <table class="doc-sign"><tr>
    <td>${escapeHtml($t('page.doc.sign.creator'))}：</td>
    <td>${escapeHtml($t('page.doc.sign.warehouseKeeper'))}：</td>
    <td>${escapeHtml($t('page.doc.sign.checker'))}：</td>
  </tr></table>
</div>`;
  printHtml(`${meta.title} ${picking.pickingNumber ?? ''}`, html);
}
