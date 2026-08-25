<script setup lang="ts">
import { useSectionTabRoute } from "@/composables/useSectionTabRoute";
import MagnetSearchView from "./MagnetSearchView.vue";
import SubscribeView from "./SubscribeView.vue";

const TABS = [
  { key: "search", label: "磁力搜索" },
  { key: "subscribe", label: "订阅追番" },
] as const;

const { activeTab, setActiveTab } = useSectionTabRoute("search", ["search", "subscribe"]);
</script>

<template>
  <div class="magnet-hub">
    <div class="magnet-hub__tabs">
      <div class="magnet-hub__tabs-inner">
        <button
          v-for="t in TABS"
          :key="t.key"
          type="button"
          class="magnet-hub__tab"
          :class="{ 'is-active': activeTab === t.key }"
          @click="setActiveTab(t.key)"
        >
          <i :class="t.key === 'search' ? 'fa-solid fa-magnet' : 'fa-solid fa-rss'" aria-hidden="true"></i>
          {{ t.label }}
        </button>
      </div>
    </div>

    <div v-show="activeTab === 'search'" class="magnet-hub__panel">
      <MagnetSearchView />
    </div>
    <div v-show="activeTab === 'subscribe'" class="magnet-hub__panel">
      <SubscribeView :active="activeTab === 'subscribe'" />
    </div>
  </div>
</template>

<style scoped>
.magnet-hub {
  min-height: 100vh;
  min-height: 100dvh;
}

.magnet-hub__tabs {
  position: sticky;
  top: 0;
  z-index: 60;
  background: var(--surface);
  border-bottom: 1px solid var(--border-soft);
  box-shadow: var(--shadow-soft);
}

.magnet-hub__tabs-inner {
  max-width: 960px;
  margin: 0 auto;
  display: flex;
  gap: 4px;
  padding: 12px 20px 0;
  box-sizing: border-box;
}

.magnet-hub__tab {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 10px 20px;
  border: none;
  background: transparent;
  color: var(--text-muted);
  font-size: 14px;
  font-weight: 600;
  cursor: pointer;
  border-bottom: 2px solid transparent;
  transition: color 0.15s ease, border-color 0.15s ease;
}

.magnet-hub__tab i {
  font-size: 13px;
}

.magnet-hub__tab:hover {
  color: var(--text);
}

.magnet-hub__tab.is-active {
  color: var(--brand-strong, var(--brand));
  border-bottom-color: var(--brand);
}

/* 子页面包裹矩形：白底卡片 + 圆角 + 边框 + 阴影 */
.magnet-hub__panel {
  max-width: 960px;
  margin: 16px auto 24px;
  background: var(--surface);
  border-radius: var(--radius-md);
  border: 1px solid var(--border-soft);
  box-shadow: var(--shadow-card);
  overflow: hidden;
}

/* 子页面在内嵌时去掉自身外层 padding / max-width / 最小高度，让矩形容器统一控制 */
.magnet-hub__panel :deep(.magnet-page),
.magnet-hub__panel :deep(.subscribe-page) {
  min-height: auto;
  max-width: none;
  padding: 0;
  margin: 0;
  box-sizing: border-box;
}

/* hero 区作为矩形的顶部条带，顶部圆角由矩形容器负责（overflow hidden 自动裁剪），去掉 hero 自身的圆角避免双层圆角 */
.magnet-hub__panel :deep(.magnet-page__hero),
.magnet-hub__panel :deep(.subscribe-page__hero) {
  border-radius: 0;
  margin-left: 0;
  margin-right: 0;
  box-shadow: none;
}

/* hero 下的功能区（搜索栏 / 统计卡片等）在矩形内增加内边距 */
.magnet-hub__panel :deep(.magnet-page__bar),
.magnet-hub__panel :deep(.subscribe-page__stats),
.magnet-hub__panel :deep(.subscribe-page__bar) {
  margin-left: 24px;
  margin-right: 24px;
}

/* 列表/结果区保持矩形内左右内边距 */
.magnet-hub__panel :deep(.magnet-page__list),
.magnet-hub__panel :deep(.magnet-page__empty),
.magnet-hub__panel :deep(.magnet-page__error),
.magnet-hub__panel :deep(.magnet-page__loading),
.magnet-hub__panel :deep(.magnet-page__stats),
.magnet-hub__panel :deep(.subscribe-page__bar + .admin-table),
.magnet-hub__panel :deep(.subscribe-page__bar) {
  padding-left: 24px;
  padding-right: 24px;
  box-sizing: border-box;
}

/* subscribe-page 的统计卡片宽度跟 hero 一致 */
.magnet-hub__panel :deep(.subscribe-page__stats) {
  padding: 16px 24px 0;
}

/* subscribe-page 的表格与状态区在矩形内加左右内边距 */
.magnet-hub__panel :deep(.subscribe-page .admin-table),
.magnet-hub__panel :deep(.subscribe-page__error),
.magnet-hub__panel :deep(.subscribe-page__loading),
.magnet-hub__panel :deep(.subscribe-page__empty) {
  margin-left: 24px;
  margin-right: 24px;
}
.magnet-hub__panel :deep(.subscribe-page__error),
.magnet-hub__panel :deep(.subscribe-page__loading),
.magnet-hub__panel :deep(.subscribe-page__empty) {
  padding-top: 16px;
  padding-bottom: 24px;
}
.magnet-hub__panel :deep(.subscribe-page .admin-table) {
  margin-bottom: 24px;
}
</style>
