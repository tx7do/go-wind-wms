<script lang="ts" setup>
import { computed, reactive, ref } from 'vue';

import { useVbenModal } from '@vben/common-ui';
import { $t } from '@vben/locales';

import { notification } from 'ant-design-vue';

import {
  apiClient,
  type asnservicev1_Asn as Asn,
  type inventoryservicev1_StockLocation,
} from '#/api';

const asn = ref<Asn>();
const locations = ref<inventoryservicev1_StockLocation[]>([]);
const locationsLoading = ref(false);

const getTitle = computed(() =>
  $t('page.asn.putaway.title', { number: asn.value?.asnNumber ?? '' }),
);

interface PutawayLine {
  asnLineId: number;
  skuCode: string;
  receivedQty: number;
  putawayQty: number;
  qcStatus?: string;
  quantity: number;
  targetLocationId?: number;
}

const lines = reactive<PutawayLine[]>([]);

const internalLocations = computed(() =>
  locations.value.filter((l) => l.usage === 'INTERNAL'),
);

const remainingQty = (line: PutawayLine) =>
  Math.max(0, line.receivedQty - line.putawayQty);

const [Modal, modalApi] = useVbenModal({
  onConfirm() {
    handlePutaway();
  },

  onOpenChange(isOpen: boolean) {
    if (isOpen) {
      const data = modalApi.getData<{ asn: Asn }>();
      asn.value = data.asn;

      lines.splice(0, lines.length);
      for (const line of data.asn?.lines ?? []) {
        lines.push({
          asnLineId: line.id ?? 0,
          skuCode: line.skuCode ?? '',
          receivedQty: line.receivedQty ?? 0,
          putawayQty: line.putawayQty ?? 0,
          qcStatus: line.qcStatus,
          quantity: 0,
        });
      }

      loadLocations();
    }
  },
});

async function loadLocations() {
  locationsLoading.value = true;
  try {
    const resp = await apiClient.locationService.List({
      paging: { pageNo: 1, pageSize: 500 },
    } as any);
    locations.value = resp.items ?? [];
  } catch {
    locations.value = [];
  } finally {
    locationsLoading.value = false;
  }
}

async function handlePutaway() {
  const payload = lines
    .filter((l) => l.quantity > 0)
    .map((l) => ({
      asnLineId: l.asnLineId,
      quantity: l.quantity,
      targetLocationId: l.targetLocationId,
    }));

  if (payload.length === 0) {
    notification.warning({ message: $t('page.asn.putaway.nothingToPutaway') });
    return;
  }
  if (payload.some((l) => !l.targetLocationId)) {
    notification.warning({ message: $t('page.asn.putaway.locationRequired') });
    return;
  }
  for (const l of lines) {
    if (l.quantity > 0 && l.quantity > remainingQty(l)) {
      notification.warning({ message: $t('page.asn.putaway.overQty') });
      return;
    }
    if (l.quantity > 0 && l.qcStatus !== 'QC_PASS') {
      notification.warning({ message: $t('page.asn.putaway.qcBlocked') });
      return;
    }
  }

  modalApi.setState({ confirmLoading: true });

  try {
    await apiClient.asnService.Putaway({
      asnId: asn.value!.id!,
      lines: payload,
    });
    notification.success({ message: $t('page.asn.putaway.success') });
    modalApi.close();
  } catch (e: any) {
    notification.error({
      message: $t('page.asn.putaway.failed'),
      description: e?.response?.data?.message ?? e?.message,
    });
  } finally {
    modalApi.setState({ confirmLoading: false });
  }
}
</script>

<template>
  <Modal :title="getTitle" class="w-[720px]">
    <div
      v-for="line in lines"
      :key="line.asnLineId"
      class="mb-3 rounded-md border p-3"
      :class="{ 'opacity-50': line.qcStatus !== 'QC_PASS' }"
    >
      <div class="mb-2 flex items-center justify-between">
        <span class="font-medium">{{ line.skuCode }}</span>
        <span class="text-muted-foreground text-xs">
          {{ $t('page.asn.line.received') }} {{ line.receivedQty }} ·
          {{ $t('page.asn.line.putaway') }} {{ line.putawayQty }}
        </span>
      </div>

      <div class="flex items-center gap-2">
        <span class="text-sm whitespace-nowrap">
          {{ $t('page.asn.putaway.qty') }}
        </span>
        <a-input-number
          v-model:value="line.quantity"
          :min="0"
          :max="remainingQty(line)"
          :disabled="line.qcStatus !== 'QC_PASS'"
          class="w-32"
        />
        <a-select
          v-model:value="line.targetLocationId"
          :placeholder="$t('page.asn.putaway.targetLocation')"
          :options="internalLocations.map((loc) => ({
            label: loc.name ?? String(loc.id),
            value: loc.id,
          }))"
          :loading="locationsLoading"
          allowClear
          showSearch
          optionFilterProp="label"
          class="flex-1"
        />
      </div>

      <div class="text-muted-foreground mt-1 text-xs">
        {{ $t('page.asn.receive.remaining') }}: {{ remainingQty(line) }}
        <span v-if="line.qcStatus !== 'QC_PASS'">
          · {{ $t('page.asn.putaway.qcBlockedHint') }}
        </span>
      </div>
    </div>

    <a-empty
      v-if="lines.length === 0"
      :description="$t('page.asn.putaway.nothingToPutaway')"
    />
  </Modal>
</template>
