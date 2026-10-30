<script lang="ts" setup>
import type { VxeGridProps } from '#/adapter/vxe-table';

import { Page, type VbenFormProps } from '@vben/common-ui';

import { notification } from 'ant-design-vue';

import { useVbenVxeGrid } from '#/adapter/vxe-table';
import {
  apiClient,
  fetchListStockQuants,
  PaginationQuery,
  type inventoryservicev1_StockQuant as StockQuant,
} from '#/api';
import { $t } from '#/locales';

const formOptions: VbenFormProps = {
  collapsed: false,
  showCollapseButton: false,
  submitOnEnter: true,
  schema: [
    {
      component: 'Input',
      fieldName: 'locationId',
      label: $t('page.stockQuant.locationId'),
      componentProps: {
        placeholder: $t('ui.placeholder.input'),
        allowClear: true,
      },
    },
    {
      component: 'Input',
      fieldName: 'productCode',
      label: $t('page.stockQuant.productCode'),
      componentProps: {
        placeholder: $t('ui.placeholder.input'),
        allowClear: true,
      },
    },
  ],
};

const gridOptions: VxeGridProps<StockQuant> = {
  toolbarConfig: {
    custom: true,
    export: true,
    refresh: true,
    zoom: true,
  },
  height: 'auto',
  exportConfig: {},
  pagerConfig: {},
  rowConfig: {
    isHover: true,
  },

  proxyConfig: {
    ajax: {
      query: async ({ page }, formValues) => {
        return await fetchListStockQuants(
          new PaginationQuery({
            paging: { page: page.currentPage, pageSize: page.pageSize },
            formValues,
          }),
        );
      },
    },
  },

  columns: [
    { title: $t('ui.table.seq'), type: 'seq', width: 50 },
    { title: $t('page.stockQuant.locationId'), field: 'locationId' },
    { title: $t('page.stockQuant.productCode'), field: 'productCode' },
    { title: $t('page.stockQuant.quantity'), field: 'quantity' },
    {
      title: $t('page.stockQuant.frozen'),
      field: 'frozen',
      slots: { default: 'frozen' },
      width: 95,
    },
    {
      title: $t('ui.table.createdAt'),
      field: 'createdAt',
      formatter: 'formatDateTime',
      width: 140,
    },
    {
      title: $t('ui.table.action'),
      field: 'action',
      fixed: 'right',
      slots: { default: 'action' },
      width: 120,
    },
  ],
};

const [Grid, gridApi] = useVbenVxeGrid({ gridOptions, formOptions });

async function handleFreeze(row: any, freeze: boolean) {
  row.pending = true;
  try {
    if (freeze) {
      await apiClient.stockQuantService.Freeze({ id: row.id });
    } else {
      await apiClient.stockQuantService.Unfreeze({ id: row.id });
    }
    notification.success({
      message: $t('ui.notification.update_status_success'),
    });
    await gridApi.reload();
  } catch {
    notification.error({
      message: $t('ui.notification.update_status_failed'),
    });
  } finally {
    row.pending = false;
  }
}
</script>

<template>
  <Page auto-content-height>
    <Grid :table-title="$t('menu.wms.stockQuant')">
      <template #frozen="{ row }">
        <a-tag :color="row.frozen ? 'red' : 'default'">
          {{
            row.frozen
              ? $t('page.stockQuant.frozenYes')
              : $t('page.stockQuant.frozenNo')
          }}
        </a-tag>
      </template>
      <template #action="{ row }">
        <a-button
          v-if="!row.frozen"
          danger
          type="link"
          size="small"
          @click.stop="handleFreeze(row, true)"
        >
          {{ $t('page.stockQuant.freeze') }}
        </a-button>
        <a-button
          v-else
          type="link"
          size="small"
          @click.stop="handleFreeze(row, false)"
        >
          {{ $t('page.stockQuant.unfreeze') }}
        </a-button>
      </template>
    </Grid>
  </Page>
</template>
