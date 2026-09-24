<template>
  <div class="page-wrap records-page">
    <header class="page-header records-header">
      <div class="records-heading">
        <h1>账号连接</h1>
        <span>{{ accounts.length }} 个账号 · {{ connectedCount }} 个实时连接</span>
      </div>
      <div class="records-header-actions">
        <v-btn icon="mdi-refresh" variant="text" :loading="loading" title="刷新" @click="load" />
        <v-btn color="primary" prepend-icon="mdi-plus" @click="dialog = true">添加账号</v-btn>
      </div>
    </header>

    <v-alert v-if="error" type="error" variant="tonal" class="mb-4" closable @click:close="error = ''">{{ error }}</v-alert>

    <section class="source-card-grid">
      <article v-for="item in accounts" :key="item.id" class="source-card data-surface">
        <header class="source-card-header">
          <div class="source-card-identity"><div class="source-card-icon"><v-icon icon="mdi-connection" /></div><div><strong>{{ item.name }}</strong><span>QQ {{ item.qq_uin || '未识别' }}</span></div></div>
          <v-chip size="small" :color="item.status === 'connected' ? 'success' : 'grey'" variant="tonal">{{ item.status === 'connected' ? '已连接' : '未连接' }}</v-chip>
        </header>
        <div class="source-card-body">
          <div class="source-endpoint"><div><v-icon icon="mdi-access-point-network" size="17" /><strong>NapCat</strong></div><span>{{ item.http_url }}</span><small>HTTP API · {{ item.status === 'connected' ? '实时通道正常' : '等待连接' }}</small></div>
          <div class="source-endpoint"><div><v-icon icon="mdi-orbit" size="17" /><strong>QQ 空间</strong></div><template v-if="qzoneByAccount[item.id]"><span>{{ qzoneByAccount[item.id].http_url }}</span><small>{{ qzoneByAccount[item.id].status === 'connected' ? '实时通道正常' : '等待连接' }}</small></template><small v-else>尚未配置</small></div>
        </div>
        <footer class="source-card-actions">
          <v-btn :prepend-icon="item.status === 'connected' ? 'mdi-link-off' : 'mdi-connection'" size="small" variant="tonal" :loading="connecting === `napcat:${item.id}`" @click="connectNapCat(item)">{{ item.status === 'connected' ? '断开连接' : '连接 NapCat' }}</v-btn>
          <v-btn icon="mdi-lan-check" size="small" variant="text" title="测试 HTTP" @click="testNapCat(item)" />
          <v-btn icon="mdi-orbit" size="small" variant="text" title="配置 QQ 空间" @click="openQZone(item)" />
          <v-btn icon="mdi-pencil-outline" size="small" variant="text" title="编辑账号" @click="openEdit(item)" />
          <v-btn icon="mdi-delete-outline" size="small" variant="text" color="error" title="删除账号" @click="confirmDelete(item)" />
        </footer>
      </article>
      <div v-if="!accounts.length && !loading" class="empty-table source-empty"><v-icon icon="mdi-connection" /><strong>还没有数据源</strong><span>添加一个 NapCat 账号，开始连接数据。</span></div>
    </section>

    <!-- 添加 / 编辑 NapCat 账号 -->
    <v-dialog v-model="dialog" max-width="640">
      <v-card>
        <v-card-title class="pa-6">{{ isEditing ? '编辑 NapCat 账号' : '添加 NapCat 账号' }}</v-card-title>
        <v-card-text class="px-6">
          <v-text-field v-model="form.name" label="名称" />
          <v-text-field v-model="form.qq_uin" label="QQ 号" class="mt-3" />
          <v-text-field v-model="form.ws_url" label="WebSocket 地址" class="mt-3" />
          <v-text-field v-model="form.ws_token" label="WS Token" type="password" placeholder="留空保持原 Token" class="mt-3" />
          <v-text-field v-model="form.http_url" label="HTTP API 地址" class="mt-3" />
          <v-text-field v-model="form.http_token" label="HTTP Token" type="password" placeholder="留空保持原 Token" class="mt-3" />
        </v-card-text>
        <v-card-actions class="pa-6 pt-0"><v-spacer /><v-btn variant="text" @click="dialog = false">取消</v-btn><v-btn color="primary" :loading="saving" @click="save">保存</v-btn></v-card-actions>
      </v-card>
    </v-dialog>

    <!-- 删除确认弹窗 -->
    <v-dialog v-model="deleteDialog" max-width="460">
      <v-card>
        <v-card-title class="pa-5 font-weight-bold">确认删除账号</v-card-title>
        <v-card-text class="px-5">
          确定要删除账号 <strong>{{ deletingAccount?.name }} (QQ: {{ deletingAccount?.qq_uin }})</strong> 吗？断开连接后将不再接收该账号的实时消息。
        </v-card-text>
        <v-card-actions class="pa-5 pt-0">
          <v-spacer />
          <v-btn variant="text" @click="deleteDialog = false">取消</v-btn>
          <v-btn color="error" variant="flat" :loading="deleting" @click="executeDelete">确认删除</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <v-dialog v-model="qzoneDialog" max-width="640">
      <v-card>
        <v-card-title class="d-flex align-center pa-6">
          <span>{{ selectedAccount?.name }} · QQ 空间</span>
          <v-spacer />
          <v-chip v-if="selectedQZone" size="small" :color="selectedQZone.status === 'connected' ? 'success' : 'grey'" variant="tonal">
            {{ selectedQZone.status === 'connected' ? '实时已连接' : '未连接' }}
          </v-chip>
        </v-card-title>
        <v-card-text class="px-6">
          <v-text-field v-model="qzoneForm.http_url" label="插件 HTTP 地址" />
          <v-text-field v-model="qzoneForm.ws_url" label="插件 WebSocket 地址" class="mt-3" />
          <v-text-field v-model="qzoneForm.access_token" label="Access Token" type="password" placeholder="留空则保持原 Token" class="mt-3" />
          <v-switch v-model="qzoneForm.enabled" label="启用空间实时连接" color="primary" hide-details class="mt-2" />
          <v-alert v-if="selectedQZone?.last_error" type="warning" variant="tonal" density="compact" class="mt-4">{{ selectedQZone.last_error }}</v-alert>
          <dl v-if="selectedQZone" class="connection-meta mt-4">
            <dt>最后连接</dt><dd>{{ formatDate(selectedQZone.last_connected_at) }}</dd>
            <dt>最后事件</dt><dd>{{ formatDate(selectedQZone.last_event_at) }}</dd>
          </dl>
        </v-card-text>
        <v-card-actions class="pa-6 pt-0">
          <v-btn v-if="selectedQZone" variant="text" :loading="testingQZone" @click="testQZone">测试</v-btn>
          <v-btn v-if="selectedQZone" variant="text" :loading="connecting === `qzone:${selectedAccount?.id}`" @click="toggleQZone">
            {{ selectedQZone.status === 'connected' ? '断开实时' : '连接实时' }}
          </v-btn>
          <v-spacer />
          <v-btn variant="text" @click="qzoneDialog = false">取消</v-btn>
          <v-btn color="primary" :loading="savingQZone" @click="saveQZone">保存</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <v-snackbar v-model="snack.show" :color="snack.color">{{ snack.text }}</v-snackbar>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import { api } from "@/services/api";
