<script lang="ts" setup>
import { computed, reactive } from 'vue';

import { useVbenDrawer } from '@vben/common-ui';
import { $t } from '@vben/locales';

import { LucidePlus, LucideTrash2 } from '@vben/icons';

import { notification } from 'ant-design-vue';

import { useVbenForm } from '#/adapter/form';
import { useCreateStockPicking } from '#/api';

const getTitle = computed(() => $t('page.stockPicking.outgoing.title'));

// 出库明细行（客户端提供 product + planned_quantity；FEFO 扣减在 Validate 执行）
const lines = reactive<Array<{ productCode: string; plannedQuantity: number }>>(
  [],
);

function addLine() {
  lines.push({ productCode: '', plannedQuantity: 1 });
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
      component: 'ApiSelect',
      fieldName: 'fromWarehouseCode',
      label: $t('page.stockPicking.outgoing.fromWarehouse'),
      rules: 'required',
      componentProps: {
        allowClear: true,
        showSearch: true,
        placeholder: $t('ui.placeholder.select'),
        api: async () => {
          const { fetchListWarehouses, PaginationQuery } = await import('#/api');
          const result = await fetchListWarehouses(
            // biome-ignore lint/style/noNonNullAssertion: 仓库下拉不分页
            new PaginationQuery({
              paging: { page: 1, pageSize: 500 },
            }),
          );
          return result?.items ?? [];
        },
        afterFetch: (warehouses: { code?: string; enable?: boolean }[]) =>
          warehouses
            .filter((w) => w.enable)
            .map((w) => ({ label: w.code, value: w.code })),
      },
    },
    {
      component: 'Input',
      fieldName: 'partnerCode',
      label: $t('page.stockPicking.outgoing.partnerCode'),
      rules: 'required',
      componentProps: {
        placeholder: $t('ui.placeholder.input'),
        allowClear: true,
      },
    },
    {
      component: 'Textarea',
      fieldName: 'remark',
      label: $t('page.stockPicking.outgoing.remark'),
      componentProps: {
        placeholder: $t('ui.placeholder.input'),
        allowClear: true,
      },
    },
  ],
});

const createMutation = useCreateStockPicking();

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
      notification.warning({ message: $t('page.stockPicking.outgoing.linesRequired') });
      return;
    }
    for (const line of lines) {
      if (!line.productCode || line.plannedQuantity <= 0) {
        notification.warning({
          message: $t('page.stockPicking.outgoing.lineInvalid'),
        });
        return;
      }
    }

    setLoading(true);

    const values = await baseFormApi.getValues();

    try {
      await createMutation.mutateAsync({
        pickingType: 'OUTGOING',
        fromWarehouseCode: values.fromWarehouseCode,
        partnerCode: values.partnerCode,
        moves: lines.map((l) => ({ ...l })),
        remark: values.remark,
      });

      notification.success({
        message: $t('ui.notification.create_success'),
      });
    } catch (e: any) {
      notification.error({
        message: $t('ui.notification.create_failed'),
        description: e?.response?.data?.message ?? e?.message,
      });
    } finally {
      drawerApi.close();
      setLoading(false);
    }
  },

  onOpenChange(isOpen: boolean) {
    if (isOpen) {
      lines.splice(0, lines.length);
      baseFormApi.setValues({});
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
        <span class="font-medium">
          {{ $t('page.stockPicking.outgoing.lines') }}
        </span>
        <a-button size="small" type="primary" @click="addLine">
          <template #icon><LucidePlus class="size-4" /></template>
          {{ $t('page.stockPicking.outgoing.addLine') }}
        </a-button>
      </div>

      <div
        v-for="(line, index) in lines"
        :key="index"
        class="mb-2 flex items-center gap-2"
      >
        <a-input
          v-model:value="line.productCode"
          :placeholder="$t('page.stockPicking.productCode')"
          class="flex-1"
        />
        <a-input-number
          v-model:value="line.plannedQuantity"
          :min="1"
          :placeholder="$t('page.stockPicking.plannedQuantity')"
          class="w-28"
        />
        <a-button danger size="small" @click="removeLine(index)">
          <template #icon><LucideTrash2 class="size-4" /></template>
        </a-button>
      </div>

      <a-empty
        v-if="lines.length === 0"
        :description="$t('page.stockPicking.outgoing.linesRequired')"
      />
    </div>
  </Drawer>
</template>
