<template>
  <div class="page-wrap records-page">
    <header class="page-header records-header">
      <div class="records-heading"><h1>操作审计</h1><span>{{ formatNumber(total) }} 条记录</span></div>
      <v-btn icon="mdi-refresh" variant="text" :loading="loading" title="刷新" @click="load" />
    </header>
    <v-alert v-if="error" type="error" variant="tonal" class="mb-4">{{ error }}</v-alert>
    <section class="data-surface records-surface">
      <v-data-table-server
        class="records-table"
        :headers="headers"
        :items="audits"
        :loading="loading"
        :items-length="total"
        :items-per-page="itemsPerPage"
        :page="page"
        :items-per-page-options="pageSizeOptions"
        hover
        density="compact"
        @update:page="page = $event; load()"
        @update:items-per-page="itemsPerPage = $event; page = 1; load()"
      >
        <template #item.status="{ item }">
          <v-chip size="x-small" :color="item.status === 'executed' ? 'success' : item.status === 'failed' ? 'error' : 'grey'" variant="tonal">{{ statusLabel(item.status) }}</v-chip>
        </template>
        <template #item.endpoint="{ item }"><code class="endpoint-code">{{ item.endpoint || item.request?.endpoint || '—' }}</code></template>
        <template #item.parameters="{ item }"><code class="operation-json">{{ compactJSON(item.parameters || item.request?.parameters) }}</code></template>
        <template #item.created_at="{ item }">{{ formatDate(item.created_at) }}</template>
        <template #item.actions="{ item }">
          <v-btn icon="mdi-text-box-search-outline" size="small" variant="text" title="查看详情" @click="selected = item; dialog = true" />
        </template>
        <template #no-data>
          <div class="empty-table"><v-icon icon="mdi-history" /><strong>暂无审计记录</strong></div>
        </template>
      </v-data-table-server>
    </section>

    <v-dialog v-model="dialog" max-width="900">
      <v-card>
        <v-card-title class="d-flex pa-5">
          审计详情
          <v-spacer />
          <v-btn icon="mdi-close" variant="text" @click="dialog = false" />
        </v-card-title>
        <v-divider />
        <v-card-text class="audit-dialog">
          <div>
            <h3>请求</h3>
            <pre class="json-viewer">{{ JSON.stringify(selected?.request, null, 2) }}</pre>
          </div>
          <div>
            <h3>响应</h3>
            <pre class="json-viewer">{{ JSON.stringify(selected?.response, null, 2) }}</pre>
          </div>
        </v-card-text>
      </v-card>
    </v-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { api } from "@/services/api";
import { useSystemCapabilitiesStore } from "@/stores/systemCapabilities";

const audits = ref<any[]>([]);
const systemCapabilities = useSystemCapabilitiesStore();
const loading = ref(false);
const error = ref("");
const total = ref(0);
const page = ref(1);
const itemsPerPage = ref(0);
const pageSizeOptions = computed(() => systemCapabilities.data?.lists.page_sizes || []);
const dialog = ref(false);
const selected = ref<any>(null);

const headers = [
  { title: "状态", key: "status" },
  { title: "账号", key: "account_id" },
  { title: "端点", key: "endpoint" },
  { title: "参数", key: "parameters" },
  { title: "时间", key: "created_at" },
  { title: "", key: "actions", sortable: false, width: 52 },
];

async function load() {
  loading.value = true;
  try {
    const r = (
      await api.get("/api/v1/operation-audits", {
        params: { limit: itemsPerPage.value, offset: (page.value - 1) * itemsPerPage.value },
      })
    ).data;
    audits.value = r.data || [];
    total.value = r.total || 0;
  } catch (e: any) {
    error.value = e.response?.data?.error || "审计记录加载失败";
  } finally {
    loading.value = false;
  }
}

const formatDate = (v: string) =>
  v
    ? new Intl.DateTimeFormat("zh-CN", { dateStyle: "short", timeStyle: "short" }).format(new Date(v))
    : "—";
const compactJSON = (value: unknown) => value ? JSON.stringify(value) : "—";
const formatNumber = (value: number) => new Intl.NumberFormat("zh-CN").format(value || 0);
const statusLabel = (value: string) => ({ executed: "已执行", failed: "失败", cancelled: "已取消" } as Record<string, string>)[value] || value || "未知";

onMounted(async () => {
  await systemCapabilities.load();
  itemsPerPage.value = itemsPerPage.value || systemCapabilities.data?.lists.default_page_size || 0;
  if (itemsPerPage.value) await load();
});
</script>
