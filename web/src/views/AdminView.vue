<script setup lang="ts">
import "@fortawesome/fontawesome-free/css/all.min.css";
import {
  computed,
  defineAsyncComponent,
  onMounted,
  ref,
  watch,
  type Component,
} from "vue";
import { onBeforeRouteLeave, onBeforeRouteUpdate, useRoute, useRouter } from "vue-router";
import WarningBanner from "@/components/admin/WarningBanner.vue";
import AdminEmptyState from "@/components/admin/AdminEmptyState.vue";
import { useAuthStore } from "@/stores/auth";
import { provideAdminPageContext } from "@/composables/useAdminLoadingBar";
import { useUnsavedChanges } from "@/composables/useUnsavedChanges";
import { toast } from "@/composables/useToast";

const adminPageLoaders = {
  dashboard: () => import("@/components/admin/DashboardManagement.vue"),
  accounts: () => import("@/components/admin/AccountManagement.vue"),
  settings: () => import("@/components/admin/SystemSettings.vue"),
  tasks: () => import("@/components/admin/TaskManagement.vue"),
  tools: () => import("@/components/admin/AuxToolsManagement.vue"),
  "cross-transfer": () => import("@/components/admin/CrossDriveTransfer.vue"),
  backup: () => import("@/components/admin/BackupManagement.vue"),
  share: () => import("@/components/admin/FileShareManagement.vue"),
};
import BusySpinner from "@/components/base/BusySpinner.vue";

function withSpinner(loader: () => Promise<unknown>) {
  return defineAsyncComponent({
    loader: loader as never,
    loadingComponent: BusySpinner,
    delay: 200,
    timeout: 15000,
  });
}

const DashboardManagement = withSpinner(adminPageLoaders.dashboard);
const AccountManagement = withSpinner(adminPageLoaders.accounts);
const SystemSettings = withSpinner(adminPageLoaders.settings);
const TaskManagement = withSpinner(adminPageLoaders.tasks);
const AuxToolsManagement = withSpinner(adminPageLoaders.tools);
const CrossDriveTransfer = withSpinner(adminPageLoaders["cross-transfer"]);
const BackupManagement = withSpinner(adminPageLoaders["backup"]);
const FileShareManagement = withSpinner(adminPageLoaders.share);

const BROWSER_LOCATION_RESET_ONCE_KEY = "litepan:index:reset-once";

const nav = [
  { key: "dashboard", label: "仪表盘", icon: "fa-solid fa-gauge-high" },
  { key: "accounts", label: "存储管理", icon: "fa-solid fa-hard-drive" },
  { key: "settings", label: "系统设置", icon: "fa-solid fa-gear" },
  { key: "tasks", label: "任务管理", icon: "fa-solid fa-list-check" },
  { key: "tools", label: "辅助工具", icon: "fa-solid fa-toolbox" },
  { key: "cross-transfer", label: "跨盘秒传", icon: "fa-solid fa-right-left" },
  { key: "backup", label: "定时备份", icon: "fa-solid fa-clock-rotate-left" },
  { key: "share", label: "文件共享", icon: "fa-solid fa-share-nodes" },
];
const navKeys = nav.map((n) => n.key);

const route = useRoute();
const router = useRouter();
const auth = useAuthStore();
const { dirty, confirmLeave, discardChanges } = useUnsavedChanges();

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
  if (mustChangePassword.value && raw !== "settings") return "settings";
  return navKeys.includes(raw) ? raw : "dashboard";
}

const page = ref(normalize(route.query.page));
provideAdminPageContext(page);

const cachedPageComponents: Record<string, Component> = {
  dashboard: DashboardManagement,
  accounts: AccountManagement,
  tasks: TaskManagement,
  tools: AuxToolsManagement,
};
const cachedPageComponent = computed(() => cachedPageComponents[page.value] ?? null);

const pageTitle = computed(() => nav.find((n) => n.key === page.value)?.label ?? "后台");

function isPageLocked(key: string): boolean {
  return mustChangePassword.value && key !== "settings";
}

async function changePage(next: string) {
  if (isPageLocked(next)) return;
  if (next === page.value) return;
  await router.push({ query: buildPageQuery(next) });
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
  if (!dirty.value) return true;
  return confirmPendingChanges();
});

