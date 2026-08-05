<script setup lang="ts">
import "@fortawesome/fontawesome-free/css/all.min.css";
import {
  computed,
  defineAsyncComponent,
  onBeforeUnmount,
  onMounted,
  ref,
  watch,
  type Component,
} from "vue";
import { onBeforeRouteLeave, onBeforeRouteUpdate, useRoute, useRouter } from "vue-router";
import AdminShell from "@/components/admin/AdminShell.vue";
import WarningBanner from "@/components/admin/WarningBanner.vue";
import AdminEmptyState from "@/components/admin/AdminEmptyState.vue";

const adminPageLoaders = {
  dashboard: () => import("@/components/admin/DashboardManagement.vue"),
  accounts: () => import("@/components/admin/AccountManagement.vue"),
  settings: () => import("@/components/admin/SystemSettings.vue"),
  tasks: () => import("@/components/admin/TaskManagement.vue"),
  tools: () => import("@/components/admin/AuxToolsManagement.vue"),
  "cross-transfer": () => import("@/components/admin/CrossDriveTransfer.vue"),
  share: () => import("@/components/admin/FileShareManagement.vue"),
  qb: () => import("@/components/admin/QbPanel.vue"),
};
const DashboardManagement = defineAsyncComponent(adminPageLoaders.dashboard);
const AccountManagement = defineAsyncComponent(adminPageLoaders.accounts);
const SystemSettings = defineAsyncComponent(adminPageLoaders.settings);
const TaskManagement = defineAsyncComponent(adminPageLoaders.tasks);
const AuxToolsManagement = defineAsyncComponent(adminPageLoaders.tools);
const CrossDriveTransfer = defineAsyncComponent(adminPageLoaders["cross-transfer"]);
const FileShareManagement = defineAsyncComponent(adminPageLoaders.share);
const QbPanel = defineAsyncComponent(adminPageLoaders.qb);
import { logout, fetchSystemConfig } from "@/api/auth";
import { useAuthStore } from "@/stores/auth";
import { provideAdminPageContext } from "@/composables/useAdminLoadingBar";
import { useUnsavedChanges } from "@/composables/useUnsavedChanges";
import { toast } from "@/composables/useToast";

const BROWSER_LOCATION_STORAGE_KEY = "litepan:index:browser-location";
const BROWSER_LOCATION_RESET_ONCE_KEY = "litepan:index:reset-once";
const LEGACY_TASK_TOOL_TABS = new Set(["scrape", "aggregate"]);

const nav = [
  { key: "dashboard", label: "仪表盘", icon: "tachometer-alt" },
  { key: "accounts", label: "存储管理", icon: "hdd" },
  { key: "settings", label: "系统设置", icon: "cogs" },
  { key: "tasks", label: "任务管理", icon: "tasks" },
  { key: "tools", label: "辅助工具", icon: "toolbox" },
  { key: "cross-transfer", label: "跨盘秒传", icon: "right-left" },
  { key: "share", label: "文件共享", icon: "share-alt" },
  { key: "qb", label: "qb 下载", icon: "download" },
];
const navKeys = nav.map((n) => n.key);

const route = useRoute();
const router = useRouter();
const auth = useAuthStore();
const { dirty, confirmLeave, discardChanges } = useUnsavedChanges();
let resetBrowserLocationOnLeave = false;
let preloadTimer: number | null = null;
let preloadIdleHandle: number | null = null;
const preloadedPages = new Set<string>();

const mustChangePassword = computed(() => auth.mustChangePassword);
const passwordChangeReason = computed(() => auth.passwordChangeReason);

const passwordChangeMessage = computed(() => {
  if (passwordChangeReason.value === "default_credentials") {
    return "当前仍在使用默认管理员口令（admin/admin）。请先到系统设置 → 账号安全修改密码。";
  }
  if (passwordChangeReason.value === "temporary_password") {
    return "当前会话使用临时密码登录，请先到系统设置 → 账号安全修改密码。";
  }
  return "当前管理员密码为非安全状态。请先到系统设置 → 账号安全修改密码。";
});

function normalize(value: unknown): string {
  const raw = String(value ?? "").trim();
  const v = raw;
  if (mustChangePassword.value && v !== "settings") return "settings";
  return navKeys.includes(v) ? v : "dashboard";
}

const page = ref(normalize(route.query.page));
provideAdminPageContext(page);
const adminHomeReturnMode = ref<"sidebar" | "top_icon">("top_icon");
const cachedPageComponents: Record<string, Component> = {
  dashboard: DashboardManagement,
  accounts: AccountManagement,
  tasks: TaskManagement,
  tools: AuxToolsManagement,
  qb: QbPanel,
};
const cachedPageComponent = computed(() => cachedPageComponents[page.value] ?? null);

const pageTitle = computed(() => nav.find((n) => n.key === page.value)?.label ?? "后台");

function preloadAdminPage(key: string) {
  const loader = adminPageLoaders[key as keyof typeof adminPageLoaders];
  if (!loader || preloadedPages.has(key)) return;
  preloadedPages.add(key);
  void loader().catch(() => preloadedPages.delete(key));
}

function preloadAdminPages() {
  navKeys.forEach(preloadAdminPage);
}

function scheduleAdminPagePreload() {
  preloadTimer = window.setTimeout(() => {
    preloadTimer = null;
    if ("requestIdleCallback" in window) {
      preloadIdleHandle = window.requestIdleCallback(preloadAdminPages, { timeout: 1500 });
      return;
    }
    preloadAdminPages();
  }, 300);
}

