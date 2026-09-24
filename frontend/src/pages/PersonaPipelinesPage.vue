<template>
  <div class="page-wrap records-page pipeline-page">
    <header class="page-header records-header">
      <div class="records-heading"><h1>画像任务</h1><span>{{ total }} 个任务 · {{ runningCount }} 个运行中</span></div>
      <div class="records-header-actions">
        <v-chip v-if="lastUpdated" size="small" variant="tonal" color="success" prepend-icon="mdi-sync">实时更新 {{ formatTime(lastUpdated) }}</v-chip>
        <v-btn icon="mdi-refresh" variant="text" :loading="loading" title="刷新任务" @click="load" />
        <v-btn color="primary" prepend-icon="mdi-play" @click="openCreate">新建批量任务</v-btn>
      </div>
    </header>
    <v-alert v-if="error" type="error" variant="tonal" class="mb-3" closable @click:close="error = ''">{{ error }}</v-alert>
    <section class="data-surface records-surface">
      <v-data-table-server
        class="records-table" density="compact" :headers="headers" :items="pipelines" :loading="loading"
        :items-length="total" :items-per-page="pageSize" :page="page" :items-per-page-options="[20, 50, 100]"
        hover @update:page="page = $event; load()" @update:items-per-page="pageSize = $event; page = 1; load()"
      >
        <template #item.title="{ item }"><button class="pipeline-title" @click="openDetail(item)"><strong>{{ item.title }}</strong><small>{{ item.id }}</small></button></template>
        <template #item.scope_type="{ item }"><v-chip size="x-small" variant="tonal" color="info">{{ scopeLabel(item.scope_type) }}</v-chip></template>
        <template #item.status="{ item }"><v-chip size="x-small" variant="tonal" :color="statusColor(item.status)">{{ statusLabel(item.status) }}</v-chip></template>
        <template #item.progress="{ item }"><div class="pipeline-progress"><div class="d-flex justify-space-between text-caption mb-1"><span>{{ item.completed_targets + item.failed_targets }} / {{ item.total_targets }}</span><span>{{ progress(item) }}%</span></div><v-progress-linear :model-value="progress(item)" :color="statusColor(item.status)" rounded height="5" /><small>成功 {{ item.completed_targets }} · 失败 {{ item.failed_targets }}</small></div></template>
        <template #item.created_at="{ item }">{{ formatDate(item.created_at) }}</template>
        <template #item.actions="{ item }"><div class="pipeline-actions"><v-btn icon="mdi-eye-outline" size="small" variant="text" title="查看详情" @click="openDetail(item)" /><v-btn v-if="['queued', 'running'].includes(item.status)" icon="mdi-stop-circle-outline" size="small" variant="text" color="warning" title="停止任务" @click="cancel(item)" /></div></template>
        <template #no-data><div class="empty-table"><v-icon icon="mdi-brain-off-outline" /><strong>暂无画像任务</strong></div></template>
      </v-data-table-server>
    </section>

    <v-dialog v-model="detailOpen" max-width="980" scrollable>
      <v-card v-if="selected" class="pipeline-detail-dialog">
        <v-card-title class="dialog-title d-flex align-center"><div><strong>{{ selected.title }}</strong><small class="d-block text-medium-emphasis">{{ selected.id }}</small></div><v-spacer /><v-btn icon="mdi-refresh" variant="text" size="small" :loading="detailLoading" title="刷新详情" @click="loadDetail(selected.id)" /><v-btn icon="mdi-close" variant="text" size="small" title="关闭" @click="detailOpen = false" /></v-card-title>
        <v-divider />
        <v-card-text>
          <div class="pipeline-detail-stats"><div><strong>{{ selected.total_targets }}</strong><small>目标总数</small></div><div><strong class="text-success">{{ selected.completed_targets }}</strong><small>成功</small></div><div><strong class="text-error">{{ selected.failed_targets }}</strong><small>失败</small></div><div><strong>{{ progress(selected) }}%</strong><small>完成进度</small></div></div>
          <v-progress-linear class="my-4" :model-value="progress(selected)" :color="statusColor(selected.status)" rounded height="8" />
          <div class="pipeline-live-meta mb-3">
            <span>范围：{{ scopeLabel(selected.scope_type) }}</span>
            <span>并发 {{ selected.concurrency }}</span>
            <span>待处理 {{ detailPendingCount }}</span>
            <span>处理中 {{ detailProcessingCount }}</span>
            <span>更新时间 {{ formatTime(selected.updated_at) }}</span>
          </div>
          <div class="text-caption text-medium-emphasis mb-3">重试 {{ selected.auto_retry ? `开启（最多 ${selected.max_retries} 次，每次等待 ${selected.retry_backoff_seconds} 秒起）` : '关闭' }} · 队列等待 {{ selected.queue_wait_seconds }} 秒</div>
          <v-alert v-if="selected.error_message" type="error" variant="tonal" density="compact" class="mb-3">{{ selected.error_message }}</v-alert>
          <div class="pipeline-items-heading">目标明细 <span>第 {{ detailPage }} 页 · 共 {{ selected.total_targets }} 个目标 · 状态和进度由服务端实时统计</span></div>
          <v-btn v-if="selected.failed_targets > 0 && !['queued', 'running'].includes(selected.status)" class="mb-3" color="warning" variant="tonal" prepend-icon="mdi-replay" @click="retryFailed(selected)">继续未完成并重试失败目标</v-btn>
          <v-data-table-server density="compact" :headers="itemHeaders" :items="detailItems" :items-length="selected.total_targets" :items-per-page="detailPageSize" :page="detailPage" :loading="detailLoading" :items-per-page-options="[25, 50, 100]" @update:page="onDetailPageChange" @update:items-per-page="onDetailPageSizeChange"><template #item.status="{ item }"><v-chip size="x-small" variant="tonal" :color="statusColor(item.status)">{{ statusLabel(item.status) }}</v-chip></template><template #item.target="{ item }"><span>{{ item.display_name || '未命名用户' }} <small class="text-medium-emphasis">{{ item.qq }}</small></span></template><template #item.error_message="{ item }"><span class="text-error text-caption">{{ item.error_message || '—' }}</span></template></v-data-table-server>
        </v-card-text>
      </v-card>
    </v-dialog>

    <v-dialog v-model="createOpen" max-width="620" persistent>
      <v-card>
        <v-card-title class="dialog-title d-flex align-center"><strong>新建批量画像任务</strong><v-spacer /><v-btn icon="mdi-close" variant="text" title="关闭" @click="createOpen = false" /></v-card-title>
        <v-divider />
        <v-card-text>
          <v-alert v-if="createError" type="error" variant="tonal" density="compact" class="mb-4">{{ createError }}</v-alert>
          <v-text-field v-model="createForm.title" label="任务名称" placeholder="留空使用系统生成名称" variant="outlined" density="compact" class="mb-2" />
          <v-select v-model="createForm.scope_type" label="目标范围" :items="scopeOptions" item-title="title" item-value="value" variant="outlined" density="compact" class="mb-2" />
          <v-text-field v-if="createForm.scope_type === 'group'" v-model="createForm.scope_target" label="群组 ID" variant="outlined" density="compact" class="mb-2" />
          <div class="form-grid">
            <v-text-field v-model.number="createForm.concurrency" type="number" min="1" label="任务并发" hint="最终并发仍受系统模型队列配置影响" persistent-hint variant="outlined" density="compact" />
            <v-text-field v-model.number="createForm.max_retries" type="number" min="0" label="最大重试次数" variant="outlined" density="compact" />
            <v-text-field v-model.number="createForm.retry_backoff_seconds" type="number" min="0" label="重试等待（秒）" variant="outlined" density="compact" />
            <v-text-field v-model.number="createForm.queue_wait_seconds" type="number" min="0" label="队列等待（秒）" variant="outlined" density="compact" />
          </div>
          <v-switch v-model="createForm.auto_retry" label="启用自动重试" color="primary" hide-details />
          <v-switch v-model="createForm.force_analyze" label="忽略已有画像，强制重新分析" color="warning" hide-details />
        </v-card-text>
        <v-divider />
        <v-card-actions class="pa-4"><v-spacer /><v-btn variant="text" @click="createOpen = false">取消</v-btn><v-btn color="primary" :loading="creating" prepend-icon="mdi-play" @click="createPipeline">下发任务</v-btn></v-card-actions>
      </v-card>
    </v-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from "vue";
