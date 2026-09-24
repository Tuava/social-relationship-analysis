<template>
  <div class="page-wrap resource-page records-page">
    <header class="page-header records-header">
      <div class="records-heading"><h1>资源下载</h1><span>{{ formatNumber(totalReferences) }} 条下载记录</span></div>
      <div class="resource-actions">
        <v-btn
          :prepend-icon="settings.paused ? 'mdi-play' : 'mdi-pause'"
          variant="tonal"
          @click="setPaused(!settings.paused)"
          >{{ settings.paused ? "继续下载" : "暂停下载" }}</v-btn
        >
        <v-btn
          icon="mdi-refresh"
          variant="tonal"
          :loading="loading"
          title="刷新"
          @click="load"
        />
      </div>
    </header>

    <section class="resource-control-bar data-surface">
      <div class="resource-control-stat">
        <span>下载状态</span
        ><strong :class="settings.paused ? 'text-warning' : 'text-success'">{{
          settings.paused ? "已暂停" : "运行中"
        }}</strong>
      </div>
      <div class="resource-control-stat">
        <span>并发下载</span
        ><strong>{{ settings.concurrency }} <small>/ 6</small></strong>
      </div>
      <div class="resource-control-stat">
        <span>已归档资源</span><strong>{{ formatNumber(assetCount) }}</strong>
      </div>
      <div class="resource-control-stat">
        <span>占用空间</span><strong>{{ formatBytes(totalBytes) }}</strong>
      </div>
      <div class="resource-concurrency">
        <span>并发数</span
        ><v-slider
          v-model="concurrency"
          min="1"
          max="6"
          step="1"
          hide-details
          density="compact"
          @end="saveConcurrency(concurrency)"
        />
      </div>
    </section>

    <v-alert v-if="error" type="error" variant="tonal" class="mt-3">{{
      error
    }}</v-alert>
    <section class="resource-policy-grid mt-3">
      <article
        v-for="policy in policies"
        :key="policy.kind"
        class="resource-policy data-surface"
        :class="{ 'resource-policy-primary': policy.kind === 'avatar' }"
      >
        <div class="resource-policy-head">
          <div class="resource-kind-icon" :class="`kind-${policy.kind}`">
            <v-icon :icon="kindIcon(policy.kind)" />
          </div>
          <div>
            <h2>{{ kindLabel(policy.kind) }}</h2>
          </div>
          <v-switch
            v-model="policy.enabled"
            color="primary"
            hide-details
            density="compact"
            :disabled="policy.kind === 'avatar'"
            @update:model-value="togglePolicy(policy)"
          />
        </div>
        <div class="resource-policy-numbers">
          <div>
            <span>待处理</span
            ><strong>{{ formatNumber(policy.pending) }}</strong>
          </div>
          <div>
            <span>已归档</span
            ><strong>{{ formatNumber(policy.completed) }}</strong>
          </div>
          <div>
            <span>失败</span><strong>{{ formatNumber(policy.failed) }}</strong>
          </div>
        </div>
        <div class="resource-policy-foot">
          <span>{{ formatBytes(policy.bytes) }}</span
          ><v-chip
            size="x-small"
            :color="policy.enabled ? 'success' : 'grey'"
            variant="tonal"
            >{{ policy.enabled ? "自动下载" : "按条下载" }}</v-chip
          >
        </div>
      </article>
    </section>

    <section class="data-surface records-surface resource-list mt-3">
      <div class="resource-list-toolbar">
        <div class="resource-list-title">
          <h2>下载队列</h2>
          <span>{{ formatNumber(totalReferences) }} 条</span>
        </div>
        <v-text-field
          v-model.trim="query"
          placeholder="文件名、QQ、群号或消息来源"
          prepend-inner-icon="mdi-magnify"
          clearable
          hide-details
          density="compact"
          @keyup.enter="resetAndLoad"
          @click:clear="resetAndLoad"
        />
        <v-select
          v-model="filterKind"
          :items="kindOptions"
          prepend-inner-icon="mdi-filter-variant"
          hide-details
          density="compact"
        /><v-select
          v-model="filterStatus"
          :items="statusOptions"
          prepend-inner-icon="mdi-list-status"
          hide-details
          density="compact"
        /><v-select
          v-model="filterDepth"
          :items="depthOptions"
          prepend-inner-icon="mdi-source-branch"
          hide-details
          density="compact"
        /><v-btn variant="tonal" prepend-icon="mdi-replay" @click="retryFailed"
          >重试失败</v-btn
        >
      </div>
      <v-data-table-server
        class="records-table"
        :headers="headers"
        :items="references"
        :loading="loadingRefs"
        :items-length="totalReferences"
        :items-per-page="itemsPerPage"
        :page="page"
        :items-per-page-options="pageSizeOptions"
        @update:page="
          page = $event;
          loadReferences();
        "
        @update:items-per-page="
          itemsPerPage = $event;
          page = 1;
          loadReferences();
        "
      >
        <template #item.kind="{ item }"
          ><div class="resource-name-cell">
            <AuthImage
              v-if="item.asset_url && item.mime_type?.startsWith('image/')"
              :src="item.asset_url"
              alt=""
              class="resource-thumb"
              ><v-icon :icon="kindIcon(item.kind)" size="18"
            /></AuthImage>
            <v-icon v-else :icon="kindIcon(item.kind)" size="18" />
            <div>
              <strong>{{
                item.filename || item.source_ref || kindLabel(item.kind)
              }}</strong>
              <small>{{ kindLabel(item.kind) }}</small>
            </div>
          </div></template
        >
        <template #item.person_name="{ item }"
          ><div v-if="item.person_name">
            <strong>{{ item.person_name }}</strong
            ><small>QQ {{ item.qq || "未知" }}</small>
          </div>
          <span v-else>—</span></template
        >
        <template #item.status="{ item }"
          ><v-chip
            size="x-small"
            :color="statusColor(item.status)"
            variant="tonal"
            >{{ statusLabel(item.status) }}</v-chip
          ><small v-if="item.error" class="resource-error">{{
            compactError(item.error)
          }}</small
          ><v-tooltip v-if="item.error" location="top"
            ><template #activator="{ props }"
              ><v-icon
                v-bind="props"
                icon="mdi-alert-circle-outline"
                size="16"
                class="ml-1 text-error" /></template
            ><span>{{ item.error }}</span></v-tooltip
          ></template
        >
        <template #item.priority="{ item }"
          ><div class="resource-priority-cell">
            <div>
              <v-chip size="x-small" :color="depthColor(item.relation_depth)" variant="tonal">{{ depthLabel(item.relation_depth) }}</v-chip>
              <strong>{{ priorityLabel(item) }}</strong>
            </div>
            <small>{{ reasonLabel(item.priority_reason) }}<template v-if="item.priority_boost"> · 加权 +{{ item.priority_boost }}</template></small>
          </div></template
        >
        <template #item.source="{ item }"
          ><div class="resource-source-cell">
            <span v-if="item.conversation_id"
              >{{ item.conversation_type === "group" ? "群聊" : "私聊" }} ·
              {{ item.conversation_id }}</span
            ><span v-else-if="item.person_name">QQ {{ item.qq || "未知" }}</span
            ><span v-else>—</span>
            <small v-if="item.source_message_id"
              >消息 {{ item.source_message_id }}</small
            >
          </div></template
        >
        <template #item.time="{ item }"
          ><div class="resource-time-cell">
            <span>{{ formatDate(item.sent_at || item.created_at) }}</span>
            <small v-if="item.size">{{ formatBytes(item.size) }}</small>
          </div></template
        >
        <template #item.actions="{ item }"
          ><v-btn
            v-if="item.status !== 'completed'"
            size="small"
            variant="text"
            prepend-icon="mdi-download-outline"
            :loading="selecting === item.id"
            @click="downloadOne(item)"
            >下载</v-btn
          ></template
        >
        <template #no-data
          ><div class="empty-table">
            <v-icon icon="mdi-download-off-outline" /><strong
              >没有符合条件的资源</strong
            >
          </div></template
        >
      </v-data-table-server>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { api } from "@/services/api";
