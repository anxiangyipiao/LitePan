<script setup lang="ts">
import { ref, watch } from "vue";
import AppModal from "@/components/base/AppModal.vue";
import AppButton from "@/components/base/AppButton.vue";
import BusySpinner from "@/components/base/BusySpinner.vue";
import { searchMagnet, type MagnetSearchResult } from "@/api/magnetSearch";
import { addToQB } from "@/api/qb";
import { getApiErrorMessage } from "@/api/client";
import { formatSize } from "@/utils/format";
import { toast, copyTextToClipboard } from "@/composables/useToast";
import MagnetOfflineModal from "@/components/common/MagnetOfflineModal.vue";
import { useMagnetSites } from "@/composables/useMagnetSites";

const props = defineProps<{ open: boolean }>();
const emit = defineEmits<{ close: [] }>();

const { sites: magnetSites, load: loadSites, toggle: toggleMagnetSite, persistEnabled: persistMagnetSites } = useMagnetSites();

const keyword = ref("");
const results = ref<MagnetSearchResult[]>([]);
const loading = ref(false);
const error = ref("");
const searched = ref(false);
const qbPushing = ref<Record<number, boolean>>({});
const offlineOpen = ref(false);
const offlineMagnet = ref("");
const offlineName = ref("");
const isFullscreen = ref(false);
const sitePickerOpen = ref(false);

watch(
  () => props.open,
  (open) => {
    if (open) {
      // 重新打开回到初始态，避免残留上次搜索的「无结果」空态
      error.value = "";
      searched.value = false;
      results.value = [];
      void loadSites();
    }
  },
);

function sourceLabel(src?: string): string {
  if (!src) return "";
  if (src === "sukebei") return "Sukebei";
  if (src === "nyaa") return "Nyaa";
  if (src === "sukebei_cn") return "Sukebei CN";
  if (src.startsWith("custom:")) return src.slice("custom:".length);
  const s = magnetSites.value.find((x) => x.id === src);
  return s ? s.label : src;
}

async function onToggleSite(id: string) {
  toggleMagnetSite(id);
  try {
    await persistMagnetSites();
  } catch (e) {
    toast.error(getApiErrorMessage(e, "保存镜像配置失败"));
  }
}

async function search() {
  const q = keyword.value.trim();
  if (!q) {
    toast.warning("请输入搜索关键词");
    return;
  }
  const enabledCount = magnetSites.value.filter((s) => s.enabled).length;
  if (magnetSites.value.length > 0 && enabledCount === 0) {
    toast.warning("请至少启用一个磁力镜像");
    sitePickerOpen.value = true;
    return;
  }
  loading.value = true;
  error.value = "";
  try {
    results.value = (await searchMagnet(q)) ?? [];
    searched.value = true;
  } catch (e) {
    error.value = getApiErrorMessage(e, "搜索失败，请检查站点地址或代理配置");
    results.value = [];
  } finally {
    loading.value = false;
  }
}

async function copyMagnet(r: MagnetSearchResult) {
  if (!r.magnet) {
    toast.warning("该结果没有磁力链");
    return;
  }
  await copyTextToClipboard(r.magnet, { successMessage: "磁力链已复制" });
}

async function pushToQB(r: MagnetSearchResult) {
  if (!r.magnet) {
    toast.warning("该结果没有磁力链");
    return;
  }
  const trackKey = r.id || 0;
  qbPushing.value[trackKey] = true;
  try {
    await addToQB(r.magnet);
    toast.success("已推送到 qBittorrent");
  } catch (e) {
    toast.error(getApiErrorMessage(e, "推送到 qB 失败，请检查 qB 地址与账号配置"));
  } finally {
    qbPushing.value[trackKey] = false;
  }
}

function openOffline(r: MagnetSearchResult) {
  if (!r.magnet) {
    toast.warning("该结果没有磁力链");
    return;
  }
  offlineMagnet.value = r.magnet;
  offlineName.value = r.name;
  offlineOpen.value = true;
}

function formatDate(unix: number): string {
  if (!unix) return "-";
  const d = new Date(unix * 1000);
  if (Number.isNaN(d.getTime())) return "-";
  const diff = Date.now() - d.getTime();
  if (diff < 60_000) return "刚刚";
  if (diff < 3_600_000) return `${Math.floor(diff / 60_000)} 分钟前`;
  if (diff < 86_400_000) return `${Math.floor(diff / 3_600_000)} 小时前`;
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
}
</script>

