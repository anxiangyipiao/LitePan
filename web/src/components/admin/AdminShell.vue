<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import AdminAccountChip from "@/components/admin/AdminAccountChip.vue";
import AdminGlobalActions from "@/components/admin/AdminGlobalActions.vue";
import AdminNavIcon from "@/components/admin/AdminNavIcon.vue";
import { useAdminLoadingBar } from "@/composables/useAdminLoadingBar";

interface NavItem {
  key: string;
  label: string;
  icon: string;
}

const SIDEBAR_COLLAPSED_KEY = "litepan-admin-sidebar-collapsed";
const MOBILE_BREAKPOINT = 768;

withDefaults(
  defineProps<{
    nav: NavItem[];
    modelValue: string;
    pageTitle?: string;
    lockedKeys?: string[];
    homeReturnMode?: "sidebar" | "top_icon";
  }>(),
  { homeReturnMode: "top_icon" },
);
const emit = defineEmits<{
  "update:modelValue": [string];
  preload: [string];
  logout: [];
  goHome: [];
}>();

const sidebarCollapsed = ref(false);
const mobileDrawerOpen = ref(false);
const isMobile = ref(false);
const { visible: pageLoadingVisible } = useAdminLoadingBar();

const sidebarCompact = computed(() => !isMobile.value && sidebarCollapsed.value);

const sidebarToggleLabel = computed(() => {
  if (isMobile.value) return mobileDrawerOpen.value ? "关闭菜单" : "打开菜单";
  return sidebarCollapsed.value ? "展开侧栏" : "收起侧栏";
});

function readCollapsedPref() {
  try {
    sidebarCollapsed.value = localStorage.getItem(SIDEBAR_COLLAPSED_KEY) === "1";
  } catch {
    sidebarCollapsed.value = false;
  }
}

function persistCollapsedPref() {
  try {
    localStorage.setItem(SIDEBAR_COLLAPSED_KEY, sidebarCollapsed.value ? "1" : "0");
  } catch {}
}

function syncViewport() {
  isMobile.value = window.innerWidth <= MOBILE_BREAKPOINT;
  if (!isMobile.value) mobileDrawerOpen.value = false;
}

function syncSidebarWidthVar() {
  // 与下方 .admin / .admin--collapsed 的实际宽度保持一致，供 Teleport 到 body 的抽屉对齐。
  const width = isMobile.value ? "0px" : sidebarCollapsed.value ? "64px" : "56px";
  document.documentElement.style.setProperty("--sidebar-width", width);
}

function toggleSidebar() {
  if (isMobile.value) {
    mobileDrawerOpen.value = !mobileDrawerOpen.value;
    return;
  }
  sidebarCollapsed.value = !sidebarCollapsed.value;
  persistCollapsedPref();
}

function closeMobileDrawer() {
  mobileDrawerOpen.value = false;
}

function selectNav(key: string) {
  emit("update:modelValue", key);
  closeMobileDrawer();
}

function goHomeFromSidebar() {
  emit("goHome");
  closeMobileDrawer();
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === "Escape" && mobileDrawerOpen.value) closeMobileDrawer();
}

onMounted(() => {
  readCollapsedPref();
  syncViewport();
  syncSidebarWidthVar();
  window.addEventListener("resize", syncViewport);
  window.addEventListener("keydown", onKeydown);
});

watch([sidebarCollapsed, isMobile], () => {
  syncSidebarWidthVar();
});

watch(mobileDrawerOpen, (open) => {
  if (typeof document === "undefined") return;
  document.body.style.overflow = open && isMobile.value ? "hidden" : "";
});

onBeforeUnmount(() => {
  window.removeEventListener("resize", syncViewport);
  window.removeEventListener("keydown", onKeydown);
  document.body.style.overflow = "";
  document.documentElement.style.removeProperty("--sidebar-width");
});
</script>

