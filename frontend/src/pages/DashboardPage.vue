<template>
  <div class="page-wrap dashboard-page">
    <header class="page-header dashboard-header">
      <div><h1>数据总览</h1></div>
      <div class="header-actions">
        <span
          class="live-status"
          :class="{ online: realtime.connected_accounts > 0 }"
          ><i />{{
            realtime.connected_accounts > 0
              ? realtime.connected_accounts + " 个 WS 在线"
              : "WS 离线"
          }}</span
        >
        <v-btn
          icon="mdi-refresh"
          variant="tonal"
          :loading="loading"
          title="刷新"
          @click="loadAll"
        />
      </div>
    </header>
    <v-alert
      v-if="error"
      type="error"
      variant="tonal"
      class="mb-4"
      closable
      @click:close="error = ''"
      >{{ error }}</v-alert
    >

    <section class="runtime-metrics">
      <router-link to="/persons" class="runtime-metric">
        <span>人员</span><strong>{{ formatNumber(counts.persons) }}</strong>
        <em>已建立档案</em>
      </router-link>
      <router-link to="/groups" class="runtime-metric">
        <span>群组</span><strong>{{ formatNumber(counts.groups) }}</strong>
        <em>已采集群聊</em>
      </router-link>
      <router-link to="/messages" class="runtime-metric">
        <span>消息</span><strong>{{ formatNumber(overview.messages) }}</strong>
        <em>实时累计 {{ formatNumber(realtime.total_realtime_messages) }}</em>
      </router-link>
      <router-link to="/messages" class="runtime-metric rate-metric">
        <span>消息接收速率</span>
        <strong>{{ realtime.current_per_minute }}<small> 条/分钟</small></strong>
        <em>近 5 分钟 {{ realtime.last_5_minutes }} 条</em>
      </router-link>
      <router-link to="/contents" class="runtime-metric">
        <span>空间动态</span><strong>{{ formatNumber(overview.contents || 0) }}</strong>
        <em>说说、评论与访客</em>
      </router-link>
      <router-link to="/ego-networks" class="runtime-metric">
        <span>关系事件</span
        ><strong>{{ formatNumber(overview.relation_events) }}</strong>
        <em>互动与变更</em>
      </router-link>
      <router-link to="/accounts" class="runtime-metric">
        <span>NapCat 账号</span><strong>{{ overview.accounts }}</strong>
        <em>{{ realtime.connected_accounts }} 个实时连接</em>
      </router-link>
      <router-link to="/collection-runs" class="runtime-metric">
        <span>原始记录</span
        ><strong>{{ formatNumber(overview.raw_records) }}</strong>
        <em>HTTP 与 WS</em>
      </router-link>
      <router-link to="/resources" class="runtime-metric">
        <span>本地资源</span><strong>{{ formatNumber(media.assets) }}</strong>
        <em>{{ formatBytes(media.bytes) }}</em>
      </router-link>
    </section>

    <section class="dashboard-runtime-grid">
      <div class="data-surface realtime-panel">
        <div class="panel-title">
          <div>
            <h2>实时消息</h2>
            <span>最近 60 分钟</span>
          </div>
          <time>{{
            realtime.last_message_at
              ? "最后接收 " + formatDate(realtime.last_message_at)
              : "暂无实时消息"
          }}</time>
        </div>
        <div ref="chartElement" class="realtime-chart" />
      </div>
      <div class="data-surface media-panel">
        <div class="panel-title">
          <div>
            <h2>资源归档</h2>
            <span>头像、图片、表情、语音、视频和文件</span>
          </div>
        </div>
        <div class="queue-list">
          <div>
            <span><i class="queue-dot pending" />等待</span
            ><strong>{{ media.pending }}</strong>
          </div>
          <div>
            <span><i class="queue-dot downloading" />下载中</span
            ><strong>{{ media.downloading }}</strong>
          </div>
          <div>
            <span><i class="queue-dot completed" />已归档</span
            ><strong>{{ media.completed }}</strong>
          </div>
          <div>
            <span><i class="queue-dot failed" />失败</span
            ><strong>{{ media.failed }}</strong>
          </div>
        </div>
      </div>
    </section>

    <section class="dashboard-lists">
      <div class="data-surface">
        <div class="panel-title panel-title-bordered">
          <div>
            <h2>最近消息</h2>
            <span>{{ messages.length }} 条</span>
          </div>
          <v-btn
            to="/messages"
            icon="mdi-arrow-right"
            size="small"
            variant="text"
            title="查看全部"
          />
        </div>
        <router-link
          v-for="message in messages"
          :key="message.id"
          :to="'/messages?message=' + message.id"
          class="dashboard-message-row"
        >
          <v-avatar size="30" color="surface-variant"
            ><AuthImage :src="message.sender_avatar" :alt="message.sender"
              ><span>{{ String(message.sender || "?").slice(0, 1) }}</span></AuthImage
            ></v-avatar
          >
          <div>
            <strong
              >{{ message.sender || "未知发送者"
              }}<small>QQ {{ message.sender_qq || "未知" }}</small></strong
            >
            <p>{{ message.text || "[非文本消息]" }}</p>
          </div>
          <time>{{ shortTime(message.sent_at) }}</time>
        </router-link>
        <div v-if="!messages.length" class="dashboard-empty">暂无消息</div>
      </div>
      <div class="data-surface">
        <div class="panel-title panel-title-bordered">
          <div>
            <h2>采集任务</h2>
            <span>{{ runs.length }} 条</span>
          </div>
          <v-btn
            to="/collection-runs"
            icon="mdi-arrow-right"
            size="small"
            variant="text"
            title="查看全部"
          />
        </div>
        <div v-for="run in runs" :key="run.id" class="dashboard-run-row">
          <v-icon
            :icon="runIcon(run.status)"
            :color="runColor(run.status)"
            size="18"
          />
          <div>
            <strong>{{ runLabel(run.type) }}</strong
            ><span>{{ formatDate(run.created_at) }}</span>
          </div>
          <div class="run-progress">
            <v-progress-linear
              :model-value="run.progress"
              height="3"
              :color="runColor(run.status)"
            /><small>{{ statusLabel(run.status) }} · {{ run.progress }}%</small>
          </div>
        </div>
        <div v-if="!runs.length" class="dashboard-empty">暂无采集任务</div>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref } from "vue";