async function loadAdminUiConfig() {
  try {
    const cfg = await fetchSystemConfig();
    adminHomeReturnMode.value = cfg.admin_home_return_mode === "sidebar" ? "sidebar" : "top_icon";
  } catch {
    adminHomeReturnMode.value = "top_icon";
  }
}

function isPageLocked(key: string): boolean {
  return mustChangePassword.value && key !== "settings";
}

async function changePage(next: string) {
  if (isPageLocked(next)) return;
  if (next === page.value) return;
  await router.push({ query: buildPageQuery(next) });
}

async function goHome() {
  resetBrowserLocationOnLeave = true;
  try {
    await router.push("/");
  } finally {
    resetBrowserLocationOnLeave = false;
  }
}

async function handleLogout() {
  if (!(await confirmPendingChanges())) return;
  try {
    await logout();
  } catch {
    /* 即使接口失败也清本地状态 */
  }
  auth.clear();
  toast.success("已退出登录");
  await router.push("/login");
}

async function handlePasswordUpdated() {
  await auth.load();
  if (!auth.mustChangePassword) {
    toast.success("密码已更新，后台功能已解锁");
  }
}

function buildPageQuery(pageKey: string): Record<string, string> {
  const query: Record<string, string> = { page: pageKey };
  if (pageKey === "settings" && mustChangePassword.value) {
    query.tab = "security";
  }
  return query;
}

async function confirmPendingChanges(): Promise<boolean> {
  if (!dirty.value) return true;
  if (!(await confirmLeave())) return false;
  discardChanges();
  return true;
}

onBeforeRouteUpdate(() => {
  // 干净页面同步放行，避免每次 sidebar/tab 导航都多等一轮异步守卫。
  if (!dirty.value) return true;
  return confirmPendingChanges();
});

onBeforeRouteLeave(async (to) => {
  if (!(await confirmPendingChanges())) return false;
  if (resetBrowserLocationOnLeave && to.name === "home") {
    localStorage.removeItem(BROWSER_LOCATION_STORAGE_KEY);
    sessionStorage.setItem(BROWSER_LOCATION_RESET_ONCE_KEY, "1");
  }
  return true;
});

watch(
  () => [route.query.page, route.query.tab] as const,
  ([qPage, qTab]) => {
    const pageKey = String(qPage ?? "").trim();
    const tabKey = String(qTab ?? "").trim();
    // 旧书签：任务管理里的刮削 → 辅助工具；聚合已下线，落到刮削页
    if (pageKey === "tasks" && LEGACY_TASK_TOOL_TABS.has(tabKey)) {
      const tab = tabKey === "aggregate" ? "scrape" : tabKey;
      void router.replace({ query: { ...route.query, page: "tools", tab } });
      return;
    }
    if (pageKey === "tools" && tabKey === "aggregate") {
      void router.replace({ query: { ...route.query, page: "tools", tab: "scrape" } });
      return;
    }
    const target = normalize(qPage);
    if (target !== page.value) page.value = target;
  },
  { immediate: true },
);

watch(mustChangePassword, (locked) => {
  if (locked) {
    page.value = "settings";
  }
});

onMounted(async () => {
  // 守卫进入后台时已拉取过认证状态，有缓存则跳过，避免重复的 /auth/status 往返。
  if (!auth.loaded) await auth.load();
  // 后台 UI 配置只影响“返回首页”按钮样式，不在首屏关键路径上，后台并行拉取。
  void loadAdminUiConfig();
  if (mustChangePassword.value) {
    page.value = "settings";
    router.replace({ query: buildPageQuery("settings") });
  }
  scheduleAdminPagePreload();
});

onBeforeUnmount(() => {
  if (preloadTimer !== null) window.clearTimeout(preloadTimer);
  if (preloadIdleHandle !== null && "cancelIdleCallback" in window) {
    window.cancelIdleCallback(preloadIdleHandle);
  }
});
</script>

<template>
  <AdminShell
    :nav="nav"
    :model-value="page"
    :page-title="pageTitle"
    :home-return-mode="adminHomeReturnMode"
    :locked-keys="mustChangePassword ? navKeys.filter((k) => k !== 'settings') : []"
    @update:model-value="changePage"
    @preload="preloadAdminPage"
    @go-home="goHome"
    @logout="handleLogout"
  >
    <WarningBanner v-if="mustChangePassword">
      <span>🛡️</span>
      <span>{{ passwordChangeMessage }}</span>
    </WarningBanner>

    <AdminEmptyState
      v-if="!cachedPageComponent && !['settings', 'cross-transfer', 'share', 'qb'].includes(page)"
      icon="🚧"
      :title="`「${nav.find((n) => n.key === page)?.label}」功能开发中`"
    />
    <KeepAlive>
      <SystemSettings
        v-if="page === 'settings'"
        :force-password-change="mustChangePassword"
        :password-change-reason="passwordChangeReason"
        @password-updated="handlePasswordUpdated"
        @admin-ui-updated="loadAdminUiConfig"
      />
      <CrossDriveTransfer v-else-if="page === 'cross-transfer'" />
      <FileShareManagement v-else-if="page === 'share'" />
      <component :is="cachedPageComponent" v-else-if="cachedPageComponent" :key="page" />
    </KeepAlive>
  </AdminShell>
</template>

<style scoped>
</style>