import AuthImage from "@/components/AuthImage.vue";
import { useSystemCapabilitiesStore } from "@/stores/systemCapabilities";

type Policy = {
  kind: string;
  enabled: boolean;
  priority: number;
  total: number;
  pending: number;
  downloading: number;
  completed: number;
  failed: number;
  bytes: number;
};
type Reference = {
  id: string;
  kind: string;
  status: string;
  filename: string;
  source_ref?: string;
  error: string;
  size: number;
  person_name: string;
  qq: string;
  source_message_id: string;
  sent_at: string;
  created_at: string;
  mime_type: string;
  asset_url: string;
  conversation_type: string;
  conversation_id: string;
  relation_depth?: number;
  priority_reason?: string;
  priority_boost?: number;
  queue_priority?: number;
  selected?: boolean;
};
const settings = ref({ paused: false, concurrency: 3 });
const systemCapabilities = useSystemCapabilitiesStore();
const assetCount = ref(0);
const assetBytes = ref(0);
const policies = ref<Policy[]>([]);
const references = ref<Reference[]>([]);
const totalReferences = ref(0);
const loading = ref(false),
  loadingRefs = ref(false),
  error = ref(""),
  selecting = ref("");
const concurrency = ref(3),
  filterKind = ref(""),
  filterStatus = ref("pending"),
  filterDepth = ref(""),
  query = ref("");