<template>
  <AppModal
    :open="open"
    title="磁力搜索"
    size="lg"
    :fullscreen="isFullscreen"
    @close="emit('close')"
  >
    <template #header>
      <h3 class="modal__title">磁力搜索</h3>
      <div class="magnet-search__header-actions">
        <button
          type="button"
          class="magnet-search__fs-btn"
          :title="isFullscreen ? '退出全屏' : '全屏'"
          @click.stop="isFullscreen = !isFullscreen"
        >
          <i :class="isFullscreen ? 'fa-solid fa-compress' : 'fa-solid fa-expand'" aria-hidden="true"></i>
        </button>
        <button class="modal__close" aria-label="关闭" @click="emit('close')">×</button>
      </div>
    </template>

    <div class="magnet-search">
      <div class="magnet-search__bar">
        <input
          v-model="keyword"
          class="magnet-search__input"
          type="search"
          placeholder="输入番号或关键词，回车搜索"
          @keydown.enter="search"
        />
        <AppButton type="button" variant="secondary" title="选择搜索镜像" @click="sitePickerOpen = !sitePickerOpen">
          <i class="fa-solid fa-server" aria-hidden="true"></i>
          镜像
        </AppButton>
        <AppButton type="button" variant="primary" :disabled="loading" @click="search">
          {{ loading ? "搜索中…" : "搜索" }}
        </AppButton>
      </div>

      <div v-if="sitePickerOpen" class="magnet-search__site-picker" @click.stop>
        <div class="magnet-search__site-head">
          <span>选择搜索镜像</span>
          <button class="magnet-search__site-close" aria-label="关闭" @click="sitePickerOpen = false">×</button>
        </div>
        <div v-if="magnetSites.length === 0" class="magnet-search__site-empty">加载中…</div>
        <label v-for="s in magnetSites" :key="s.id" class="magnet-search__site-item">
          <input type="checkbox" :checked="s.enabled" @change="onToggleSite(s.id)" />
          <span class="magnet-search__site-label">{{ s.label }}</span>
          <span class="magnet-search__site-url">{{ s.base_url }}</span>
        </label>
      </div>

      <p v-if="error" class="magnet-search__error">{{ error }}</p>

      <div v-if="!loading && !error && !searched" class="magnet-search__empty">
        输入番号或关键词，搜索磁力资源
      </div>
      <div v-else-if="!loading && !error && searched && results.length === 0" class="magnet-search__empty">
        没有找到结果，换个关键词试试
      </div>

      <div v-if="loading" class="magnet-search__loading">
        <BusySpinner :size="22" />
        <span>正在搜索…</span>
      </div>

      <ul v-if="results.length" class="magnet-search__list">
        <li v-for="r in results" :key="r.id" class="magnet-search__row">
          <div class="magnet-search__main">
            <div class="magnet-search__name" :title="r.name">{{ r.name }}</div>
            <div class="magnet-search__meta">
              <span v-if="r.source" class="magnet-search__meta-item magnet-search__source">{{ sourceLabel(r.source) }}</span>
              <span v-if="r.category" class="magnet-search__meta-item">{{ r.category }}</span>
              <span class="magnet-search__meta-item">{{ formatSize(r.size) }}</span>
              <span class="magnet-search__meta-item" :title="'做种 ' + r.seeders">↑{{ r.seeders }}</span>
              <span class="magnet-search__meta-item" :title="'下载 ' + r.leechers">↓{{ r.leechers }}</span>
              <span class="magnet-search__meta-item">{{ formatDate(r.date) }}</span>
            </div>
          </div>
          <div class="magnet-search__actions">
            <AppButton type="button" variant="secondary" size="sm" @click="copyMagnet(r)">
              复制
            </AppButton>
            <AppButton
              type="button"
              variant="primary"
              size="sm"
              :disabled="!!qbPushing[r.id]"
              @click="pushToQB(r)"
            >
              {{ qbPushing[r.id] ? "推送中…" : "下载到 qB" }}
            </AppButton>
          <AppButton type="button" variant="secondary" size="sm" @click="openOffline(r)">
            离线到网盘
          </AppButton>
            <a
              v-if="r.view_url"
              :href="r.view_url"
              target="_blank"
              rel="noopener"
              class="magnet-search__link"
            >
              详情
            </a>
          </div>
        </li>
      </ul>
    </div>
  </AppModal>
  <MagnetOfflineModal :open="offlineOpen" :magnet="offlineMagnet" :magnet-name="offlineName" @close="offlineOpen = false" />
