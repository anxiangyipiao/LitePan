<script setup lang="ts">
import { computed } from "vue";
import { RouterLink, RouterView, useRoute, useRouter } from "vue-router";
import "@fortawesome/fontawesome-free/css/all.min.css";
import SvgIcon from "@/components/icons/SvgIcon.vue";
import { useAuthStore } from "@/stores/auth";
import { logout } from "@/api/auth";
import { toast } from "@/composables/useToast";
import {
  getNextThemePref,
  getThemePref,
  getThemeToggleTitle,
  setThemePref,
  supportsThemeToggle,
  type ThemePref,
} from "@/utils/theme";
import { ref } from "vue";

const route = useRoute();
const router = useRouter();
const auth = useAuthStore();

// 仅缓存影视相关页面（返回保留状态）；网盘页 IndexView 不缓存，避免启动数据不刷新。
const keepAlivePages = ["MediaLibraryView", "MovieDetailView", "MagnetSearchView", "AdminView"];

// 管理后台子导航（对应 AdminView 的 nav 定义）
const adminNav = [
  { key: "dashboard", label: "仪表盘", icon: "fa-solid fa-gauge-high" },
  { key: "accounts", label: "存储管理", icon: "fa-solid fa-hard-drive" },
  { key: "settings", label: "系统设置", icon: "fa-solid fa-gear" },
  { key: "tasks", label: "任务管理", icon: "fa-solid fa-list-check" },
  { key: "tools", label: "辅助工具", icon: "fa-solid fa-toolbox" },
  { key: "cross-transfer", label: "跨盘秒传", icon: "fa-solid fa-right-left" },
  { key: "share", label: "文件共享", icon: "fa-solid fa-share-nodes" },
];
const adminKeys = adminNav.map((n) => n.key);

const isAdminRoute = computed(() => route.path.startsWith("/admin"));

// 当前管理子页（无 query 或非法时回落仪表盘）
const adminActive = computed(() => {
  if (!isAdminRoute.value) return "";
  const raw = String(route.query.page ?? "");
  return adminKeys.includes(raw) ? raw : "dashboard";
});

const lockedKeys = computed(() =>
  auth.mustChangePassword ? adminKeys.filter((k) => k !== "settings") : [],
);

// 主题
const theme = ref<ThemePref>(getThemePref());
const themeToggleTitle = computed(() => getThemeToggleTitle(theme.value));
const showThemeToggle = computed(() => supportsThemeToggle());

function toggleTheme() {
  theme.value = getNextThemePref(theme.value);
  setThemePref(theme.value);
}

async function handleLogout() {
  try {
    await logout();
  } catch {
    /* 即使接口失败也清本地状态 */
  }
  auth.clear();
  toast.success("已退出登录");
  await router.push("/login");
}

</script>