const page = ref(1);
const itemsPerPage = ref(0);
const pageSizeOptions = computed(() => systemCapabilities.data?.lists.page_sizes || []);
let timer: number | undefined;
const kindOptions = [
  { title: "全部资源", value: "" },
  ...["avatar", "image", "sticker", "audio", "video", "file"].map((v) => ({
    title: kindLabel(v),
    value: v,
  })),
];
const statusOptions = [
  { title: "全部状态", value: "" },
  { title: "等待中", value: "pending" },
  { title: "下载中", value: "downloading" },
  { title: "已归档", value: "completed" },
  { title: "失败", value: "failed" },
];
const depthOptions = [
  { title: "全部关系层级", value: "" },
  { title: "目标人物 · 0 阶", value: "0" },
  { title: "直接关系 · 1 阶", value: "1" },
  { title: "扩散关系 · 2 阶", value: "2" },
  { title: "其他层级", value: "99" },
];
const headers = [
  { title: "类型", key: "kind" },
  { title: "关联人员", key: "person_name" },
  { title: "状态", key: "status" },
  { title: "队列优先级", key: "priority", sortable: false, width: 190 },
  { title: "来源", key: "source", sortable: false },
  { title: "时间 / 大小", key: "time", sortable: false },
  { title: "", key: "actions", sortable: false, width: 100 },
];
const totalBytes = computed(() => assetBytes.value);
async function load() {
  loading.value = true;
  error.value = "";
  try {
    const result = (await api.get("/api/v1/media/downloads")).data;
    settings.value = { paused: result.paused, concurrency: result.concurrency };
    assetCount.value = result.asset_count;
    assetBytes.value = result.asset_bytes;
    concurrency.value = result.concurrency;
    policies.value = result.policies;
  } catch (e: any) {
    error.value = e.response?.data?.error || "资源策略加载失败";
  } finally {
    loading.value = false;
  }
  await loadReferences();
}
async function loadReferences() {
  loadingRefs.value = true;
  try {
    const result = (
      await api.get("/api/v1/media/references", {
        params: {
          kind: filterKind.value,
          status: filterStatus.value,
          q: query.value,
          depth: filterDepth.value,
          limit: itemsPerPage.value,
          offset: (page.value - 1) * itemsPerPage.value,
        },
      })
    ).data;
    references.value = result.data;
    totalReferences.value = result.total;
  } catch (e: any) {
    error.value = e.response?.data?.error || "下载队列加载失败";
  } finally {
    loadingRefs.value = false;
  }
}
function resetAndLoad() {
  page.value = 1;
  loadReferences();
}
async function updateSettings(payload: any) {
  try {
    const result = (await api.patch("/api/v1/media/downloads", payload)).data;
    settings.value = { paused: result.paused, concurrency: result.concurrency };
    assetCount.value = result.asset_count;
    assetBytes.value = result.asset_bytes;
    concurrency.value = result.concurrency;
    policies.value = result.policies;
  } catch (e: any) {
    error.value = e.response?.data?.error || "资源策略更新失败";
  }
}
function setPaused(value: boolean) {
  updateSettings({ paused: value });
}
function saveConcurrency(value: number) {
  updateSettings({ concurrency: value });
}
function togglePolicy(policy: Policy) {
  updateSettings({ policies: { [policy.kind]: policy.enabled } });
}
async function downloadOne(item: Reference) {
  selecting.value = item.id;
  try {
    await api.post(`/api/v1/media/references/${item.id}/download`);
    await loadReferences();
  } catch (e: any) {
    error.value = e.response?.data?.error || "资源已加入队列失败";
  } finally {
    selecting.value = "";
  }
}
async function retryFailed() {
  try {
    await api.post("/api/v1/media/references/retry", {
      kind: filterKind.value,
    });
    await loadReferences();
  } catch (e: any) {
    error.value = e.response?.data?.error || "重试失败";
  }
}
function kindLabel(kind: string) {
  return (
    {
      avatar: "头像",
      image: "图片",
      sticker: "表情",
      audio: "语音",
      video: "视频",
      file: "文件",
    }[kind] || kind
  );
}
function kindIcon(kind: string) {
  return (
    {
      avatar: "mdi-account-circle-outline",
      image: "mdi-image-outline",
      sticker: "mdi-emoticon-outline",
      audio: "mdi-volume-high",
      video: "mdi-video-outline",
      file: "mdi-file-outline",
    }[kind] || "mdi-paperclip"
  );
}
function depthLabel(depth?: number) {
  if (depth === 0) return "目标";
  if (depth === 1) return "一阶";
  if (depth === 2) return "二阶";
  if (depth !== undefined && depth < 99) return `${depth} 阶`;
  return "全局";
}
function depthColor(depth?: number) {
  if (depth === 0) return "error";
  if (depth === 1) return "warning";
  if (depth === 2) return "info";
  return "grey";
}
function priorityLabel(item: Reference) {
  if (item.queue_priority !== undefined) return `P${item.queue_priority}`;
  const depth = item.relation_depth ?? 99;
  const boost = item.priority_boost || 0;
  if (item.selected) return `置顶${boost ? ` +${boost}` : ""}`;
  if (depth === 0) return `最高${boost ? ` +${boost}` : ""}`;
  if (depth === 1) return `高${boost ? ` +${boost}` : ""}`;
  if (depth === 2) return `中${boost ? ` +${boost}` : ""}`;
  return boost ? `普通 +${boost}` : "普通";
}
function reasonLabel(reason?: string) {
  return ({ manual: "人工提升", research_path: "当前研究路径", target: "目标人物", direct_relation: "直接关系", recursive_relation: "递归发现", avatar: "头像优先", evidence: "关系证据", global: "全局队列" } as Record<string, string>)[reason || ""] || reason || "未标记原因";
}
const statusLabel = (status: string) =>
  ({
    pending: "等待中",
    downloading: "下载中",
    completed: "已归档",
    failed: "失败",
  })[status] || status;