<template>
  <div
    class="admin"
    :class="{
      'admin--collapsed': sidebarCompact,
      'admin--drawer-open': isMobile && mobileDrawerOpen,
      'admin--mobile': isMobile,
    }"
  >
    <div
      v-if="isMobile"
      class="sidebar-backdrop"
      :class="{ 'sidebar-backdrop--visible': mobileDrawerOpen }"
      aria-hidden="true"
      @click="closeMobileDrawer"
    />

    <aside class="sidebar">
      <header class="sidebar__header">
        <img
          :src="sidebarCompact ? '/static/img/logo-l.png' : '/static/img/logo.png'"
          alt="LitePan"
          class="sidebar__logo"
        />
      </header>

      <nav class="sidebar__nav">
        <button
          v-for="item in nav"
          :key="item.key"
          class="nav-item"
          :class="{
            'nav-item--active': item.key === modelValue,
            'nav-item--locked': lockedKeys?.includes(item.key),
          }"
          :disabled="lockedKeys?.includes(item.key)"
          @pointerenter="emit('preload', item.key)"
          @focus="emit('preload', item.key)"
          @click="selectNav(item.key)"
        >
          <AdminNavIcon :name="item.icon" class="nav-item__icon" />
          <span class="nav-item__label">{{ item.label }}</span>
        </button>
        <button
          v-if="homeReturnMode === 'sidebar'"
          type="button"
          class="nav-item nav-item--home"
          @click="goHomeFromSidebar"
        >
          <AdminNavIcon name="home" class="nav-item__icon" />
          <span class="nav-item__label">返回首页</span>
        </button>
      </nav>

      <footer class="sidebar__footer">
        <AdminAccountChip :compact="sidebarCompact" @logout="emit('logout')" />
      </footer>
    </aside>

    <header class="global-chrome">
      <button
        type="button"
        class="sidebar-toggle"
        :class="{ 'sidebar-toggle--active': isMobile && mobileDrawerOpen }"
        :aria-label="sidebarToggleLabel"
        :aria-expanded="isMobile ? mobileDrawerOpen : !sidebarCollapsed"
        @click="toggleSidebar"
      >
        <svg v-if="isMobile && mobileDrawerOpen" viewBox="0 0 24 24" aria-hidden="true">
          <path d="m6 6 12 12M18 6 6 18" />
        </svg>
        <svg v-else-if="isMobile" viewBox="0 0 24 24" aria-hidden="true">
          <path d="M4 7h16M4 12h16M4 17h16" />
        </svg>
        <svg v-else-if="sidebarCollapsed" viewBox="0 0 24 24" aria-hidden="true">
          <path d="m10 6 6 6-6 6M4 6v12" />
        </svg>
        <svg v-else viewBox="0 0 24 24" aria-hidden="true">
          <path d="m14 6-6 6 6 6M20 6v12" />
        </svg>
      </button>

      <div v-if="pageTitle" class="global-chrome__context">
        <span class="global-chrome__crumb">后台</span>
        <span class="global-chrome__sep">/</span>
        <span class="global-chrome__title">{{ pageTitle }}</span>
      </div>
      <div class="global-chrome__spacer" />
      <AdminGlobalActions
        :show-home-return="homeReturnMode === 'top_icon'"
        @go-home="emit('goHome')"
      />
      <Transition name="admin-loading-bar">
        <div v-if="pageLoadingVisible" class="global-loading-bar" aria-hidden="true">
          <span />
        </div>
      </Transition>
    </header>

    <main class="admin__body">
      <slot />
    </main>
  </div>
</template>

<style scoped>
.admin {
  --admin-chrome-h: 44px;
  --sidebar-width: 56px;
  display: grid;
  grid-template-columns: var(--sidebar-width) minmax(0, 1fr);
  grid-template-rows: var(--admin-chrome-h) minmax(0, 1fr);
  height: 100vh;
  overflow: hidden;
  background: var(--bg);
}

.admin--collapsed {
  --sidebar-width: 64px;
}

.admin--mobile {
  --sidebar-width: 0px;
  grid-template-columns: minmax(0, 1fr);
}