import * as echarts from "echarts/core";
import { LineChart } from "echarts/charts";
import { GridComponent, TooltipComponent } from "echarts/components";
import { CanvasRenderer } from "echarts/renderers";
import type { EChartsType } from "echarts/core";
import { api } from "@/services/api";
import AuthImage from "@/components/AuthImage.vue";
import type { Overview } from "@/types/api";

echarts.use([LineChart, GridComponent, TooltipComponent, CanvasRenderer]);

const overview = ref<Overview>({
  accounts: 0,
  raw_records: 0,
  messages: 0,
  relation_events: 0,
});
const counts = ref({ persons: 0, groups: 0 });
const realtime = ref<any>({
  current_per_minute: 0,
  last_5_minutes: 0,
  total_realtime_messages: 0,
  connected_accounts: 0,
  last_message_at: null,
  series: [],
});
const media = ref<any>({
  pending: 0,
  downloading: 0,
  completed: 0,
  failed: 0,
  assets: 0,
  bytes: 0,
});
const runs = ref<any[]>([]);
const messages = ref<any[]>([]);
const loading = ref(false);
const error = ref("");
const chartElement = ref<HTMLElement>();
const chart = ref<EChartsType>();
const timer = ref<number>();

async function loadAll() {
  loading.value = true;
  try {
    const [
      overviewResult,
      realtimeResult,
      mediaResult,
      runsResult,
      messagesResult,
      personsResult,
      groupsResult,
    ] = await Promise.all([
      api.get("/api/v1/overview"),
      api.get("/api/v1/realtime/stats"),
      api.get("/api/v1/media/stats"),
      api.get("/api/v1/collection-runs"),
      api.get("/api/v1/messages", { params: { limit: 8 } }),
      api.get("/api/v1/persons", { params: { limit: 1 } }),
      api.get("/api/v1/groups", { params: { limit: 1 } }),
    ]);
    overview.value = overviewResult.data;
    realtime.value = realtimeResult.data;
    media.value = mediaResult.data;
    runs.value = (runsResult.data.data || []).slice(0, 6);
    messages.value = messagesResult.data.data || [];
    counts.value.persons = Number(personsResult.data.total ?? 0);
    counts.value.groups = Number(groupsResult.data.total ?? 0);
    await nextTick();
    renderChart();
  } catch (e: any) {
    error.value = e.response?.data?.error || "总览数据加载失败";
  } finally {
    loading.value = false;
  }
}
function renderChart() {
  if (!chartElement.value) return;
  chart.value ||= echarts.init(chartElement.value);
  chart.value.setOption({
    animationDuration: 250,
    grid: { left: 12, right: 12, top: 18, bottom: 22, containLabel: true },
    tooltip: {
      trigger: "axis",
      backgroundColor: "#20262d",
      borderColor: "#3a4652",
      textStyle: { color: "#edf2f7", fontSize: 11 },
    },
    xAxis: {
      type: "category",
      boundaryGap: false,
      data: (realtime.value.series || []).map((v: any) =>
        new Date(v.time).toLocaleTimeString("zh-CN", {
          hour: "2-digit",
          minute: "2-digit",
        }),
      ),
      axisLine: { lineStyle: { color: "#39414a" } },
      axisTick: { show: false },
      axisLabel: { color: "#737e89", fontSize: 10, interval: 9 },
    },
    yAxis: {
      type: "value",
      minInterval: 1,
      splitNumber: 3,
      axisLabel: { color: "#737e89", fontSize: 10 },
      splitLine: { lineStyle: { color: "rgba(255,255,255,.05)" } },
    },
    series: [
      {
        type: "line",
        data: (realtime.value.series || []).map((v: any) => v.count),
        showSymbol: false,
        smooth: 0.25,
        lineStyle: { color: "#8ab4f8", width: 2 },
        areaStyle: { color: "rgba(138,180,248,.09)" },
      },
    ],
  });
}
const formatNumber = (v: number) =>
  new Intl.NumberFormat("zh-CN").format(v || 0);
