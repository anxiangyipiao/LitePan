<script setup lang="ts">
import { ref } from "vue";
import AppButton from "@/components/base/AppButton.vue";
import BusySpinner from "@/components/base/BusySpinner.vue";
import { searchMagnet, type MagnetSearchResult } from "@/api/magnetSearch";
import { addToQB } from "@/api/qb";
import { getApiErrorMessage } from "@/api/client";
import { formatSize } from "@/utils/format";
import { toast, copyTextToClipboard } from "@/composables/useToast";

const keyword = ref("");
const results = ref<MagnetSearchResult[]>([]);
const loading = ref(false);
const error = ref("");
const searched = ref(false);
const qbPushing = ref<Record<number, boolean>>({});
const offlineOpen = ref(false);
const offlineMagnet = ref("");
const offlineName = ref("");

async function search() {
  const q = keyword.value.trim();
  if (!q) {
    toast.warning("请输入搜索关键词");
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
  <div class="magnet-page">
    <header class="magnet-page__head">
      <h1 class="magnet-page__title">磁力搜索</h1>
      <p class="magnet-page__desc">输入番号或关键词，搜索磁力资源</p>
    </header>

    <div class="magnet-page__bar">
      <input
        v-model="keyword"
        class="magnet-page__input"
        type="search"
        placeholder="输入番号或关键词，回车搜索"
        @keydown.enter="search"
      />
      <AppButton type="button" variant="primary" :disabled="loading" @click="search">
        {{ loading ? "搜索中…" : "搜索" }}
      </AppButton>
    </div>

    <p v-if="error" class="magnet-page__error">{{ error }}</p>

    <div v-if="!loading && !error && !searched" class="magnet-page__empty">
      输入番号或关键词，搜索磁力资源
    </div>
    <div v-else-if="!loading && !error && searched && results.length === 0" class="magnet-page__empty">
      没有找到结果，换个关键词试试
    </div>

    <div v-if="loading" class="magnet-page__loading">
      <BusySpinner :size="22" />
      <span>正在搜索…</span>
    </div>

    <ul v-if="results.length" class="magnet-page__list">
      <li v-for="r in results" :key="r.id" class="magnet-page__row">
        <div class="magnet-page__main">
          <div class="magnet-page__name" :title="r.name">{{ r.name }}</div>
          <div class="magnet-page__meta">
            <span v-if="r.category" class="magnet-page__meta-item">{{ r.category }}</span>
            <span class="magnet-page__meta-item">{{ formatSize(r.size) }}</span>
            <span class="magnet-page__meta-item" :title="'做种 ' + r.seeders">↑{{ r.seeders }}</span>
            <span class="magnet-page__meta-item" :title="'下载 ' + r.leechers">↓{{ r.leechers }}</span>
            <span class="magnet-page__meta-item">{{ formatDate(r.date) }}</span>
          </div>
        </div>
        <div class="magnet-page__actions">
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
            class="magnet-page__link"
          >
            详情
          </a>
        </div>
      </li>
    </ul>
    <MagnetOfflineModal :open="offlineOpen" :magnet="offlineMagnet" :magnet-name="offlineName" @close="offlineOpen = false" />
  </div>
</template>

<style scoped>
.magnet-page {
  max-width: 900px;
  margin: 0 auto;
  min-height: 100vh;
  min-height: 100dvh;
  padding: 24px 20px calc(20px + env(safe-area-inset-bottom, 0px));
  box-sizing: border-box;
}

.magnet-page__head {
  margin-bottom: 16px;
}

.magnet-page__title {
  margin: 0 0 4px;
  font-size: 20px;
  font-weight: 700;
  color: var(--text);
}

.magnet-page__desc {
  margin: 0;
  font-size: 13px;
  color: var(--text-muted);
}

.magnet-page__bar {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
  margin-bottom: 16px;
}

.magnet-page__input {
  flex: 1 1 auto;
  min-width: 0;
  height: 40px;
  padding: 0 14px;
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  background: var(--surface);
  color: var(--text);
  font-size: 14px;
  outline: none;
}
.magnet-page__input:focus {
  border-color: var(--brand);
}

.magnet-page__error {
  margin: 0 0 12px;
  color: var(--danger);
  font-size: 13px;
}

.magnet-page__empty {
  display: grid;
  place-items: center;
  color: var(--text-muted);
  font-size: 13px;
  text-align: center;
  padding: 60px 0;
}

.magnet-page__loading {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 10px;
  color: var(--text-muted);
  font-size: 13px;
  padding: 60px 0;
}

.magnet-page__list {
  list-style: none;
  margin: 0;
  padding: 0;
  border: 1px solid var(--border-soft);
  border-radius: var(--radius-sm);
  overflow: hidden;
}

.magnet-page__row {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px 14px;
  border-bottom: 1px solid var(--border-soft);
}
.magnet-page__row:last-child {
  border-bottom: none;
}

.magnet-page__main {
  flex: 1 1 auto;
  min-width: 0;
}

.magnet-page__name {
  font-size: 14px;
  font-weight: 600;
  color: var(--text);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.magnet-page__meta {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 4px 10px;
  margin-top: 4px;
  font-size: 12px;
  color: var(--text-muted);
}

.magnet-page__actions {
  flex: 0 0 auto;
  display: flex;
  align-items: center;
  gap: 8px;
}

.magnet-page__link {
  color: var(--brand);
  font-size: 13px;
  text-decoration: none;
  white-space: nowrap;
}
</style>