.sidebar-backdrop {
  display: none;
}

.sidebar {
  grid-column: 1;
  grid-row: 1 / -1;
  z-index: 120;
  position: relative;
  min-height: 0;
  /* 覆盖 pdfjs-dist 全局 .sidebar 规则（pdf-vendor 会给任意 .sidebar 强加
     239px 宽 / 180px 最小宽 / 边框 / 圆角 / 内边距），否则侧栏会溢出网格列遮住内容 */
  width: 100%;
  min-width: 0;
  max-width: none;
  box-sizing: border-box;
  padding: 0;
  border: none;
  border-right: 1px solid var(--admin-sidebar-border);
  border-radius: 0;
  border-top-right-radius: var(--radius-lg);
  backdrop-filter: none;
  display: flex;
  flex-direction: column;
  background: var(--admin-sidebar-bg);
  box-shadow: var(--admin-sidebar-shadow);
  color: #fff;
  transition: transform 0.28s ease, box-shadow 0.28s ease;
}

.sidebar__header {
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 52px;
  padding: 0;
  border-bottom: 1px solid rgba(255, 255, 255, 0.15);
}

.sidebar__logo {
  max-width: 32px;
  max-height: 32px;
  width: auto;
  height: auto;
  object-fit: contain;
  object-position: center;
  transition: max-width 0.2s ease, max-height 0.2s ease;
}

.sidebar__nav {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 2px;
  padding: 8px 0;
  overflow-y: auto;
  scrollbar-width: thin;
  scrollbar-color: rgba(255, 255, 255, 0.35) transparent;
}

.sidebar__nav::-webkit-scrollbar {
  width: 6px;
}

.sidebar__nav::-webkit-scrollbar-thumb {
  background: rgba(255, 255, 255, 0.3);
  border-radius: 3px;
}

.sidebar__nav::-webkit-scrollbar-track {
  background: transparent;
}

.sidebar__footer {
  flex-shrink: 0;
  padding: 8px 0;
  display: flex;
  justify-content: center;
  border-top: 1px solid rgba(255, 255, 255, 0.15);
}

.nav-item {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 40px;
  height: 40px;
  padding: 0;
  border: none;
  background: transparent;
  color: rgba(255, 255, 255, 0.85);
  border-radius: 10px;
  font-size: 0;
  transition: all 0.2s ease;
  cursor: pointer;
}

.nav-item:hover:not(.nav-item--active):not(:disabled) {
  background: rgba(255, 255, 255, 0.1);
  color: #fff;
}

.nav-item--active,
.nav-item--active:hover {
  background: #fff;
  color: var(--brand);
  font-weight: 600;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.1);
}

.nav-item--home {
  text-decoration: none;
  margin-top: 4px;
}

.nav-item--locked,
.nav-item:disabled {
  opacity: 0.55;
  cursor: not-allowed;
}

.nav-item:disabled:hover {
  background: transparent;
  color: rgba(255, 255, 255, 0.85);
}

.nav-item__icon {
  margin-right: 0;
  flex-shrink: 0;
}

.nav-item__label {
  display: none;
}

.admin--collapsed .sidebar__header {
  height: 52px;
  padding: 0;
}

.admin--collapsed .sidebar__logo {
  max-width: 28px;
  max-height: 34px;
}

.admin--collapsed .sidebar__nav {
  padding: 10px 8px 12px;
}

.admin--collapsed .sidebar__footer {
  padding: 8px;
}

.admin--collapsed .nav-item {
  justify-content: center;
  padding: 0;
  height: 50px;
}

.admin--collapsed .nav-item__icon {
  margin-right: 0;
}

.admin--collapsed .nav-item__label {
  display: none;
}

.sidebar-toggle {
  flex-shrink: 0;
  width: 36px;
  height: 36px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: none;
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--text-muted);
  cursor: pointer;
  transition: var(--transition);
}

.sidebar-toggle:hover,
.sidebar-toggle--active {
  color: var(--brand);
  background: var(--surface-sunken);
}

