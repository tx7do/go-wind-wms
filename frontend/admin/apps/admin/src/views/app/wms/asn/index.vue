<script lang="ts" setup>
import type { VxeGridProps } from '#/adapter/vxe-table';

import { h, ref } from 'vue';

import { Page, useVbenDrawer, useVbenModal, type VbenFormProps } from '@vben/common-ui';
import { LucideFilePenLine, LucideTrash2 } from '@vben/icons';

import { Modal, notification, Tag } from 'ant-design-vue';

import { useVbenVxeGrid } from '#/adapter/vxe-table';
import {
  apiClient,
  fetchListAsns,
  PaginationQuery,
  type asnservicev1_Asn as Asn,
} from '#/api';
import { $t } from '#/locales';

import AsnDrawer from './asn-drawer.vue';
import AsnReceiveModal from './asn-receive-modal.vue';
import AsnPutawayModal from './asn-putaway-modal.vue';

const ASN_STATUS = {
  ASN_DRAFT: { color: 'default', labelKey: 'page.asn.statusText.draft' },
  ASN_ANNOUNCED: { color: 'processing', labelKey: 'page.asn.statusText.announced' },
  ASN_RECEIVING: { color: 'warning', labelKey: 'page.asn.statusText.receiving' },
  ASN_COMPLETED: { color: 'success', labelKey: 'page.asn.statusText.completed' },
  ASN_CANCELLED: { color: 'error', labelKey: 'page.asn.statusText.cancelled' },
} as const;

function statusOf(row: Asn) {
  return ASN_STATUS[(row.status ?? 'ASN_DRAFT') as keyof typeof ASN_STATUS] ??
    ASN_STATUS.ASN_DRAFT;
}

const formOptions: VbenFormProps = {
  collapsed: false,
  showCollapseButton: false,
  submitOnEnter: true,
  schema: [
    {
      component: 'Input',
      fieldName: 'asnNumber',
      label: $t('page.asn.asnNumber'),
      componentProps: {
        placeholder: $t('ui.placeholder.input'),
        allowClear: true,
      },
    },
    {
      component: 'Input',
      fieldName: 'warehouseCode',
      label: $t('page.asn.warehouseCode'),
      componentProps: {
        placeholder: $t('ui.placeholder.input'),
        allowClear: true,
      },
    },
    {
      component: 'Input',
      fieldName: 'partnerCode',
      label: $t('page.asn.partnerCode'),
      componentProps: {
        placeholder: $t('ui.placeholder.input'),
        allowClear: true,
      },
    },
  ],
};

