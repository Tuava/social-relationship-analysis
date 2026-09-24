<template>
  <div class="page-wrap groups-page records-page">
    <section class="list-toolbar records-toolbar data-surface">
      <v-text-field v-model.trim="query" label="群名或群号" prepend-inner-icon="mdi-magnify" hide-details clearable @keyup.enter="resetAndLoad" @click:clear="resetAndLoad" />
      <v-btn color="primary" variant="tonal" @click="resetAndLoad">搜索</v-btn>
      <span class="result-hint">{{ formatNumber(total) }} 个群</span>
      <v-btn icon="mdi-refresh" variant="text" :loading="loading" title="刷新" @click="load" />
    </section>
    <v-alert v-if="error" type="error" variant="tonal" class="mt-4">{{ error }}</v-alert>
    <section class="data-surface records-surface mt-3">
    <v-data-table-server class="records-table" density="compact" :headers="headers" :items="groups" :loading="loading" :items-length="total" :items-per-page="pageSize" :page="page" :items-per-page-options="pageSizeOptions" hover @click:row="openGroup" @update:page="page=$event;persistAndLoad()" @update:items-per-page="pageSize=$event;page=1;persistAndLoad()">
        <template #item.group_name="{ item }"><div class="identity-cell"><span class="row-avatar id-avatar"><AuthImage :src="item.avatar_uri" :alt="item.group_name || '群'"><span>{{ initial(item.group_name || '群') }}</span></AuthImage></span><div><strong>{{ item.group_name || "未命名群" }}</strong><small>群号 {{ item.group_id }}</small></div></div></template>
        <template #item.member_count="{ item }"><span class="numeric-cell">{{ formatNumber(item.member_count) }}</span></template>
        <template #item.first_seen_at="{ item }">{{ formatDate(item.first_seen_at) }}</template>
        <template #item.actions="{ item }"><v-btn icon="mdi-chevron-right" size="small" variant="text" title="查看群资料" @click.stop="openGroup(null,{item})" /></template>
        <template #no-data><div class="empty-table"><v-icon icon="mdi-account-group-outline" /><strong>没有匹配群</strong></div></template>
      </v-data-table-server>
    </section>

    <v-navigation-drawer v-model="drawer" location="right" temporary width="640">
      <div class="detail-drawer group-drawer">
        <div class="detail-header">
          <div class="d-flex align-center ga-3">
            <span class="row-avatar" style="width:48px;height:48px">
              <AuthImage :src="detail?.avatar_uri" :alt="detail?.group_name || '群'">
                <span>{{ initial(detail?.group_name || '群') }}</span>
              </AuthImage>
            </span>
            <div>
              <small>群号 {{ detail?.group_id || "—" }}</small>
              <h2>{{ detail?.group_name || "群详情" }}</h2>
            </div>
          </div>
          <v-btn icon="mdi-close" variant="text" @click="drawer=false" />
        </div>
        <v-skeleton-loader v-if="detailLoading" type="heading,paragraph,list-item-three-line@4" />
        <template v-else-if="detail">
          <div class="group-summary">
            <div><span>成员</span><strong>{{ formatNumber(memberTotal) }}</strong></div>
            <div><span>消息</span><strong>{{ formatNumber(messageTotal) }}</strong></div>
            <div><span>群号</span><strong>{{ detail.group_id }}</strong></div>
            <div><span>首次发现</span><strong>{{ formatDate(detail.first_seen_at) }}</strong></div>
          </div>

          <!-- 群组操作快捷栏 -->
          <div class="group-actions-strip px-4 py-2 d-flex ga-2">
            <v-btn size="small" color="primary" variant="flat" prepend-icon="mdi-graph-outline" @click="openGroupInGraph">在图谱中分析</v-btn>
            <v-btn size="small" variant="tonal" prepend-icon="mdi-message-text-outline" @click="searchGroupMessages">检索群消息</v-btn>
            <v-btn size="small" variant="tonal" prepend-icon="mdi-account-sync-outline" @click="syncGroupProfiles">同步本群资料</v-btn>
          </div>

          <v-tabs v-model="tab" density="compact" class="detail-tabs">
            <v-tab value="members">成员 ({{ formatNumber(memberTotal) }})</v-tab>
            <v-tab value="messages">消息 ({{ formatNumber(messageTotal) }})</v-tab>
          </v-tabs>
          <v-window v-model="tab">
            <v-window-item value="members">
              <div class="detail-section flush">
                <div v-for="member in members" :key="member.id" class="entity-row cursor-pointer" @click="openPerson(member.qq)">
                  <span class="row-avatar">
                    <AuthImage :src="member.avatar_uri" :alt="member.display_name">
                      <span>{{ initial(member.display_name) }}</span>
                    </AuthImage>
                  </span>
                  <div class="flex-grow-1">
                    <strong>{{ member.display_name || "未命名用户" }}</strong>
                    <small>QQ {{ member.qq || "未知" }}</small>
                  </div>
                  <v-chip v-if="member.role !== 'member'" size="x-small" color="warning" variant="tonal">{{ roleLabel(member.role) }}</v-chip>
                  <v-icon icon="mdi-chevron-right" size="16" color="grey" />
                </div>
                <v-btn v-if="members.length < memberTotal" block variant="text" :loading="sectionLoading" class="mt-2" @click="loadMoreMembers">加载更多成员</v-btn>
              </div>
            </v-window-item>
            <v-window-item value="messages">
              <div class="group-message-list">
                <article v-for="message in messages" :key="message.id" class="group-message">
                  <span class="row-avatar cursor-pointer" @click="openPerson(message.sender_qq)">
                    <AuthImage :src="message.sender_avatar" :alt="message.sender">
                      <span>{{ initial(message.sender) }}</span>
                    </AuthImage>
                  </span>
                  <div>
                    <header>
                      <strong class="cursor-pointer" @click="openPerson(message.sender_qq)">{{ message.sender || "未知发送者" }}</strong>
                      <small>QQ {{ message.sender_qq || "未知" }} · {{ formatDate(message.sent_at) }}</small>
                      <v-btn icon="mdi-open-in-new" size="x-small" variant="text" class="ml-1" title="在聊天工作台中定位此条消息" @click.stop="searchGroupMessages(message.id)" />
                    </header>
                    <p class="cursor-pointer" @click="searchGroupMessages(message.id)">{{ message.text || "[非文本消息]" }}</p>
                    <ContentMedia v-if="message.media_preview?.length" :items="message.media_preview" variant="compact" @archive="archive" />
                  </div>
                </article>
                <v-btn v-if="messages.length < messageTotal" block variant="text" :loading="sectionLoading" class="mt-2" @click="loadMoreMessages">加载更多消息</v-btn>
                <div v-if="!messages.length && !sectionLoading" class="compact-empty">暂无群消息</div>
              </div>
            </v-window-item>
          </v-window>
        </template>
      </div>
    </v-navigation-drawer>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { useRoute, useRouter } from "vue-router";