import { api } from "@/services/api";
import { useRoute, useRouter } from "vue-router";

type Pipeline = { id: string; title: string; status: string; scope_type: string; total_targets: number; completed_targets: number; failed_targets: number; concurrency: number; auto_retry: boolean; max_retries: number; retry_backoff_seconds: number; queue_wait_seconds: number; error_message?: string; created_at: string; updated_at: string };
const pipelines = ref<Pipeline[]>([]); const total = ref(0); const page = ref(1); const pageSize = ref(20); const loading = ref(false); const error = ref("");
const detailOpen = ref(false); const detailLoading = ref(false); const selected = ref<Pipeline | null>(null); const detailItems = ref<any[]>([]); let pollTimer: ReturnType<typeof setInterval> | undefined;
const detailPollTimer = ref<ReturnType<typeof setInterval> | undefined>();
const detailPage = ref(1); const detailPageSize = ref(50);
const lastUpdated = ref("");
const createOpen = ref(false); const creating = ref(false); const createError = ref("");
const router = useRouter(); const route = useRoute();
const createForm = ref({ title: "", scope_type: "with_messages", scope_target: "", concurrency: 3, auto_retry: true, max_retries: 3, retry_backoff_seconds: 15, queue_wait_seconds: 180, force_analyze: false });
const scopeOptions = [{ title: "全库有发言记录的人物", value: "with_messages" }, { title: "全库所有人物", value: "all_persons" }, { title: "指定群组", value: "group" }];
const headers = [{ title: "任务", key: "title", sortable: false, minWidth: 260 }, { title: "范围", key: "scope_type", sortable: false }, { title: "状态", key: "status", sortable: false }, { title: "进度", key: "progress", sortable: false, width: 220 }, { title: "创建时间", key: "created_at", sortable: false }, { title: "", key: "actions", sortable: false, width: 90 }];
const itemHeaders = [{ title: "目标", key: "target", sortable: false }, { title: "状态", key: "status", sortable: false }, { title: "尝试", key: "attempts", sortable: false }, { title: "错误", key: "error_message", sortable: false }];
const runningCount = computed(() => pipelines.value.filter((p) => ["queued", "running"].includes(p.status)).length);
const detailPendingCount = computed(() => detailItems.value.filter((item) => item.status === "pending").length);
const detailProcessingCount = computed(() => detailItems.value.filter((item) => item.status === "processing").length);
const progress = (p: Pipeline) => p.total_targets > 0 ? Math.min(100, Math.round(((p.completed_targets + p.failed_targets) / p.total_targets) * 100)) : 0;
const statusLabel = (v: string) => ({ queued: "排队中", running: "运行中", completed: "已完成", partial: "部分完成", failed: "失败", cancelled: "已取消" } as Record<string, string>)[v] || v;
const statusColor = (v: string) => ({ queued: "info", running: "primary", completed: "success", partial: "warning", failed: "error", cancelled: "grey" } as Record<string, string>)[v] || "grey";
const scopeLabel = (v: string) => ({ with_messages: "全库有发言记录", all_persons: "全库所有人物", custom_qqs: "指定目标", group: "指定群组" } as Record<string, string>)[v] || v;
const formatDate = (v: string) => v ? new Intl.DateTimeFormat("zh-CN", { dateStyle: "short", timeStyle: "medium" }).format(new Date(v)) : "—";
const formatTime = (v: string) => v ? new Intl.DateTimeFormat("zh-CN", { hour: "2-digit", minute: "2-digit", second: "2-digit" }).format(new Date(v)) : "—";
async function load() { loading.value = true; try { const res = (await api.get("/api/v1/pipelines/batch-persona", { params: { limit: pageSize.value, offset: (page.value - 1) * pageSize.value } })).data; pipelines.value = res.data || []; total.value = res.total || 0; lastUpdated.value = new Date().toISOString(); if (selected.value) { const fresh = pipelines.value.find((p) => p.id === selected.value?.id); if (fresh) selected.value = { ...selected.value, ...fresh }; } } catch (e: any) { error.value = e.response?.data?.error || "画像任务加载失败"; } finally { loading.value = false; } }
function openCreate() { createError.value = ""; createOpen.value = true; }
async function createPipeline() { creating.value = true; createError.value = ""; try { const res = (await api.post("/api/v1/pipelines/batch-persona", createForm.value)).data; createOpen.value = false; await load(); const created = pipelines.value.find((p) => p.id === res.pipeline_id); if (created) await openDetail(created); else { await load(); } } catch (e: any) { createError.value = e.response?.data?.error || "下发画像任务失败"; } finally { creating.value = false; } }
async function openDetail(pipeline: Pipeline) { selected.value = pipeline; detailPage.value = 1; detailOpen.value = true; await loadDetail(pipeline.id); startDetailPolling(pipeline.id); }
async function loadDetail(id: string) { detailLoading.value = true; try { const res = (await api.get(`/api/v1/pipelines/batch-persona/${id}`, { params: { item_limit: detailPageSize.value, item_offset: (detailPage.value - 1) * detailPageSize.value } })).data; selected.value = res.data; detailItems.value = res.items || []; lastUpdated.value = new Date().toISOString(); } catch (e: any) { error.value = e.response?.data?.error || "任务详情加载失败"; } finally { detailLoading.value = false; } }
async function onDetailPageChange(pageNumber: number) { detailPage.value = pageNumber; if (selected.value) await loadDetail(selected.value.id); }
async function onDetailPageSizeChange(size: number) { detailPageSize.value = size; detailPage.value = 1; if (selected.value) await loadDetail(selected.value.id); }
function startDetailPolling(id: string) { if (detailPollTimer.value) clearInterval(detailPollTimer.value); detailPollTimer.value = setInterval(async () => { if (!detailOpen.value || !selected.value || selected.value.id !== id) return; await loadDetail(id); if (!["queued", "running"].includes(selected.value?.status || "")) stopDetailPolling(); }, 2000); }
function stopDetailPolling() { if (detailPollTimer.value) { clearInterval(detailPollTimer.value); detailPollTimer.value = undefined; } }
async function cancel(pipeline: Pipeline) { if (!window.confirm(`确定停止任务“${pipeline.title}”吗？`)) return; try { await api.post(`/api/v1/pipelines/batch-persona/${pipeline.id}/cancel`); await load(); if (selected.value?.id === pipeline.id) await loadDetail(pipeline.id); } catch (e: any) { error.value = e.response?.data?.error || "停止任务失败"; } }
async function retryFailed(pipeline: Pipeline) { try { await api.post(`/api/v1/pipelines/batch-persona/${pipeline.id}/retry-failed`); await load(); await loadDetail(pipeline.id); startDetailPolling(pipeline.id); } catch (e: any) { error.value = e.response?.data?.error || "重试失败目标失败"; } }
onMounted(async () => { await load(); pollTimer = setInterval(() => { if (runningCount.value > 0) load(); }, 3000); if (route.query.create === "1") openCreate(); });
onUnmounted(() => { if (pollTimer) clearInterval(pollTimer); stopDetailPolling(); });
</script>