onBeforeRouteLeave(async (to) => {
  if (!(await confirmPendingChanges())) return false;
  if (to.name === "home") {
    sessionStorage.setItem(BROWSER_LOCATION_RESET_ONCE_KEY, "1");
  }
  return true;
});

watch(
  () => route.query.page,
  (qPage) => {
    const target = normalize(qPage);
    if (target !== page.value) page.value = target;
  },
  { immediate: true },
);

watch(mustChangePassword, (locked) => {
  if (locked) {
    page.value = "settings";
    router.replace({ query: buildPageQuery("settings") });
  }
});

onMounted(async () => {
  if (!auth.loaded) await auth.load();
  if (mustChangePassword.value) {
    page.value = "settings";
    router.replace({ query: buildPageQuery("settings") });
  }
});
</script>

<template>
  <div class="admin-page">
    <div v-if="mustChangePassword" class="admin-page__warn">
      <WarningBanner>
        <span>🛡️</span>
        <span>{{ passwordChangeMessage }}</span>
      </WarningBanner>
    </div>

    <!-- 页面标题 + 移动端子导航（桌面由左侧导航承接） -->
    <header class="admin-page__head">
      <h1 class="admin-page__title">{{ pageTitle }}</h1>
      <nav class="admin-page__tabs" aria-label="后台子导航">
        <button
          v-for="item in nav"
          :key="item.key"
          type="button"
          class="admin-page__tab"
          :class="{ 'is-active': page === item.key }"
          :disabled="isPageLocked(item.key)"
          :title="isPageLocked(item.key) ? '请先修改管理员密码' : item.label"
          @click="changePage(item.key)"
        >
          <i :class="item.icon" aria-hidden="true"></i>
          <span>{{ item.label }}</span>
        </button>
      </nav>
    </header>

    <AdminEmptyState
      v-if="!cachedPageComponent && !['settings', 'cross-transfer', 'backup', 'share'].includes(page)"
      icon="🚧"
      :title="`「${nav.find((n) => n.key === page)?.label}」功能开发中`"
    />

    <div class="admin-page__body">
      <KeepAlive>
        <SystemSettings
          v-if="page === 'settings'"
          :force-password-change="mustChangePassword"
          :password-change-reason="passwordChangeReason"
          @password-updated="handlePasswordUpdated"
        />
        <CrossDriveTransfer v-else-if="page === 'cross-transfer'" />
        <BackupManagement v-else-if="page === 'backup'" />
        <FileShareManagement v-else-if="page === 'share'" />
        <component :is="cachedPageComponent" v-else-if="cachedPageComponent" :key="page" />
      </KeepAlive>
    </div>
  </div>
</template>

<style scoped>
.admin-page {
  min-height: 100vh;
  background: var(--bg);
}

.admin-page__warn {
  padding: 16px 20px 0;
}

.admin-page__head {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 18px 20px 0;
}

.admin-page__title {
  margin: 0;
  font-size: 20px;
  font-weight: 700;
  color: var(--text);
}

.admin-page__tabs {
  display: flex;
  gap: 6px;
  overflow-x: auto;
  scrollbar-width: thin;
  padding-bottom: 4px;
}

.admin-page__tab {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  flex-shrink: 0;
  padding: 7px 14px;
  border: 1px solid var(--border-soft);
  border-radius: 999px;
  background: var(--surface);
  color: var(--text-muted);
  font-size: 13px;
  cursor: pointer;
  transition: all 0.18s ease;
}

.admin-page__tab:hover:not(:disabled) {
  color: var(--text);
  border-color: var(--border);
}

.admin-page__tab.is-active {
  background: color-mix(in srgb, var(--brand) 12%, var(--surface));
  color: var(--brand-strong, var(--brand));
  border-color: color-mix(in srgb, var(--brand) 30%, transparent);
  font-weight: 600;
}

.admin-page__tab:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.admin-page__body {
  padding: 20px;
  min-width: 0;
}

@media (min-width: 768px) {
  /* 桌面：子导航在左侧栏，内容区不再显示横向条，仅保留标题 */
  .admin-page__tabs {
    display: none;
  }
}
</style>