import type { Account, QZoneConnection } from "@/types/api";

const defaults = { name: "", qq_uin: "", ws_url: "ws://127.0.0.1:3001", ws_token: "", http_url: "http://127.0.0.1:3000", http_token: "" };
const qzoneDefaults = { http_url: "http://127.0.0.1:5700", ws_url: "ws://127.0.0.1:5700/event", access_token: "", enabled: true };
const accounts = ref<Account[]>([]), qzoneConnections = ref<QZoneConnection[]>([]);
const loading = ref(false), saving = ref(false), savingQZone = ref(false), testingQZone = ref(false);
const connecting = ref(""), dialog = ref(false), qzoneDialog = ref(false), deleteDialog = ref(false), deleting = ref(false), error = ref("");
const selectedAccount = ref<Account | null>(null), editingAccount = ref<Account | null>(null), deletingAccount = ref<Account | null>(null);
const isEditing = computed(() => !!editingAccount.value);
const form = reactive({ ...defaults }), qzoneForm = reactive({ ...qzoneDefaults });
const snack = reactive({ show: false, text: "", color: "success" });

const qzoneByAccount = computed<Record<string, QZoneConnection>>(() => Object.fromEntries(qzoneConnections.value.map((item) => [item.account_id, item])));
const selectedQZone = computed(() => selectedAccount.value ? qzoneByAccount.value[selectedAccount.value.id] : undefined);
const connectedCount = computed(() => accounts.value.filter((item) => item.status === "connected").length);

