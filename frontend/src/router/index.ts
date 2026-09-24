import { createRouter, createWebHistory } from "vue-router";
import { useSessionStore } from "@/stores/session";

const DashboardPage = () => import("@/pages/DashboardPage.vue");
const AccountsPage = () => import("@/pages/AccountsPage.vue");
const BotPage = () => import("@/pages/BotPage.vue");
const CollectionRunsPage = () => import("@/pages/CollectionRunsPage.vue");
const EgoNetworksPage = () => import("@/pages/EgoNetworksPage.vue");
const OperationsPage = () => import("@/pages/OperationsPage.vue");
const PersonsPage = () => import("@/pages/PersonsPage.vue");
const GroupsPage = () => import("@/pages/GroupsPage.vue");
const MessagesPage = () => import("@/pages/MessagesPage.vue");
const ContentsPage = () => import("@/pages/ContentsPage.vue");
const AuditsPage = () => import("@/pages/AuditsPage.vue");
const SettingsPage = () => import("@/pages/SettingsPage.vue");
const ResourceDownloadsPage = () => import("@/pages/ResourceDownloadsPage.vue");
const PersonaPipelinesPage = () => import("@/pages/PersonaPipelinesPage.vue");

const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: "/login",
      component: () => import("@/pages/LoginPage.vue"),
      meta: { public: true },
    },
    { path: "/", component: DashboardPage },
    { path: "/accounts", component: AccountsPage },
    { path: "/bot", component: BotPage },
    {
      path: "/collection-runs",
      component: CollectionRunsPage,
      meta: { title: "采集任务", icon: "mdi-database-sync" },
    },
    {
      path: "/persona-pipelines",
      component: PersonaPipelinesPage,
      meta: { title: "画像任务", icon: "mdi-brain" },
    },
    {
      path: "/ego-networks",
      component: EgoNetworksPage,
      meta: { title: "一阶关系图", icon: "mdi-graph-outline" },
    },
    {
      path: "/persons",
      name: "persons",
      component: PersonsPage,
      meta: { title: "用户资料", icon: "mdi-account-multiple-outline" },
    },
    {
      path: "/groups",
      name: "groups",
      component: GroupsPage,
      meta: { title: "群与成员", icon: "mdi-account-group-outline" },
    },
    {
      path: "/messages",
      name: "messages",
      component: MessagesPage,
      meta: { title: "消息检索", icon: "mdi-message-text-outline" },
    },
    {
      path: "/contents",
      name: "contents",
      component: ContentsPage,
      meta: { title: "空间内容", icon: "mdi-image-text" },
    },
    {
      path: "/operations",
      component: OperationsPage,
      meta: { title: "操作中心", icon: "mdi-shield-check-outline" },
    },
    {
      path: "/audits",
      component: AuditsPage,
      meta: { title: "审计记录", icon: "mdi-history" },
    },
    {
      path: "/settings",
      component: SettingsPage,
      meta: { title: "系统设置", icon: "mdi-cog-outline" },
    },
    {
      path: "/resources",
      name: "resources",
      component: ResourceDownloadsPage,
      meta: { title: "资源下载", icon: "mdi-download-box-outline" },
    },
  ],
});

router.beforeEach(async (to) => {
  const session = useSessionStore();
  if (!session.username && session.token) await session.hydrate();
  if (!to.meta.public && !session.authenticated) return "/login";
  if (to.path === "/login" && session.authenticated) return "/";
});

export default router;