</template>

<style scoped>
.magnet-search {
  display: flex;
  flex-direction: column;
  gap: 12px;
  min-height: 260px;
  max-height: 60vh;
}

.magnet-search__bar {
  display: flex;
  gap: 8px;
}

.magnet-search__input {
  flex: 1 1 auto;
  min-width: 0;
  height: 38px;
  padding: 0 12px;
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  background: var(--surface);
  color: var(--text);
  font-size: 14px;
  outline: none;
}
.magnet-search__input:focus {
  border-color: var(--brand);
}

.magnet-search__error {
  margin: 0;
  color: var(--danger);
  font-size: 13px;
}

.magnet-search__empty {
  flex: 1;
  display: grid;
  place-items: center;
  color: var(--text-muted);
  font-size: 13px;
  text-align: center;
  padding: 40px 0;
}

.magnet-search__loading {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 10px;
  color: var(--text-muted);
  font-size: 13px;
}

.magnet-search__list {
  list-style: none;
  margin: 0;
  padding: 0;
  overflow-y: auto;
  border: 1px solid var(--border-soft);
  border-radius: var(--radius-sm);
}

.magnet-search__row {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 12px;
  border-bottom: 1px solid var(--border-soft);
}
.magnet-search__row:last-child {
  border-bottom: none;
}

.magnet-search__main {
  flex: 1 1 auto;
  min-width: 0;
}

.magnet-search__name {
  font-size: 13px;
  font-weight: 600;
  color: var(--text);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.magnet-search__meta {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 4px 10px;
  margin-top: 4px;
  font-size: 12px;
  color: var(--text-muted);
}

.magnet-search__actions {
  flex: 0 0 auto;
  display: flex;
  align-items: center;
  gap: 8px;
}

.magnet-search__link {
  color: var(--brand);
  font-size: 13px;
  text-decoration: none;
  white-space: nowrap;
}

.magnet-search__header-actions {
  display: flex;
  align-items: center;
  gap: 4px;
}

.magnet-search__fs-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  border: none;
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--text-muted);
  font-size: 14px;
  cursor: pointer;
  transition: color 0.2s ease, background-color 0.2s ease;
}
.magnet-search__fs-btn:hover {
  color: var(--text);
  background: var(--surface-sunken);
}

:deep(.modal--fullscreen) .magnet-search {
  max-height: calc(100vh - 120px);
}
:deep(.modal--fullscreen) .magnet-search__list {
  max-height: calc(100vh - 200px);
  min-height: 0;
}

/* 镜像多选 popover */
.magnet-search__site-picker {
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  padding: 10px 14px 12px;
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.magnet-search__site-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 12px;
  font-weight: 600;
  margin-bottom: 4px;
  color: var(--text);
}
.magnet-search__site-close {
  background: transparent;
  border: none;
  color: var(--text-muted);
  font-size: 18px;
  line-height: 1;
  cursor: pointer;
  padding: 0 4px;
}
.magnet-search__site-close:hover {
  color: var(--text);
}
.magnet-search__site-item {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 12px;
  cursor: pointer;
  padding: 3px 0;
}
.magnet-search__site-item input[type="checkbox"] {
  width: 14px;
  height: 14px;
  accent-color: var(--brand);
  cursor: pointer;
}
.magnet-search__site-label {
  font-weight: 600;
  min-width: 84px;
}
.magnet-search__site-url {
  color: var(--text-muted);
  font-family: var(--font-mono, monospace);
  word-break: break-all;
}
.magnet-search__site-empty {
  font-size: 12px;
  color: var(--text-muted);
}
.magnet-search__source {
  background: rgba(124, 92, 255, 0.12);
  color: #6c4ce6;
  padding: 1px 6px;
  border-radius: 4px;
  font-weight: 600;
}
</style>
