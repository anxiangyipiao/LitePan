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

/* 内嵌为标签页后，子页面不再撑满整屏 */
.magnet-hub__panel :deep(.magnet-page),
.magnet-hub__panel :deep(.subscribe-page) {
  min-height: auto;
}
</style>
