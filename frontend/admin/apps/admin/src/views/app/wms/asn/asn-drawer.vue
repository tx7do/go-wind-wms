<script lang="ts" setup>
import { computed, reactive, ref } from 'vue';

import { useVbenDrawer } from '@vben/common-ui';
import { $t } from '@vben/locales';

import { LucidePlus, LucideTrash2 } from '@vben/icons';
import { notification } from 'ant-design-vue';

import { useVbenForm } from '#/adapter/form';
import { apiClient, makeUpdateMask } from '#/api';

const data = ref();

const getTitle = computed(() =>
  data.value?.create
    ? $t('ui.modal.create', { moduleName: $t('page.asn.moduleName') })
    : $t('ui.modal.update', { moduleName: $t('page.asn.moduleName') }),
);

// 明细行（前端维护，提交时整体随 data.lines 上送）
const lines = reactive<Array<{ skuCode: string; expectedQty: number; remark?: string }>>(
  [],
);

function addLine() {
  lines.push({ skuCode: '', expectedQty: 1 });
}

function removeLine(index: number) {
  lines.splice(index, 1);
}

const [BaseForm, baseFormApi] = useVbenForm({
  showDefaultActions: false,
  commonConfig: {
    componentProps: {
      class: 'w-full',
    },
  },
  schema: [
    {
      component: 'Input',
      fieldName: 'warehouseCode',
      label: $t('page.asn.warehouseCode'),
      rules: 'required',
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
    {
      component: 'DatePicker',
      fieldName: 'expectedDate',
      label: $t('page.asn.expectedDate'),
      componentProps: {
        showTime: true,
        valueFormat: 'YYYY-MM-DD HH:mm:ss',
        placeholder: $t('ui.placeholder.select'),
        class: 'w-full',
      },
    },
    {
      component: 'Textarea',
      fieldName: 'remark',
      label: $t('ui.table.remark'),
      componentProps: {
        placeholder: $t('ui.placeholder.input'),
        allowClear: true,
      },
    },
  ],
});

const [Drawer, drawerApi] = useVbenDrawer({
  onCancel() {
    drawerApi.close();
  },

  async onConfirm() {
    const validate = await baseFormApi.validate();
    if (!validate.valid) {
      return;
    }

    if (lines.length === 0) {
      notification.warning({ message: $t('page.asn.linesRequired') });
      return;
    }
    for (const line of lines) {
      if (!line.skuCode || line.expectedQty <= 0) {
        notification.warning({ message: $t('page.asn.lineInvalid') });
        return;
      }
    }

    setLoading(true);

    const values = await baseFormApi.getValues();
    // DatePicker 值转 RFC3339（proto Timestamp）
    const expectedDate = values.expectedDate
      ? new Date(values.expectedDate as string).toISOString()
      : undefined;

    try {
      if (data.value?.create) {
        await apiClient.asnService.Create({
          data: {
            ...values,
            expectedDate,
            lines: lines.map((l) => ({ ...l })),
          },
        });
      } else {
        await apiClient.asnService.Update({
          id: data.value.row.id,
          data: {
            ...values,
            expectedDate,
            lines: lines.map((l) => ({ ...l })),
          },
          updateMask: makeUpdateMask(['warehouseCode', 'partnerCode', 'expectedDate', 'remark', 'lines']),
        });
      }

      notification.success({
        message: data.value?.create
          ? $t('ui.notification.create_success')
          : $t('ui.notification.update_success'),
      });
    } catch (e: any) {
      notification.error({
        message: data.value?.create
          ? $t('ui.notification.create_failed')
          : $t('ui.notification.update_failed'),
        description: e?.response?.data?.message ?? e?.message,
      });
    } finally {
      drawerApi.close();
      setLoading(false);
    }
  },

  onOpenChange(isOpen: boolean) {
    if (isOpen) {
      data.value = drawerApi.getData<Record<string, any>>();

      lines.splice(0, lines.length);
      const row = data.value?.row;
      if (row) {
        baseFormApi.setValues({
          warehouseCode: row.warehouseCode,
          partnerCode: row.partnerCode,
          expectedDate: row.expectedDate,
          remark: row.remark,
        });
        for (const line of row.lines ?? []) {
          lines.push({
            skuCode: line.skuCode ?? '',
            expectedQty: line.expectedQty ?? 0,
            remark: line.remark,
          });
        }
      } else {
        baseFormApi.setValues({});
      }

      setLoading(false);
    }
  },
});

function setLoading(loading: boolean) {
  drawerApi.setState({ confirmLoading: loading });
}
</script>

<template>
  <Drawer :title="getTitle" class="w-[640px]">
    <BaseForm />

    <div class="mt-2">
      <div class="mb-2 flex items-center justify-between">
        <span class="font-medium">{{ $t('page.asn.lines') }}</span>
        <a-button size="small" type="primary" @click="addLine">
          <template #icon><LucidePlus class="size-4" /></template>
          {{ $t('page.asn.button.addLine') }}
        </a-button>
      </div>

      <div
        v-for="(line, index) in lines"
        :key="index"
        class="mb-2 flex items-center gap-2"
      >
        <a-input
          v-model:value="line.skuCode"
          :placeholder="$t('page.asn.skuCode')"
          class="flex-1"
        />
        <a-input-number
          v-model:value="line.expectedQty"
          :min="1"
          :placeholder="$t('page.asn.expectedQty')"
          class="w-28"
        />
        <a-button danger size="small" @click="removeLine(index)">
          <template #icon><LucideTrash2 class="size-4" /></template>
        </a-button>
      </div>

      <a-empty
        v-if="lines.length === 0"
        :description="$t('page.asn.linesRequired')"
      />
    </div>
  </Drawer>
</template>
