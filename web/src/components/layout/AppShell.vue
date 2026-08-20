<script setup lang="ts">
import { RouterLink, RouterView } from "vue-router";
import SvgIcon from "@/components/icons/SvgIcon.vue";
import AppFooter from "@/components/layout/AppFooter.vue";

const navItems = [
  { to: "/", name: "网盘", icon: "cloud", exact: true },
  { to: "/movies", name: "影视", icon: "video", exact: false },
  { to: "/admin", name: "管理", icon: "settings", exact: false },
];
</script>

<template>
  <div class="app-shell">
    <aside class="app-nav">
      <nav class="app-nav__list" aria-label="主导航">
        <RouterLink
          v-for="item in navItems"
          :key="item.to"
          :to="item.to"
          class="app-nav__btn"
          :class="{ 'is-active': item.exact ? $route.path === item.to : $route.path.startsWith(item.to) }"
          :aria-label="item.name"
        >
          <SvgIcon :name="item.icon" :size="22" class="app-nav__icon" />
          <span class="app-nav__label">{{ item.name }}</span>
        </RouterLink>
      </nav>
    </aside>

    <div class="app-body">
      <main class="app-main">
        <RouterView v-slot="{ Component }">
          <Transition name="page" mode="out-in">
            <component :is="Component" :key="$route.path" />
          </Transition>
        </RouterView>
      </main>
      <AppFooter />
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
  flex: 0 0 88px;
  height: 100vh;
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 20px 8px;
  border-right: 1px solid var(--border-soft);
  background: var(--surface);
  z-index: 90;
  box-sizing: border-box;
}

.app-nav__list {
  display: flex;
  flex-direction: column;
  gap: 10px;
  width: 100%;
  margin-top: 20px;
}

.app-nav__btn {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 6px;
  padding: 14px 4px 12px;
  border-radius: 14px;
  color: var(--text-muted);
  text-decoration: none;
  font-size: 12px;
  font-weight: 600;
  letter-spacing: 0.02em;
  transition: background 0.18s ease, color 0.18s ease, transform 0.18s ease;
}

.app-nav__btn:hover {
  background: var(--surface-sunken);
  color: var(--text);
  transform: translateY(-1px);
}

.app-nav__btn.is-active {
  background: color-mix(in srgb, var(--brand) 14%, var(--surface));
  color: var(--brand-strong, var(--brand));
  box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--brand) 28%, transparent);
}

.app-nav__icon {
  display: block;
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

/* 页面切换动画 */
.page-enter-active,
.page-leave-active {
  transition: opacity 0.26s ease, transform 0.26s ease;
}

.page-enter-from {
  opacity: 0;
  transform: translateX(28px);
}

.page-leave-to {
  opacity: 0;
  transform: translateX(-16px);
}

/* 移动端：底部 Tab 栏 */
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
  }

  .app-nav__list {
    flex-direction: row;
    justify-content: space-around;
    align-items: center;
    gap: 6px;
    width: 100%;
    max-width: 360px;
    margin-top: 0;
  }

  .app-nav__btn {
    flex: 1;
    flex-direction: column;
    padding: 6px 4px;
    border-radius: 10px;
  }

  .app-body {
    padding-bottom: 58px;
  }
}
</style>
