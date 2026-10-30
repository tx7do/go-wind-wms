<script lang="ts" setup>
import type { VxeGridProps } from '#/adapter/vxe-table';

import { h, ref } from 'vue';

import { Page, useVbenModal, type VbenFormProps } from '@vben/common-ui';
import { LucidePlay, LucideTrash2 } from '@vben/icons';

import { Modal, notification, Tag } from 'ant-design-vue';

import { useVbenVxeGrid } from '#/adapter/vxe-table';
import {
  apiClient,
  fetchListWaves,
  PaginationQuery,
  type waveservicev1_Wave as Wave,
} from '#/api';
import { $t } from '#/locales';

import WaveCreateModal from './wave-create-modal.vue';

const WAVE_STATUS = {
  WAVE_DRAFT: { color: 'default', labelKey: 'page.wave.status.draft' },
  WAVE_RUNNING: { color: 'processing', labelKey: 'page.wave.status.running' },
  WAVE_COMPLETED: { color: 'success', labelKey: 'page.wave.status.completed' },
  WAVE_CANCELLED: { color: 'error', labelKey: 'page.wave.status.cancelled' },
} as const;

function statusOf(row: Wave) {
  return WAVE_STATUS[(row.status ?? 'WAVE_DRAFT') as keyof typeof WAVE_STATUS] ??
    WAVE_STATUS.WAVE_DRAFT;
}

const formOptions: VbenFormProps = {
  collapsed: false,
  showCollapseButton: false,
  submitOnEnter: true,
  schema: [
    {
      component: 'Input',
      fieldName: 'waveNumber',
      label: $t('page.wave.waveNumber'),
      componentProps: {
        placeholder: $t('ui.placeholder.input'),
        allowClear: true,
      },
    },
  ],
};

const gridOptions: VxeGridProps<Wave> = {
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
        return await fetchListWaves(
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
    { title: $t('page.wave.waveNumber'), field: 'waveNumber', width: 180 },
    {
      title: $t('page.wave.status'),
      field: 'status',
      slots: { default: 'status' },
      width: 100,
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
      width: 160,
    },
  ],
};

const [Grid, gridApi] = useVbenVxeGrid({ gridOptions, formOptions });

const [CreateModal, createModalApi] = useVbenModal({
  connectedComponent: WaveCreateModal,
  onOpenChange(isOpen: boolean) {
    if (!isOpen) {
      gridApi.reload();
    }
  },
});

const acting = ref(false);

function handleCreate() {
  createModalApi.open();
}

async function runAction(action: () => Promise<any>, successKey: string) {
  if (acting.value) return;
  acting.value = true;
  try {
    const result = await action();
    notification.success({
      message: $t(successKey),
      description:
        result?.succeeded !== undefined
          ? `${$t('page.wave.runSucceeded')}: ${result.succeeded}, ${$t('page.wave.runFailed')}: ${result.failed}`
          : undefined,
    });
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

function handleRun(row: Wave) {
  runAction(
    () => apiClient.waveService.Run({ id: row.id! }),
    'page.wave.action.runSuccess',
  );
}

function handleCancel(row: Wave) {
  runAction(
    () => apiClient.waveService.Cancel({ id: row.id! }),
    'ui.notification.update_status_success',
  );
}

function confirmCancel(row: Wave) {
  Modal.confirm({
    title: $t('page.wave.cancelConfirm'),
    okText: $t('ui.button.ok'),
    cancelText: $t('ui.button.cancel'),
    onOk: () => handleCancel(row),
  });
}

</script>

<template>
  <Page auto-content-height>
    <Grid :table-title="$t('menu.wms.wave')">
      <template #toolbar-tools>
        <a-button class="mr-2" type="primary" @click="handleCreate">
          {{ $t('page.wave.button.create') }}
        </a-button>
      </template>

      <template #status="{ row }">
        <Tag :color="statusOf(row).color">
          {{ $t(statusOf(row).labelKey) }}
        </Tag>
      </template>

      <template #action="{ row }">
        <a-button
          v-if="row.status === 'WAVE_DRAFT' || row.status === 'WAVE_RUNNING'"
          type="link"
          size="small"
          :icon="h(LucidePlay)"
          @click.stop="handleRun(row)"
        >
          {{ $t('page.wave.action.run') }}
        </a-button>
        <a-button
          v-if="row.status === 'WAVE_DRAFT'"
          danger
          type="link"
          size="small"
          :icon="h(LucideTrash2)"
          @click.stop="confirmCancel(row)"
        >
          {{ $t('page.wave.action.cancel') }}
        </a-button>
      </template>
    </Grid>
    <CreateModal />
  </Page>
</template>