<template>
  <div class="app-shell">
    <aside class="app-nav">
      <nav class="app-nav__list" aria-label="主导航">
        <!-- 主页面：网盘 / 影视 / 磁力 / 管理 -->
        <RouterLink
          to="/"
          class="app-nav__btn"
          :class="{ 'is-active': route.path === '/' }"
          aria-label="网盘"
        >
          <SvgIcon name="cloud" :size="22" class="app-nav__icon" />
          <span class="app-nav__label">网盘</span>
        </RouterLink>
        <RouterLink
          to="/movies"
          class="app-nav__btn"
          :class="{ 'is-active': route.path === '/movies' }"
          aria-label="影视"
        >
          <SvgIcon name="video" :size="22" class="app-nav__icon" />
          <span class="app-nav__label">影视</span>
        </RouterLink>

        <!-- 磁力搜索（登录态可用，独立页） -->
        <RouterLink
          v-if="auth.sessionAdmin"
          to="/magnet"
          class="app-nav__btn"
          :class="{ 'is-active': route.path === '/magnet' }"
          title="磁力搜索"
          aria-label="磁力搜索"
        >
          <svg viewBox="0 0 24 24" class="app-nav__magnet" aria-hidden="true">
            <path d="M6 8v5a6 6 0 0 0 12 0V8" />
            <path d="M6 4v4" />
            <path d="M18 4v4" />
          </svg>
          <span class="app-nav__label">磁力</span>
        </RouterLink>

        <RouterLink
          to="/admin"
          class="app-nav__btn"
          :class="{ 'is-active': isAdminRoute }"
          aria-label="管理"
        >
          <SvgIcon name="settings" :size="22" class="app-nav__icon" />
          <span class="app-nav__label">管理</span>
        </RouterLink>

        <!-- 管理后台子导航 -->
        <template v-if="isAdminRoute">
          <div class="app-nav__divider" aria-hidden="true" />
          <button
            v-for="item in adminNav"
            :key="item.key"
            type="button"
            class="app-nav__btn app-nav__btn--sub"
            :class="{ 'is-active': adminActive === item.key }"
            :disabled="lockedKeys.includes(item.key)"
            :title="lockedKeys.includes(item.key) ? '请先修改管理员密码' : item.label"
            @click="router.push({ path: '/admin', query: { page: item.key } })"
          >
            <i :class="item.icon" class="app-nav__subicon" aria-hidden="true"></i>
            <span class="app-nav__label">{{ item.label }}</span>
          </button>
        </template>
      </nav>

      <!-- 底部：主题 / 通知 / 退出（管理后台时显示） -->
      <div v-if="isAdminRoute" class="app-nav__foot">
        <button
          v-if="showThemeToggle"
          type="button"
          class="app-nav__iconbtn"
          :title="themeToggleTitle"
          :aria-label="themeToggleTitle"
          @click="toggleTheme"
        >
          <svg v-if="theme === 'light'" viewBox="0 0 24 24" aria-hidden="true">
            <circle cx="12" cy="12" r="4" />
            <path d="M12 2v2" /><path d="M12 20v2" />
            <path d="m4.93 4.93 1.41 1.41" /><path d="m17.66 17.66 1.41 1.41" />
            <path d="M2 12h2" /><path d="M20 12h2" />
            <path d="m6.34 17.66-1.41 1.41" /><path d="m19.07 4.93-1.41 1.41" />
          </svg>
          <svg v-else-if="theme === 'dark'" viewBox="0 0 24 24" aria-hidden="true">
            <path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z" />
          </svg>
          <svg v-else viewBox="0 0 24 24" aria-hidden="true">
            <rect x="2" y="3" width="20" height="14" rx="2" />
            <path d="M8 21h8" /><path d="M12 17v4" />
          </svg>
        </button>
        <button
          type="button"
          class="app-nav__iconbtn"
          title="退出登录"
          aria-label="退出登录"
          @click="handleLogout"
        >
          <SvgIcon name="sign-out" :size="17" />
        </button>
      </div>
    </aside>

    <div class="app-body">
      <main class="app-main">
        <RouterView v-slot="{ Component }">
          <!-- Transition 外层 + KeepAlive 内层 + key 放组件上：
               KeepAlive 按 (组件,key) 缓存页面，返回时保留库选择/筛选/滚动等状态。
               无 mode + 离场 transition:none，避免异步路由离场竞态卡住 -->
          <Transition name="page">
            <KeepAlive :include="keepAlivePages">
              <component :is="Component" :key="$route.path" />
            </KeepAlive>
          </Transition>
        </RouterView>
      </main>
    </div>
  </div>
</template>

<style scoped>
.app-shell {
  display: flex;
  min-height: 100vh;
  background: var(--bg);
}

/* 左侧导航栏 */
.app-nav {
  position: sticky;
  top: 0;
  flex: 0 0 96px;
  height: 100vh;
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 20px 8px 14px;
  border-right: 1px solid var(--border-soft);
  background: var(--surface);
  z-index: 90;
  box-sizing: border-box;
}

.app-nav__list {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  gap: 8px;
  width: 100%;
  margin-top: 16px;
  overflow-y: auto;
  scrollbar-width: thin;
}