const gridOptions: VxeGridProps<Asn> = {
  toolbarConfig: {
    custom: true,
    refresh: true,
    zoom: true,
  },
  height: 'auto',
  pagerConfig: {},
  rowConfig: {
    isHover: true,
  },

  proxyConfig: {
    ajax: {
      query: async ({ page }, formValues) => {
        return await fetchListAsns(
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
    { title: $t('page.asn.asnNumber'), field: 'asnNumber', width: 170 },
    { title: $t('page.asn.warehouseCode'), field: 'warehouseCode', width: 110 },
    { title: $t('page.asn.partnerCode'), field: 'partnerCode', width: 110 },
    {
      title: $t('page.asn.status'),
      field: 'status',
      slots: { default: 'status' },
      width: 100,
    },
    {
      title: $t('page.asn.expectedDate'),
      field: 'expectedDate',
      formatter: 'formatDateTime',
      width: 150,
    },
    { title: $t('ui.table.remark'), field: 'remark' },
    {
      title: $t('ui.table.createdAt'),
      field: 'createdAt',
      formatter: 'formatDateTime',
      width: 150,
    },
    {
      title: $t('ui.table.action'),
      field: 'action',
      fixed: 'right',
      slots: { default: 'action' },
      width: 300,
    },
  ],
};

const [Grid, gridApi] = useVbenVxeGrid({ gridOptions, formOptions });

const [Drawer, drawerApi] = useVbenDrawer({
  connectedComponent: AsnDrawer,
  onOpenChange(isOpen: boolean) {
    if (!isOpen) {
      gridApi.reload();
    }
  },
});

const [ReceiveModal, receiveModalApi] = useVbenModal({
  connectedComponent: AsnReceiveModal,
  onOpenChange(isOpen: boolean) {
    if (!isOpen) {
      gridApi.reload();
    }
  },
});

const [PutawayModal, putawayModalApi] = useVbenModal({
  connectedComponent: AsnPutawayModal,
  onOpenChange(isOpen: boolean) {
    if (!isOpen) {
      gridApi.reload();
    }
  },
});

const acting = ref(false);

function openModal(create: boolean, row?: any) {
  drawerApi.setData({ create, row });
  drawerApi.open();
}

function handleCreate() {
  openModal(true);
}

function handleEdit(row: Asn) {
  openModal(false, row);
}

async function runAction(
  action: () => Promise<any>,
  successKey: string,
) {
  if (acting.value) return;
  acting.value = true;
  try {
    await action();
    notification.success({ message: $t(successKey) });
    await gridApi.reload();
  } catch (e: any) {
    notification.error({
      message: $t('ui.notification.action_failed'),
      description: e?.response?.data?.message ?? e?.message,
    });
  } finally {
    acting.value = false;
  }
}

function handleAnnounce(row: Asn) {
  runAction(() => apiClient.asnService.Announce({ id: row.id! }), 'ui.notification.update_status_success');
}

function handleCancel(row: Asn) {
  runAction(() => apiClient.asnService.Cancel({ id: row.id! }), 'ui.notification.update_status_success');
}

function handleClose(row: Asn) {
  runAction(() => apiClient.asnService.Close({ id: row.id! }), 'ui.notification.update_status_success');
}

function handleReceive(row: Asn) {
  receiveModalApi.setData({ asn: row });
  receiveModalApi.open();
}

function handlePutaway(row: Asn) {
  putawayModalApi.setData({ asn: row });
  putawayModalApi.open();
}

async function handleDelete(row: Asn) {
  try {
    await apiClient.asnService.Delete({ id: row.id! });
    notification.success({
      message: $t('ui.notification.delete_success'),
    });
    await gridApi.reload();
  } catch {
    notification.error({
      message: $t('ui.notification.delete_failed'),
    });
  }
}

function confirmDelete(row: Asn) {
  Modal.confirm({
    title: $t('ui.text.do_you_want_delete', {
      moduleName: $t('page.asn.moduleName'),
    }),
    okText: $t('ui.button.ok'),
    cancelText: $t('ui.button.cancel'),
    onOk: () => handleDelete(row),
  });
}
</script>

<template>
  <Page auto-content-height>
    <Grid :table-title="$t('menu.wms.asn')">
      <template #toolbar-tools>
        <a-button class="mr-2" type="primary" @click="handleCreate">
          {{ $t('page.asn.button.create') }}
        </a-button>
      </template>

      <template #status="{ row }">
        <Tag :color="statusOf(row).color">
          {{ $t(statusOf(row).labelKey) }}
        </Tag>
      </template>

      <template #action="{ row }">
        <a-button
          v-if="row.status === 'ASN_DRAFT'"
          type="link"
          size="small"
          @click.stop="handleAnnounce(row)"
        >
          {{ $t('page.asn.action.announce') }}
        </a-button>
        <a-button
          v-if="row.status === 'ASN_ANNOUNCED' || row.status === 'ASN_RECEIVING'"
          type="link"
          size="small"
          @click.stop="handleReceive(row)"
        >
          {{ $t('page.asn.action.receive') }}
        </a-button>
        <a-button
          v-if="row.status === 'ASN_RECEIVING'"
          type="link"
          size="small"
          @click.stop="handlePutaway(row)"
        >
          {{ $t('page.asn.action.putaway') }}
        </a-button>
        <a-button
          v-if="row.status === 'ASN_ANNOUNCED' || row.status === 'ASN_RECEIVING'"
          type="link"
          size="small"
          @click.stop="handleClose(row)"
        >
          {{ $t('page.asn.action.close') }}
        </a-button>
        <a-button
          v-if="row.status === 'ASN_DRAFT'"
          type="link"
          :icon="h(LucideFilePenLine)"
          @click.stop="handleEdit(row)"
        />
        <a-button
          v-if="row.status === 'ASN_DRAFT' || row.status === 'ASN_CANCELLED'"
          danger
          type="link"
          :icon="h(LucideTrash2)"
          @click.stop="confirmDelete(row)"
        />
        <a-button
          v-if="row.status === 'ASN_ANNOUNCED' || row.status === 'ASN_RECEIVING'"
          danger
          type="link"
          size="small"
          @click.stop="handleCancel(row)"
        >
          {{ $t('page.asn.action.cancel') }}
        </a-button>
      </template>
    </Grid>
    <Drawer />
    <ReceiveModal />
    <PutawayModal />
  </Page>
</template>