import { api } from "@/services/api";
import AuthImage from "@/components/AuthImage.vue";
import ContentMedia from "@/components/ContentMedia.vue";
import { useListStateStore } from "@/stores/listState";
import { useSystemCapabilitiesStore } from "@/stores/systemCapabilities";

const route = useRoute(), router = useRouter();
const listState = useListStateStore(), systemCapabilities = useSystemCapabilitiesStore();
const PAGE_KEY = "groups";
const restored = listState.load(PAGE_KEY, { query: "", page: 1, pageSize: 0 });

const groups = ref<any[]>([]), query = ref(String(restored.query || "")), loading = ref(false), error = ref("");
const total = ref(0), page = ref(Number(restored.page) || 1), pageSize = ref(Number(restored.pageSize) || 0);
const pageSizeOptions = computed(() => systemCapabilities.data?.lists.page_sizes || []);
const drawer = ref(false), detail = ref<any>(null), detailLoading = ref(false), sectionLoading = ref(false), tab = ref("members");
const members = ref<any[]>([]), messages = ref<any[]>([]), memberTotal = ref(0), messageTotal = ref(0);
const headers = [{ title: "群", key: "group_name" }, { title: "已采集成员", key: "member_count" }, { title: "首次发现", key: "first_seen_at" }, { title: "", key: "actions", sortable: false, width: 52 }];