<style scoped>
.pipeline-title { display: grid; gap: 3px; text-align: left; color: inherit; background: none; border: 0; cursor: pointer; max-width: 420px; }
.pipeline-title strong { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.pipeline-title small { color: rgba(var(--v-theme-on-surface), .5); font-family: monospace; }
.pipeline-progress { min-width: 180px; }
.pipeline-progress small { color: rgba(var(--v-theme-on-surface), .55); font-size: 11px; }
.pipeline-actions { display: flex; justify-content: flex-end; }
.pipeline-detail-stats { display: grid; grid-template-columns: repeat(4, 1fr); gap: 10px; }
.pipeline-detail-stats > div { padding: 12px; border: 1px solid rgba(var(--v-theme-on-surface), .1); border-radius: 8px; }
.pipeline-detail-stats strong, .pipeline-detail-stats small { display: block; }
.pipeline-detail-stats small { margin-top: 4px; color: rgba(var(--v-theme-on-surface), .6); }
.pipeline-items-heading { font-weight: 600; margin-bottom: 8px; }
.pipeline-items-heading span { font-size: 12px; font-weight: 400; color: rgba(var(--v-theme-on-surface), .55); margin-left: 8px; }
.pipeline-live-meta { display: flex; flex-wrap: wrap; gap: 8px 16px; color: rgba(var(--v-theme-on-surface), .65); font-size: 12px; }
.pipeline-live-meta span { white-space: nowrap; }
.form-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px; }
@media (max-width: 700px) { .pipeline-detail-stats { grid-template-columns: repeat(2, 1fr); } }
@media (max-width: 560px) { .form-grid { grid-template-columns: 1fr; } }
</style>