const statusColor = (status: string) =>
  ({
    pending: "secondary",
    downloading: "info",
    completed: "success",
    failed: "error",
  })[status] || "grey";
const formatNumber = (value: number) =>
  new Intl.NumberFormat("zh-CN").format(value || 0);
const formatDate = (value: string) =>
  value
    ? new Intl.DateTimeFormat("zh-CN", {
        dateStyle: "short",
        timeStyle: "short",
      }).format(new Date(value))
    : "—";
const compactError = (value: string) =>
  value.replace(/^media download:\s*/i, "").slice(0, 46);
const formatBytes = (value: number) =>
  value >= 1073741824
    ? (value / 1073741824).toFixed(1) + " GB"
    : value >= 1048576
      ? (value / 1048576).toFixed(1) + " MB"
      : value >= 1024
        ? (value / 1024).toFixed(1) + " KB"
        : (value || 0) + " B";
watch([filterKind, filterStatus, filterDepth], resetAndLoad);
onMounted(() => {
  void systemCapabilities.load().then(() => {
    itemsPerPage.value = itemsPerPage.value || systemCapabilities.data?.lists.default_page_size || 0;
    if (itemsPerPage.value) load();
  });
  timer = window.setInterval(load, 5000);
});
onBeforeUnmount(() => window.clearInterval(timer));
</script>

<style scoped>
.resource-priority-cell{display:flex;flex-direction:column;gap:4px}.resource-priority-cell>div{display:flex;align-items:center;gap:7px}.resource-priority-cell strong{font-size:11px;font-variant-numeric:tabular-nums}.resource-priority-cell small{max-width:180px;overflow:hidden;color:rgba(var(--v-theme-on-surface),.45);font-size:10px;text-overflow:ellipsis;white-space:nowrap}.resource-list-toolbar{grid-template-columns:190px minmax(220px,1fr) 140px 140px 160px auto}@media(max-width:1200px){.resource-list-toolbar{grid-template-columns:1fr 1fr 1fr}.resource-list-title{grid-column:1/-1}}@media(max-width:700px){.resource-list-toolbar{grid-template-columns:1fr}}
</style>