.sidebar-toggle svg {
  width: 18px;
  height: 18px;
  stroke: currentColor;
  stroke-width: 2;
  fill: none;
  stroke-linecap: round;
  stroke-linejoin: round;
}

.global-chrome {
  grid-column: 1 / -1;
  grid-row: 1;
  position: relative;
  z-index: 1;
  height: var(--admin-chrome-h);
  display: flex;
  align-items: center;
  gap: 10px;
  padding-left: calc(var(--sidebar-width) + 8px);
  padding-right: 22px;
  background: var(--surface);
  border-bottom: 1px solid var(--border-soft);
  box-shadow: var(--shadow-card);
}

.global-chrome__context {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  font-size: 13px;
}

.global-chrome__crumb {
  flex-shrink: 0;
  color: var(--text-muted);
}

.global-chrome__sep {
  flex-shrink: 0;
  color: var(--border);
}

.global-chrome__title {
  overflow: hidden;
  white-space: nowrap;
  text-overflow: ellipsis;
  color: var(--text);
  font-weight: 700;
}

.global-chrome__spacer {
  flex: 1;
  min-width: 12px;
}

.global-loading-bar {
  position: absolute;
  left: var(--sidebar-width);
  right: 0;
  bottom: -1px;
  height: 2px;
  overflow: hidden;
  pointer-events: none;
}

.global-loading-bar span {
  position: absolute;
  inset: 0;
  background: linear-gradient(
    90deg,
    transparent 0%,
    var(--brand-start) 45%,
    var(--brand-end) 55%,
    transparent 100%
  );
  transform: translateX(-100%);
  animation: admin-loading-slide 0.9s ease-in-out infinite;
}

.admin-loading-bar-enter-active,
.admin-loading-bar-leave-active {
  transition: opacity 0.16s ease;
}

.admin-loading-bar-enter-from,
.admin-loading-bar-leave-to {
  opacity: 0;
}

@keyframes admin-loading-slide {
  to {
    transform: translateX(100%);
  }
}

@media (prefers-reduced-motion: reduce) {
  .global-loading-bar span {
    animation: none;
    transform: none;
    background: var(--brand);
  }
}

.admin__body {
  grid-column: 2;
  grid-row: 2;
  min-height: 0;
  padding: 24px;
  overflow-x: auto;
  overflow-y: auto;
  background: var(--bg);
}

@media (max-width: 768px) {
  .sidebar-backdrop {
    display: block;
    position: fixed;
    inset: 0;
    z-index: 110;
    background: rgba(15, 23, 42, 0.35);
    opacity: 0;
    pointer-events: none;
    transition: opacity 0.22s ease;
  }

  .sidebar-backdrop--visible {
    opacity: 1;
    pointer-events: auto;
  }

  .sidebar {
    position: fixed;
    top: 0;
    left: 0;
    width: min(260px, 82vw);
    min-width: 0;
    height: 100vh;
    transform: translateX(-100%);
    border-top-right-radius: 0;
  }

  .admin--drawer-open .sidebar {
    transform: translateX(0);
    box-shadow: 2px 0 16px rgba(15, 23, 42, 0.18);
  }

  .admin--drawer-open .global-chrome {
    /* 抽屉打开时把顶栏提到遮罩之上，汉堡可独立点击（backdrop z-index:110） */
    z-index: 121;
  }

  .global-chrome {
    --admin-chrome-h: 42px;
    grid-column: 1;
    padding-left: 14px;
    padding-right: 14px;
  }

  /* 移动端保留当前页名（crumb 省略），关闭抽屉后仍能知道所在页面 */
  .global-chrome__crumb,
  .global-chrome__sep {
    display: none;
  }

  .global-chrome__context {
    display: flex;
  }

  .global-chrome__title {
    max-width: 44vw;
    font-size: 14px;
  }

  .global-loading-bar {
    left: 0;
  }

  .admin__body {
    grid-column: 1;
    padding: 16px 14px 20px;
  }
}
</style>