async function load() {
  loading.value = true;
  try {
    const [accountResponse, qzoneResponse] = await Promise.all([api.get("/api/v1/accounts"), api.get("/api/v1/qzone/connections")]);
    accounts.value = accountResponse.data.data;
    qzoneConnections.value = qzoneResponse.data.data;
  } catch (e: any) { error.value = e.response?.data?.error || "无法加载账号连接"; }
  finally { loading.value = false; }
}

function openEdit(account: Account) {
  editingAccount.value = account;
  Object.assign(form, {
    name: account.name,
    qq_uin: account.qq_uin,
    ws_url: account.ws_url,
    ws_token: "",
    http_url: account.http_url,
    http_token: "",
  });
  dialog.value = true;
}

function confirmDelete(account: Account) {
  deletingAccount.value = account;
  deleteDialog.value = true;
}

async function executeDelete() {
  if (!deletingAccount.value) return;
  deleting.value = true;
  try {
    await api.delete(`/api/v1/accounts/${deletingAccount.value.id}`);
    deleteDialog.value = false;
    deletingAccount.value = null;
    await load();
    notify("账号已删除");
  } catch (e: any) {
    error.value = e.response?.data?.error || "删除账号失败";
  } finally {
    deleting.value = false;
  }
}

async function save() {
  saving.value = true;
  try {
    if (editingAccount.value) {
      await api.patch(`/api/v1/accounts/${editingAccount.value.id}`, form);
      notify("账号已更新");
    } else {
      await api.post("/api/v1/accounts", form);
      notify("账号已添加");
    }
    dialog.value = false;
    editingAccount.value = null;
    Object.assign(form, defaults);
    await load();
  } catch (e: any) {
    error.value = e.response?.data?.error || "保存失败";
  } finally {
    saving.value = false;
  }
}
async function connectNapCat(account: Account) {
  connecting.value = `napcat:${account.id}`;
  try { await api.post(`/api/v1/accounts/${account.id}/${account.status === "connected" ? "disconnect" : "connect"}`); await load(); }
  catch (e: any) { error.value = e.response?.data?.error || "NapCat 连接失败"; }
  finally { connecting.value = ""; }
}
async function testNapCat(account: Account) {
  try { await api.post(`/api/v1/accounts/${account.id}/test`); notify("NapCat HTTP 正常"); }
  catch (e: any) { error.value = e.response?.data?.error || "NapCat HTTP 测试失败"; }
}
function openQZone(account: Account) {
  selectedAccount.value = account;
  const current = qzoneByAccount.value[account.id];
  Object.assign(qzoneForm, current ? { http_url: current.http_url, ws_url: current.ws_url, access_token: "", enabled: current.enabled } : qzoneDefaults);
  qzoneDialog.value = true;
}
async function saveQZone() {
  if (!selectedAccount.value) return;
  savingQZone.value = true;
  try { await api.put(`/api/v1/accounts/${selectedAccount.value.id}/qzone`, qzoneForm); await load(); notify("空间连接已保存"); }
  catch (e: any) { error.value = e.response?.data?.error || "空间连接保存失败"; }
  finally { savingQZone.value = false; }
}
async function testQZone() {
  if (!selectedAccount.value) return;
  testingQZone.value = true;
  try { await api.post(`/api/v1/accounts/${selectedAccount.value.id}/qzone/test`); notify("QQ 空间插件 HTTP 正常"); }
  catch (e: any) { error.value = e.response?.data?.error || "QQ 空间插件测试失败"; }
  finally { testingQZone.value = false; }
}
async function toggleQZone() {
  if (!selectedAccount.value || !selectedQZone.value) return;
  connecting.value = `qzone:${selectedAccount.value.id}`;
  try {
    const action = selectedQZone.value.status === "connected" ? "disconnect" : "connect";
    await api.post(`/api/v1/accounts/${selectedAccount.value.id}/qzone/${action}`);
    await load();
  } catch (e: any) { error.value = e.response?.data?.error || "空间实时连接失败"; }
  finally { connecting.value = ""; }
}
function notify(text: string) { snack.text = text; snack.color = "success"; snack.show = true; }
function formatDate(value?: string) { return value ? new Intl.DateTimeFormat("zh-CN", { dateStyle: "short", timeStyle: "short" }).format(new Date(value)) : "—"; }
onMounted(load);
</script>