async function load() {
  loading.value = true; error.value = "";
  try { const result = (await api.get("/api/v1/groups", { params: { q: query.value, limit: pageSize.value, offset: (page.value-1)*pageSize.value } })).data; groups.value = result.data || []; total.value = Number(result.total ?? groups.value.length); }
  catch (e: any) { error.value = e.response?.data?.error || "群数据加载失败"; }
  finally { loading.value = false; }
  persistState();
}
function persistState() { listState.save(PAGE_KEY, { query: query.value, page: page.value, pageSize: pageSize.value }); }
function persistAndLoad() { persistState(); load(); }
function resetAndLoad() { page.value = 1; load(); }
async function openGroup(_: unknown, row: any) {
  drawer.value = true; detailLoading.value = true; tab.value = "members"; members.value = []; messages.value = [];
  try { detail.value = (await api.get(`/api/v1/groups/${row.item.id}`)).data.data; await Promise.all([loadMembers(true), loadMessages(true)]); }
  catch (e: any) { error.value = e.response?.data?.error || "群详情加载失败"; drawer.value = false; }
  finally { detailLoading.value = false; }
}
async function loadMembers(reset = false) { if (!detail.value) return; sectionLoading.value = true; try { const offset = reset ? 0 : members.value.length; const result = (await api.get(`/api/v1/groups/${detail.value.id}/members`, { params: { limit: systemCapabilities.data?.lists.default_page_size, offset } })).data; members.value = reset ? result.data : [...members.value, ...result.data]; memberTotal.value = Number(result.total ?? members.value.length); } finally { sectionLoading.value = false; } }
async function loadMessages(reset = false) { if (!detail.value) return; sectionLoading.value = true; try { const offset = reset ? 0 : messages.value.length; const result = (await api.get(`/api/v1/groups/${detail.value.id}/messages`, { params: { limit: systemCapabilities.data?.lists.default_page_size, offset } })).data; messages.value = reset ? result.data : [...messages.value, ...result.data]; messageTotal.value = Number(result.total ?? messages.value.length); } finally { sectionLoading.value = false; } }
const loadMoreMembers = () => loadMembers(false), loadMoreMessages = () => loadMessages(false);
async function archive(item: any) { const id = item.id || item.reference_id; if (!id) return; try { await api.post(`/api/v1/media/references/${id}/download`); item.status = "pending"; } catch (e: any) { error.value = e.response?.data?.error || "资源加入下载队列失败"; } }

function openGroupInGraph() {
  if (!detail.value?.group_id) return;
  router.push({ path: "/ego-networks", query: { target: `group:${detail.value.group_id}` } });
}

function searchGroupMessages(messageId?: string) {
  if (!detail.value?.group_id) return;
  router.push({
    path: "/messages",
    query: { q: detail.value.group_id, ...(messageId ? { message_id: messageId } : {}) },
  });
}

function syncGroupProfiles() {
  if (!detail.value?.group_id) return;
  router.push({ path: "/collection-runs", query: { group_id: detail.value.group_id, action: "sync_profiles" } });
}

function openPerson(qq?: string) {
  if (!qq) return;
  router.push({ path: "/persons", query: { q: qq } });
}

const formatNumber = (value: number) => new Intl.NumberFormat("zh-CN").format(value || 0);
const formatDate = (value: string) => value ? new Intl.DateTimeFormat("zh-CN", { dateStyle: "medium", timeStyle: "short" }).format(new Date(value)) : "—";
const roleLabel = (value: string) => ({ owner: "群主", admin: "管理员", member: "成员" } as Record<string,string>)[value] || value;
const initial = (value: string) => String(value || "?").trim().slice(0,1);
onMounted(async () => {
  await systemCapabilities.load();
  if (!pageSize.value) pageSize.value = systemCapabilities.data?.lists.default_page_size || 0;
  const initialQ = String(route.query.q || route.query.query || "");
  if (initialQ) {
    query.value = initialQ;
    page.value = 1;
  }
  if (pageSize.value) {
    await load();
    if (initialQ && groups.value.length) {
      const match = groups.value.find((g) => g.group_id === initialQ || g.group_name === initialQ) || groups.value[0];
      if (match) {
        await openGroup(null, { item: match });
      }
    }
  }
});
</script>

<style scoped>
.identity-cell{display:grid;grid-template-columns:34px minmax(0,1fr);gap:10px;align-items:center}
.row-avatar{width:32px;height:32px;display:grid;place-items:center;overflow:hidden;border-radius:50%;background:rgb(var(--v-theme-surface-variant));font-size:11px;flex-shrink:0}.row-avatar :deep(img){width:100%;height:100%;object-fit:cover}.group-message-list{padding:4px 14px 18px}.group-message{display:grid;grid-template-columns:32px minmax(0,1fr);gap:10px;padding:14px 0;border-bottom:1px solid rgba(var(--v-theme-on-surface),.07)}.group-message header{display:flex;align-items:baseline;gap:8px}.group-message header strong{font-size:12px}.group-message header small{color:rgba(var(--v-theme-on-surface),.48);font-size:10px}.group-message p{margin:5px 0 9px;white-space:pre-wrap;overflow-wrap:anywhere;font-size:12px;line-height:1.55}
.entity-row{display:flex;align-items:center;gap:12px;padding:10px 14px;border-bottom:1px solid rgba(var(--v-theme-on-surface),.06);transition:background .15s}.entity-row:hover{background:rgba(var(--v-theme-on-surface),.04)}
.group-actions-strip{background:rgba(0,0,0,.1);border-bottom:1px solid rgba(var(--v-theme-on-surface),.08);flex-wrap:wrap}
.cursor-pointer{cursor:pointer}
</style>
