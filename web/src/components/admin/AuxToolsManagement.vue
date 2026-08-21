<script setup lang="ts">
import { computed, defineAsyncComponent, nextTick, onDeactivated, ref, watchEffect } from "vue";
import AppButton from "@/components/base/AppButton.vue";
import SectionTabBar from "@/components/admin/SectionTabBar.vue";
import AdminSettingsDrawer from "@/components/admin/AdminSettingsDrawer.vue";
import type StrmScrapePanelComponent from "@/components/admin/StrmScrapePanel.vue";
import { useSectionTabRoute } from "@/composables/useSectionTabRoute";
import { useSettingsPageDirty } from "@/composables/useSettingsPageDirty";
import { readPanelSaving, type SettingsPanelExpose } from "@/composables/useSettingsForm";
import "@/styles/admin-shared.css";

import BusySpinner from "@/components/base/BusySpinner.vue";

function lazyAuxPanel(loader: () => Promise<unknown>) {
  return defineAsyncComponent({
    loader: loader as never,
    loadingComponent: BusySpinner,
    delay: 200,
    timeout: 15000,
  });
}
const StrmScrapePanel = lazyAuxPanel(() => import("@/components/admin/StrmScrapePanel.vue"));
const StrmScrapeSettings = lazyAuxPanel(() => import("@/components/admin/StrmScrapeSettings.vue"));
const ProxyToolsPanel = lazyAuxPanel(() => import("@/components/admin/ProxyToolsPanel.vue"));

const SCRAPE_TAB = "scrape";
const PROXY_TAB = "proxy";
const tabs = [
  { key: SCRAPE_TAB, label: "STRM 刮削" },
  { key: PROXY_TAB, label: "反代工具" },
];

const settingsDrawerOpen = ref(false);
const scrapeSettingsVisited = ref(false);
const scrapePanelRef = ref<InstanceType<typeof StrmScrapePanelComponent> | null>(null);
const scrapeSettingsRef = ref<SettingsPanelExpose | null>(null);
const scrapePanelDirty = ref(false);

watchEffect(() => {
  scrapePanelDirty.value = (scrapeSettingsRef.value as SettingsPanelExpose | null)?.isDirty?.() ?? false;
});

const settingsPageDirty = computed(() => settingsDrawerOpen.value && scrapePanelDirty.value);

function revertDrawerSettings() {
  scrapeSettingsRef.value?.revert?.();
}

const { confirmDiscardChanges } = useSettingsPageDirty(settingsPageDirty, revertDrawerSettings);

const { activeTab, setActiveTab } = useSectionTabRoute(
  SCRAPE_TAB,
  [SCRAPE_TAB, PROXY_TAB],
  {
    beforeTabChange: async () => {
      if (!settingsDrawerOpen.value) return true;
      const ok = await confirmDiscardChanges(() => scrapePanelDirty.value);
      if (!ok) return false;
      settingsDrawerOpen.value = false;
      return true;
    },
  },
);

const drawerSaving = computed(() => readPanelSaving(scrapeSettingsRef.value?.saving));
const isScrapeTab = computed(() => activeTab.value === SCRAPE_TAB);
const isProxyTab = computed(() => activeTab.value === PROXY_TAB);

async function openScrapeSettings() {
  scrapeSettingsVisited.value = true;
  settingsDrawerOpen.value = true;
  await nextTick();
  if (scrapeSettingsRef.value && !scrapeSettingsRef.value.isDirty?.()) {
    await scrapeSettingsRef.value.reload?.();
  }
}

async function closeSettingsDrawer() {
  if (!(await confirmDiscardChanges(() => scrapePanelDirty.value))) return;
  settingsDrawerOpen.value = false;
}

onDeactivated(() => {
  if (!settingsDrawerOpen.value) return;
  if (scrapePanelDirty.value) revertDrawerSettings();
  settingsDrawerOpen.value = false;
});

async function handleDrawerSave() {
  await scrapeSettingsRef.value?.save?.();
}
</script>

<template>
  <div class="settings">
    <SectionTabBar :model-value="activeTab" :tabs="tabs" @update:model-value="setActiveTab">
      <template v-if="isScrapeTab" #actions>
        <AppButton
          v-if="scrapePanelRef?.running"
          type="button"
          variant="secondary"
          @click="scrapePanelRef?.stopScrape()"
        >
          停止刮削
        </AppButton>
        <AppButton v-else type="button" variant="primary" @click="scrapePanelRef?.startScrape()">
          开始刮削
        </AppButton>
      </template>
    </SectionTabBar>

    <StrmScrapePanel v-if="isScrapeTab" ref="scrapePanelRef" @open-settings="openScrapeSettings" />
    <ProxyToolsPanel v-else-if="isProxyTab" />

    <AdminSettingsDrawer
      :open="settingsDrawerOpen"
      title="STRM 刮削设置"
      :saving="drawerSaving"
      :can-save="scrapePanelDirty"
      @close="closeSettingsDrawer"
      @cancel="closeSettingsDrawer"
      @save="handleDrawerSave"
    >
      <StrmScrapeSettings v-if="scrapeSettingsVisited" ref="scrapeSettingsRef" />
    </AdminSettingsDrawer>
  </div>
</template>