<style scoped>
.account-cell,.connection-cell{display:flex;flex-direction:column;gap:4px;padding:8px 0}.account-cell span,.connection-cell small{font-size:12px;color:rgba(var(--v-theme-on-surface),.58)}
.connection-meta{display:grid;grid-template-columns:88px 1fr;gap:8px 16px;font-size:13px}.connection-meta dt{color:rgba(var(--v-theme-on-surface),.58)}
.source-card-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(360px,1fr));gap:12px}.source-card{overflow:hidden}.source-card-header{display:flex;align-items:flex-start;justify-content:space-between;gap:12px;padding:18px;border-bottom:1px solid rgba(var(--v-theme-on-surface),.08)}.source-card-identity{display:flex;align-items:center;gap:12px;min-width:0}.source-card-identity>div:last-child{display:flex;min-width:0;flex-direction:column;gap:4px}.source-card-identity strong{font-size:15px}.source-card-identity span{color:rgba(var(--v-theme-on-surface),.52);font-size:11px}.source-card-icon{width:40px;height:40px;display:grid;place-items:center;border-radius:6px;background:rgba(var(--v-theme-primary),.12);color:rgb(var(--v-theme-primary))}.source-card-body{display:grid;grid-template-columns:1fr 1fr;gap:10px;padding:14px 18px}.source-endpoint{min-width:0;display:flex;flex-direction:column;gap:5px;padding:12px;background:rgba(0,0,0,.14);border:1px solid rgba(var(--v-theme-on-surface),.07);border-radius:4px}.source-endpoint>div{display:flex;align-items:center;gap:6px}.source-endpoint>div .v-icon{color:rgb(var(--v-theme-primary))}.source-endpoint strong{font-size:12px}.source-endpoint span,.source-endpoint small{overflow:hidden;color:rgba(var(--v-theme-on-surface),.48);font-size:10px;text-overflow:ellipsis;white-space:nowrap}.source-endpoint small{color:rgba(var(--v-theme-on-surface),.34)}.source-card-actions{display:flex;align-items:center;justify-content:flex-end;gap:4px;padding:10px 14px;border-top:1px solid rgba(var(--v-theme-on-surface),.08)}.source-empty{grid-column:1/-1}.source-card-actions .v-btn:first-child{margin-right:auto}@media(max-width:700px){.source-card-grid{grid-template-columns:1fr}.source-card-body{grid-template-columns:1fr}.source-card-header{padding:14px}.source-card-body{padding:12px 14px}}
</style>
