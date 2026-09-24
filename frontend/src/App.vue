<template>
  <v-app>
    <template v-if="session.authenticated">
      <v-navigation-drawer
        v-model:rail="rail"
        :expand-on-hover="!isPinned"
        :permanent="isPinned"
        rail-width="56"
        width="240"
        class="main-navigation-drawer"
      >
        <!-- 顶部品牌区 -->
        <v-list class="pa-0">
          <v-list-item
            prepend-icon="mdi-share-variant-outline"
            title="社会关系分析"
            subtitle="关系拓扑网络"
            class="app-brand-nav-item"
          />
        </v-list>
        <v-divider />

        <!-- 导航菜单 -->
        <v-list nav density="compact" class="pa-2 app-nav-list">
          <template v-for="section in navSections" :key="section.title">
            <v-list-subheader v-if="section.title" class="nav-section-title">{{ section.title }}</v-list-subheader>
            <v-list-item
              v-for="item in section.items"
              :key="item.to"
              :to="item.to"
              :prepend-icon="item.icon"
              :title="item.title"
              :aria-label="item.title"
              :value="item.to"
              class="app-nav-item"
              active-class="app-nav-item-active"
            />
          </template>
        </v-list>

        <!-- 底部用户信息与退出 -->
        <template #append>
          <v-divider />
          <v-list class="pa-0">
            <v-list-item
              :title="session.username"
              subtitle="本机账号"
              class="app-user-nav-item"
            >
              <template #prepend>
                <v-avatar color="primary" size="28">
                  <span class="text-caption font-weight-bold">{{ session.username.slice(0, 1).toUpperCase() }}</span>
                </v-avatar>
              </template>
              <template #append>
                <v-btn icon="mdi-logout" variant="text" size="small" title="退出登录" @click.stop="session.logout()" />
              </template>
            </v-list-item>
          </v-list>
        </template>
      </v-navigation-drawer>

      <!-- 全局顶栏：严格 56px 高度与左侧品牌区平齐，底部分割线完全贯通 -->
      <v-app-bar flat border="b" height="56">
        <v-btn
          :icon="isPinned ? 'mdi-pin' : 'mdi-menu'"
          variant="text"
          size="40"
          class="app-shell-menu-button"
          :title="isPinned ? '取消固定（恢复悬浮导轨）' : '固定常开侧边栏'"
          @click="togglePin"
        />
        <v-toolbar-title class="text-body-2 font-weight-medium">
          {{ currentTitle }}
        </v-toolbar-title>
        <v-spacer />
        <div class="d-flex align-center ga-2 pr-3">
          <v-chip size="x-small" variant="tonal" color="success" prepend-icon="mdi-check-circle-outline">系统就绪</v-chip>
          <v-btn
            :icon="theme.global.current.value.dark ? 'mdi-weather-sunny' : 'mdi-weather-night'"
            variant="text"
            size="small"
            :title="theme.global.current.value.dark ? '切换至白天明亮模式' : '切换至暗黑极客模式'"
            @click="toggleTheme"
          />
          <v-btn icon="mdi-cog-outline" variant="text" size="small" title="系统设置" to="/settings" />
        </div>
      </v-app-bar>
    </template>
    <v-main><router-view /></v-main>
  </v-app>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { useRoute } from "vue-router";
import { useTheme } from "vuetify";
import { useSessionStore } from "@/stores/session";
import { useSystemCapabilitiesStore } from "@/stores/systemCapabilities";

const route = useRoute();
const session = useSessionStore();
const systemCapabilities = useSystemCapabilitiesStore();
const theme = useTheme();

function toggleTheme() {
  const next = theme.global.name.value === "analysisDark" ? "analysisLight" : "analysisDark";
  theme.global.name.value = next;
  localStorage.setItem("sra.theme", next);
  document.documentElement.setAttribute("data-theme", next === "analysisLight" ? "light" : "dark");
}

onMounted(() => {
  systemCapabilities.load();
  const saved = localStorage.getItem("sra.theme");
  if (saved && (saved === "analysisDark" || saved === "analysisLight")) {
    theme.global.name.value = saved;
    document.documentElement.setAttribute("data-theme", saved === "analysisLight" ? "light" : "dark");
  } else {
    document.documentElement.setAttribute("data-theme", theme.global.current.value.dark ? "dark" : "light");
  }
});

// 侧边栏模式：默认悬浮导轨 (isPinned = false, rail = true)
const isPinned = ref(false);
const rail = ref(true);

function togglePin() {
  isPinned.value = !isPinned.value;
  rail.value = !isPinned.value;
}

const navSections = [
  { title: "", items: [{ to: "/", title: "总览仪表盘", icon: "mdi-view-dashboard-outline" }] },
  { title: "研究", items: [
    { to: "/ego-networks", title: "关系工作台", icon: "mdi-graph-outline" },
    { to: "/persona-pipelines", title: "画像任务", icon: "mdi-brain" },
  ] },
  { title: "数据", items: [
    { to: "/persons", title: "人员名册", icon: "mdi-account-multiple-outline" },
    { to: "/groups", title: "群组档案", icon: "mdi-account-group-outline" },
    { to: "/messages", title: "消息检索", icon: "mdi-message-text-outline" },
    { to: "/contents", title: "空间动态", icon: "mdi-image-text" },
  ] },
  { title: "采集", items: [
    { to: "/accounts", title: "数据源配置", icon: "mdi-connection" },
    { to: "/collection-runs", title: "采集任务", icon: "mdi-database-sync" },
    { to: "/resources", title: "资源下载", icon: "mdi-download-box-outline" },
  ] },
  { title: "管理", items: [
    { to: "/operations", title: "操作中心", icon: "mdi-shield-check-outline" },
    { to: "/audits", title: "审计记录", icon: "mdi-history" },
    { to: "/bot", title: "QQ 机器人", icon: "mdi-cat" },
    { to: "/settings", title: "系统设置", icon: "mdi-cog-outline" },
  ] },
];
const navItems = navSections.flatMap((section) => section.items);

const currentTitle = computed(
  () =>
    navItems.find((item) => item.to === route.path)?.title || "社会关系分析",
);
</script>
