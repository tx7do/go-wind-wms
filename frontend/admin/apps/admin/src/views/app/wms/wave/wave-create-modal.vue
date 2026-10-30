<script lang="ts" setup>
import { computed, reactive, ref } from 'vue';

import { useVbenModal } from '@vben/common-ui';
import { $t } from '@vben/locales';

import { notification } from 'ant-design-vue';

import {
  apiClient,
  fetchListStockPickings,
  PaginationQuery,
  type inventoryservicev1_StockPicking as StockPicking,
} from '#/api';

const getTitle = computed(() => $t('page.wave.button.create'));

// 可入波次的候选单：OUTGOING + CONFIRMED + 未入波次（waveId 为空或 0）
const candidates = ref<StockPicking[]>([]);
const loading = ref(false);
const selectedIds = reactive<Set<number>>(new Set());

const [Modal, modalApi] = useVbenModal({
  onConfirm() {
    handleCreate();
  },
  onOpenChange(isOpen: boolean) {
    if (isOpen) {
      selectedIds.clear();
      loadCandidates();
    }
  },
});

async function loadCandidates() {
  loading.value = true;
  try {
    const resp = await fetchListStockPickings(
      new PaginationQuery({
        paging: { page: 1, pageSize: 100 },
      }),
    );
    candidates.value = (resp?.items ?? []).filter(
      (p) =>
        p.pickingType === 'OUTGOING' &&
        p.derivedState === 'CONFIRMED' &&
        !p.waveId,
    );
  } catch {
    candidates.value = [];
  } finally {
    loading.value = false;
  }
}

function toggle(id: number, checked: boolean) {
  if (checked) {
    selectedIds.add(id);
  } else {
    selectedIds.delete(id);
  }
}

async function handleCreate() {
  if (selectedIds.size === 0) {
    notification.warning({ message: $t('page.wave.noneSelected') });
    return;
  }

  modalApi.setState({ confirmLoading: true });

  try {
    await apiClient.waveService.Create({
      pickingIds: [...selectedIds],
    });
    notification.success({ message: $t('ui.notification.create_success') });
    modalApi.close();
  } catch (e: any) {
    notification.error({
      message: $t('ui.notification.create_failed'),
      description: e?.response?.data?.message ?? e?.message,
    });
  } finally {
    modalApi.setState({ confirmLoading: false });
  }
}
</script>

<template>
  <Modal :title="getTitle" class="w-[720px]">
    <a-spin :spinning="loading">
      <div
        v-for="p in candidates"
        :key="p.id"
        class="mb-2 flex items-center gap-3 rounded-md border p-3"
      >
        <a-checkbox
          :checked="selectedIds.has(p.id!)"
          @change="(e: any) => toggle(p.id!, e.target.checked)"
        />
        <div class="flex-1">
          <div class="font-medium">{{ p.pickingNumber }}</div>
          <div class="text-muted-foreground text-xs">
            {{ p.partnerCode }}
            <template v-for="m in p.moves ?? []" :key="m.productCode">
              · {{ m.productCode }} × {{ m.plannedQuantity }}
            </template>
          </div>
        </div>
      </div>

      <a-empty
        v-if="candidates.length === 0 && !loading"
        :description="$t('page.wave.noCandidates')"
      />
    </a-spin>
  </Modal>
</template>
