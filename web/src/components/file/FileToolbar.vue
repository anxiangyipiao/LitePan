<script setup lang="ts">
import { computed, ref } from "vue";
import SvgIcon from "@/components/icons/SvgIcon.vue";
import AppButton from "@/components/base/AppButton.vue";
import AppDropdown from "@/components/base/AppDropdown.vue";
import { usePerformancePanel } from "@/composables/usePerformancePanel";
import type { DropdownMenuItem } from "@/types/menu";

const emit = defineEmits<{
  refresh: [];
  "update:view": ["list" | "grid"];
  "create-folder": [];
  "upload-file": [];
  "upload-folder": [];
  "offline-download": [];
  "batch-move": [];
  "batch-copy": [];
  "batch-delete": [];
  "toggle-favorites": [];
}>();

const { expanded: performanceExpanded, toggle: togglePerformancePanel } = usePerformancePanel();

const createOpen = ref(false);
const transferOpen = ref(false);

const props = defineProps<{
  isAdmin: boolean;
  selectedCount: number;
  view: "list" | "grid";
  refreshing: boolean;
  responseTime: string;
  cacheRate: string;
  favoritesOpen?: boolean;
  offlineDownloadSupported?: boolean;
}>();

const createItems = computed<DropdownMenuItem[]>(() => {
  const items: DropdownMenuItem[] = [
    { key: "create-folder", label: "新建文件夹", icon: "folder", type: "action" },
    { key: "upload-file", label: "上传文件", icon: "file", type: "action" },
    { key: "upload-folder", label: "上传文件夹", icon: "folder-open", type: "action" },
  ];
  if (props.offlineDownloadSupported) {
    items.push({ key: "offline-download", label: "离线下载", icon: "cloud", type: "action" });
  }
  return items;
});

const transferItems = computed<DropdownMenuItem[]>(() => [
  { key: "batch-move", label: "批量移动", icon: "package", type: "action" },
  { key: "batch-copy", label: "批量复制", icon: "copy", type: "action" },
]);

function onCreateSelect(key: string) {
  if (key === "create-folder") emit("create-folder");
  else if (key === "upload-file") emit("upload-file");
  else if (key === "upload-folder") emit("upload-folder");
  else emit("offline-download");
}

function onTransferSelect(key: string) {
  if (key === "batch-move") emit("batch-move");
  else emit("batch-copy");
}
</script>

<template>
  <div class="file-toolbar">
    <div class="file-toolbar__left">
      <button
        v-if="isAdmin"
        type="button"
        class="file-toolbar__favorites-toggle"
        :class="{ active: favoritesOpen }"
        :title="favoritesOpen ? '关闭收藏夹' : '打开收藏夹'"
        aria-label="切换收藏夹显示"
        @click="emit('toggle-favorites')"
      >
        <SvgIcon
          name="chevron-down"
          :size="16"
          class-name="file-toolbar__favorites-toggle-icon"
        />
      </button>

      <AppDropdown
        v-if="isAdmin"
        v-model:open="createOpen"
        trigger="hover"
        hover-bridge
        align="left"
        :items="createItems"
        density="comfortable"
        @select="onCreateSelect"
      >
        <template #trigger="{ open, toggle }">
          <AppButton
            variant="primary"
            class="file-toolbar__menu-trigger"
            :aria-expanded="open"
            @click="toggle"
          >
            <span class="file-toolbar__menu-main">
              <span class="file-toolbar__icon"><SvgIcon name="folder" :size="17" /></span>
              <span class="file-toolbar__menu-label">新建</span>
            </span>
            <span class="file-toolbar__menu-arrow" :class="{ open }">
              <SvgIcon name="chevron-down" :size="14" />
            </span>
          </AppButton>
        </template>
      </AppDropdown>

      <AppButton variant="secondary" class="file-toolbar__btn file-toolbar__btn--refresh" :disabled="refreshing" @click="emit('refresh')">
        <span class="file-toolbar__icon" :class="{ spin: refreshing }">
          <SvgIcon name="refresh" :size="17" />
        </span>
        <span>刷新</span>
      </AppButton>

      <div v-if="isAdmin && selectedCount > 0" class="file-toolbar__batch">
        <AppDropdown
          v-model:open="transferOpen"
          trigger="hover"
          hover-bridge
          align="left"
          :items="transferItems"
          density="comfortable"
          @select="onTransferSelect"
        >
          <template #trigger="{ open, toggle }">
            <AppButton variant="secondary" class="file-toolbar__btn" :aria-expanded="open" @click="toggle">
              <span class="file-toolbar__icon"><SvgIcon name="package" :size="17" /></span>
              <span>转移/复制</span>
              <span class="file-toolbar__menu-arrow file-toolbar__menu-arrow--muted" :class="{ open }">
                <SvgIcon name="chevron-down" :size="14" />
              </span>
            </AppButton>
          </template>
        </AppDropdown>

        <AppButton variant="danger" class="file-toolbar__btn" @click="emit('batch-delete')">
          <span class="file-toolbar__icon"><SvgIcon name="trash-button" :size="17" /></span>
          <span>批量删除</span>
        </AppButton>
      </div>
    </div>

    <div class="file-toolbar__right">
      <div class="performance-panel" :class="{ expanded: performanceExpanded }">
        <div class="performance-metrics" :aria-hidden="!performanceExpanded">
          <span class="performance-metric">
            <span class="performance-label">响应时间</span>
            <span class="performance-value">{{ responseTime }}</span>
          </span>
          <span class="performance-divider" />
          <span class="performance-metric">
            <span class="performance-label">缓存命中率</span>
            <span class="performance-value">{{ cacheRate }}</span>
          </span>
        </div>
        <button
          type="button"
          class="performance-toggle"
          :aria-expanded="performanceExpanded"
          :title="performanceExpanded ? '收起性能信息' : '展开性能信息'"
          @click="togglePerformancePanel"
        >
          <span class="file-toolbar__icon"><SvgIcon name="lightning" :size="17" /></span>
        </button>
      </div>

      <div class="view-mode-switch" role="group" aria-label="文件视图切换">
        <button
          type="button"
          class="view-mode-btn"
          :class="{ active: view === 'list' }"
          title="列表视图"
          @click="emit('update:view', 'list')"
        >
          <span class="view-icon list" />
        </button>
        <button
          type="button"
          class="view-mode-btn"
          :class="{ active: view === 'grid' }"
          title="网格视图"
          @click="emit('update:view', 'grid')"
        >
          <span class="view-icon grid" />
        </button>
      </div>
    </div>
  </div>
</template>
