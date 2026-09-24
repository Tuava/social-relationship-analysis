<template>
  <div class="page-wrap records-page">
    <header class="page-header records-header">
      <div class="records-heading">
        <h1>操作中心</h1>
        <span>{{ items.length }} 条记录 · {{ pendingCount }} 条待确认</span>
      </div>
      <div class="records-header-actions">
        <v-btn icon="mdi-refresh" variant="text" :loading="loading" title="刷新" @click="load" />
        <v-btn color="primary" prepend-icon="mdi-plus" @click="dialog = true">创建预览</v-btn>
      </div>
    </header>
    <v-alert v-if="error" type="error" variant="tonal" class="mb-4">{{
      error
    }}</v-alert>
    <section class="data-surface records-surface"
      ><v-data-table class="records-table" density="compact" :headers="headers" :items="items" :loading="loading"
        ><template #item.endpoint="{ item }"><code class="endpoint-code">{{ item.endpoint }}</code></template
        ><template #item.status="{ item }"
          ><v-chip
            size="x-small"
            :color="
              item.status === 'pending'
                ? 'warning'
                : item.status === 'executed'
                  ? 'success'
                  : 'grey'
            "
            variant="tonal"
            >{{ statusLabel(item.status) }}</v-chip
          ></template
        ><template #item.parameters="{ item }"
          ><code class="text-caption operation-json">{{
            JSON.stringify(item.parameters)
          }}</code></template
        ><template #item.actions="{ item }"
          ><div class="row-actions"><v-btn
            v-if="item.status === 'pending'"
            icon="mdi-check"
            size="small"
            color="primary"
            variant="text"
            title="确认执行"
            @click="confirm(item)"
          /><v-btn
            v-if="item.status === 'pending'"
            icon="mdi-close"
            size="small"
            variant="text"
            title="取消"
            @click="cancel(item)"
          /></div></template
        ><template #item.created_at="{ item }"><span class="table-time">{{ formatDate(item.created_at) }}</span></template
        ><template #no-data
          ><div class="empty-table"><v-icon icon="mdi-shield-check-outline" /><strong>暂无操作记录</strong><span>创建预览后，确认状态会显示在这里。</span></div></template
        ></v-data-table
      ></section
    >
    <v-dialog v-model="dialog" max-width="680"
      ><v-card
        ><v-card-title class="pa-6">创建操作预览</v-card-title
        ><v-card-text class="px-6"
          ><v-select
            v-model="form.account_id"
            :items="accounts"
            item-title="name"
            item-value="id"
            label="NapCat 账号"
          /><v-autocomplete
            v-model="form.endpoint"
            :items="writeCapabilities"
            item-title="label"
            item-value="endpoint"
            label="副作用接口"
            class="mt-3"
          /><v-textarea
            v-model="parametersText"
            label="参数 JSON"
            rows="8"
            class="mt-3"
            spellcheck="false"
          /><v-alert type="warning" variant="tonal" density="compact"
            >这里只创建预览，不会立即调用
            NapCat。确认执行仍需在列表中再次确认。</v-alert
          ></v-card-text
        ><v-card-actions class="pa-6 pt-0"
          ><v-spacer /><v-btn variant="text" @click="dialog = false">取消</v-btn
          ><v-btn
            color="primary"
            :loading="saving"
            :disabled="!form.account_id || !form.endpoint"
            @click="preview"
            >保存预览</v-btn
          ></v-card-actions
        ></v-card
      ></v-dialog
    >
  </div>
</template>
<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import { api } from "@/services/api";
import type { Account, Capability } from "@/types/api";
const items = ref<any[]>([]);
const accounts = ref<Account[]>([]);
const capabilities = ref<Capability[]>([]);
const loading = ref(false);
const saving = ref(false);
const dialog = ref(false);
const error = ref("");
const parametersText = ref("{}");
const form = reactive({ account_id: "", endpoint: "" });
const headers = [
  { title: "接口", key: "endpoint" },
  { title: "参数", key: "parameters" },
  { title: "状态", key: "status" },
  { title: "创建时间", key: "created_at" },
  { title: "操作", key: "actions", sortable: false },
];
const writeCapabilities = computed(() =>
  capabilities.value
    .filter((c) => !c.read_only)
    .map((c) => ({
      ...c,
      label: `${c.endpoint} · ${c.summary || "未命名接口"}`,
    })),
);
const pendingCount = computed(() => items.value.filter((item) => item.status === "pending").length);
const statusLabel = (value: string) => ({ pending: "待确认", executed: "已执行", cancelled: "已取消", failed: "失败" } as Record<string, string>)[value] || value || "未知";
const formatDate = (value?: string) => value ? new Intl.DateTimeFormat("zh-CN", { dateStyle: "short", timeStyle: "short" }).format(new Date(value)) : "—";
async function load() {
  loading.value = true;
  try {
    const [ops, acc, caps] = await Promise.all([
      api.get("/api/v1/operations"),
      api.get("/api/v1/accounts"),
      api.get("/api/v1/napcat/capabilities"),
    ]);
    items.value = ops.data.data;
    accounts.value = acc.data.data;
    capabilities.value = caps.data.data;
  } catch (e: any) {
    error.value = e.response?.data?.error || "无法加载操作";
  } finally {
    loading.value = false;
  }
}
async function preview() {
  error.value = "";
  let parameters: any;
  try {
    parameters = JSON.parse(parametersText.value);
  } catch {
    error.value = "参数必须是有效 JSON";
    return;
  }
  saving.value = true;
  try {
    await api.post("/api/v1/operations/preview", { ...form, parameters });
    dialog.value = false;
    parametersText.value = "{}";
    form.endpoint = "";
    await load();
  } catch (e: any) {
    error.value = e.response?.data?.error || "创建预览失败";
  } finally {
    saving.value = false;
  }
}
async function confirm(item: any) {
  if (!window.confirm(`确认执行 ${item.endpoint}？此操作可能修改 QQ 数据。`))
    return;
  try {
    await api.post(`/api/v1/operations/${item.id}/confirm`);
    await load();
  } catch (e: any) {
    error.value = e.response?.data?.error || "执行失败";
  }
}
async function cancel(item: any) {
  try {
    await api.post(`/api/v1/operations/${item.id}/cancel`);
    await load();
  } catch (e: any) {
    error.value = e.response?.data?.error || "取消失败";
  }
}
onMounted(load);
</script>
