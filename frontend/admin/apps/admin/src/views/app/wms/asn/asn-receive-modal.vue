<script lang="ts" setup>
import { computed, reactive, ref } from 'vue';

import { useVbenModal } from '@vben/common-ui';
import { $t } from '@vben/locales';

import { notification } from 'ant-design-vue';

import { apiClient, type asnservicev1_Asn as Asn } from '#/api';

const asn = ref<Asn>();

const getTitle = computed(() =>
  $t('page.asn.receive.title', { number: asn.value?.asnNumber ?? '' }),
);

interface ReceiveLine {
  asnLineId: number;
  skuCode: string;
  expectedQty: number;
  receivedQty: number;
  quantity: number;
  qcStatus: 'QC_PASS' | 'QC_FAIL';
  lotName?: string;
}

const lines = reactive<ReceiveLine[]>([]);

const remainingQty = (line: ReceiveLine) =>
  Math.max(0, line.expectedQty - line.receivedQty);

const [Modal, modalApi] = useVbenModal({
  onConfirm() {
    handleReceive();
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
          expectedQty: line.expectedQty ?? 0,
          receivedQty: line.receivedQty ?? 0,
          quantity: 0,
          qcStatus: 'QC_PASS',
        });
      }
    }
  },
});

async function handleReceive() {
  const payload = lines
    .filter((l) => l.quantity > 0)
    .map((l) => ({
      asnLineId: l.asnLineId,
      quantity: l.quantity,
      qcStatus: l.qcStatus,
      lotName: l.lotName || undefined,
    }));

  if (payload.length === 0) {
    notification.warning({ message: $t('page.asn.receive.nothingToReceive') });
    return;
  }

  for (const l of lines) {
    if (l.quantity > 0 && l.quantity > remainingQty(l)) {
      notification.warning({ message: $t('page.asn.receive.overQty') });
      return;
    }
  }

  modalApi.setState({ confirmLoading: true });

  try {
    await apiClient.asnService.Receive({
      asnId: asn.value!.id!,
      lines: payload,
    });
    notification.success({ message: $t('page.asn.receive.success') });
    modalApi.close();
  } catch (e: any) {
    notification.error({
      message: $t('page.asn.receive.failed'),
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
    >
      <div class="mb-2 flex items-center justify-between">
        <span class="font-medium">{{ line.skuCode }}</span>
        <span class="text-muted-foreground text-xs">
          {{ $t('page.asn.line.expected') }} {{ line.expectedQty }} ·
          {{ $t('page.asn.line.received') }} {{ line.receivedQty }}
        </span>
      </div>

      <div class="flex items-center gap-2">
        <span class="text-sm whitespace-nowrap">
          {{ $t('page.asn.receive.qty') }}
        </span>
        <a-input-number
          v-model:value="line.quantity"
          :min="0"
          :max="remainingQty(line)"
          class="w-32"
        />
        <a-radio-group v-model:value="line.qcStatus" size="small">
          <a-radio-button value="QC_PASS">
            {{ $t('page.asn.qc.pass') }}
          </a-radio-button>
          <a-radio-button value="QC_FAIL">
            {{ $t('page.asn.qc.fail') }}
          </a-radio-button>
        </a-radio-group>
        <a-input
          v-model:value="line.lotName"
          :placeholder="$t('page.asn.line.lotName')"
          class="w-36"
        />
      </div>

      <div class="text-muted-foreground mt-1 text-xs">
        {{ $t('page.asn.receive.remaining') }}: {{ remainingQty(line) }}
      </div>
    </div>

    <a-empty
      v-if="lines.length === 0"
      :description="$t('page.asn.receive.nothingToReceive')"
    />
  </Modal>
</template>