.app-nav__btn {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 6px;
  padding: 12px 4px 10px;
  border: none;
  border-radius: 14px;
  background: transparent;
  color: var(--text-muted);
  text-decoration: none;
  font-size: 12px;
  font-weight: 600;
  letter-spacing: 0.02em;
  cursor: pointer;
  transition: background 0.18s ease, color 0.18s ease, transform 0.18s ease;
}

.app-nav__btn:hover:not(:disabled) {
  background: var(--surface-sunken);
  color: var(--text);
  transform: translateY(-1px);
}

.app-nav__btn.is-active {
  background: color-mix(in srgb, var(--brand) 14%, var(--surface));
  color: var(--brand-strong, var(--brand));
  box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--brand) 28%, transparent);
}

.app-nav__btn:disabled {
  opacity: 0.45;
  cursor: not-allowed;
}

.app-nav__icon {
  display: block;
}

.app-nav__magnet {
  width: 22px;
  height: 22px;
  stroke: currentColor;
  stroke-width: 2;
  fill: none;
  stroke-linecap: round;
  stroke-linejoin: round;
}

.app-nav__subicon {
  font-size: 18px;
  line-height: 1;
}

.app-nav__divider {
  width: 36px;
  height: 1px;
  margin: 6px auto;
  background: var(--border-soft);
}

.app-nav__foot {
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  gap: 6px;
  align-items: center;
  padding-top: 10px;
  border-top: 1px solid var(--border-soft);
  width: 100%;
}

.app-nav__iconbtn {
  width: 40px;
  height: 40px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: none;
  border-radius: 12px;
  background: transparent;
  color: var(--text-muted);
  cursor: pointer;
  transition: background 0.18s ease, color 0.18s ease;
}

.app-nav__iconbtn:hover {
  background: var(--surface-sunken);
  color: var(--text);
}

.app-nav__iconbtn svg {
  width: 18px;
  height: 18px;
  stroke: currentColor;
  stroke-width: 2;
  fill: none;
  stroke-linecap: round;
  stroke-linejoin: round;
}

.app-body {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
}

.app-main {
  flex: 1;
  min-width: 0;
}

/* 页面切换动画：新页淡入右移；旧页立即让位（无离场动画，规避异步路由卡顿） */
.page-enter-active {
  transition: opacity 0.3s ease, transform 0.3s ease;
}

.page-enter-from {
  opacity: 0;
  transform: translateX(24px);
}

.page-leave-active {
  transition: none;
}

.page-leave-to {
  opacity: 0;
}

@media (max-width: 767px) {
  .page-enter-active {
    transition: opacity 0.2s ease;
  }
  .page-enter-from {
    transform: none;
  }
}

/* 移动端：底部 Tab 栏 + 管理子导航横向条 */
@media (max-width: 767px) {
  .app-shell {
    flex-direction: column;
  }

  .app-nav {
    position: fixed;
    top: auto;
    bottom: 0;
    left: 0;
    right: 0;
    flex: none;
    width: 100%;
    height: 58px;
    padding: 0 8px;
    flex-direction: row;
    justify-content: center;
    align-items: center;
    border-right: none;
    border-top: 1px solid var(--border-soft);
    padding-bottom: env(safe-area-inset-bottom, 0);
    z-index: 90;
  }

  .app-nav__list {
    flex-direction: row;
    justify-content: space-around;
    align-items: center;
    gap: 6px;
    width: 100%;
    max-width: 360px;
    margin-top: 0;
    overflow: visible;
  }

  .app-nav__btn {
    flex: 1;
    flex-direction: column;
    padding: 6px 4px;
    border-radius: 10px;
  }

  .app-nav__divider {
    display: none;
  }

  .app-nav__btn--sub {
    display: none;
  }

  .app-nav__foot {
    display: none;
  }

  .app-body {
    padding-bottom: calc(58px + env(safe-area-inset-bottom, 0));
  }
}
</style>