const formatBytes = (v: number) =>
  v >= 1073741824
    ? (v / 1073741824).toFixed(1) + " GB"
    : v >= 1048576
      ? (v / 1048576).toFixed(1) + " MB"
      : v >= 1024
        ? (v / 1024).toFixed(1) + " KB"
        : (v || 0) + " B";
const formatDate = (v: string) =>
  v
    ? new Intl.DateTimeFormat("zh-CN", {
        dateStyle: "short",
        timeStyle: "short",
      }).format(new Date(v))
    : "—";
const shortTime = (v: string) =>
  v
    ? new Intl.DateTimeFormat("zh-CN", {
        hour: "2-digit",
        minute: "2-digit",
      }).format(new Date(v))
    : "—";
const runLabel = (v: string) =>
  ({
    full_visible_data: "可见数据采集",
    group_history: "群历史",
    media_sync: "媒体同步",
  })[v] || v;
const statusLabel = (v: string) =>
  ({
    queued: "等待",
    running: "运行中",
    completed: "完成",
    failed: "失败",
    cancelled: "已取消",
  })[v] || v;
const runColor = (v: string) =>
  ({
    running: "info",
    completed: "success",
    failed: "error",
    cancelled: "warning",
  })[v] || "secondary";
const runIcon = (v: string) =>
  ({
    running: "mdi-sync",
    completed: "mdi-check-circle-outline",
    failed: "mdi-alert-circle-outline",
    cancelled: "mdi-cancel",
  })[v] || "mdi-clock-outline";
function resize() {
  chart.value?.resize();
}
onMounted(() => {
  loadAll();
  timer.value = window.setInterval(loadAll, 5000);
  window.addEventListener("resize", resize);
});
onBeforeUnmount(() => {
  if (timer.value) clearInterval(timer.value);
  window.removeEventListener("resize", resize);
  chart.value?.dispose();
});
</script>
