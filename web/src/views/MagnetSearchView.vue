<script setup lang="ts">
import { ref } from "vue";
import AppButton from "@/components/base/AppButton.vue";
import BusySpinner from "@/components/base/BusySpinner.vue";
import { searchMagnet, type MagnetSearchResult } from "@/api/magnetSearch";
import { addToQB } from "@/api/qb";
import { getApiErrorMessage } from "@/api/client";
import { formatSize } from "@/utils/format";
import { toast, copyTextToClipboard } from "@/composables/useToast";
import MagnetOfflineModal from "@/components/common/MagnetOfflineModal.vue";

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
      <div class="magnet-page__hero">
        <!-- 左侧：标识 + 标题 + 描述 -->
        <div class="magnet-page__hero-info">
          <div class="magnet-page__icon">
            <i class="fa-solid fa-magnet" aria-hidden="true"></i>
          </div>
          <div class="magnet-page__hero-text">
            <h1 class="magnet-page__title">磁力搜索</h1>
            <p class="magnet-page__desc">输入番号或关键词，搜索磁力资源</p>
          </div>
        </div>

        <!-- 右侧：搜索框内嵌进横幅，消除右侧留白 -->
        <div class="magnet-page__hero-search">
          <div class="magnet-page__input-wrap">
            <i class="fa-solid fa-search magnet-page__input-icon" aria-hidden="true"></i>
            <input
              v-model="keyword"
              class="magnet-page__input"
              type="search"
              placeholder="输入番号或关键词，回车搜索"
              @keydown.enter="search"
            />
          </div>
          <AppButton type="button" variant="primary" :disabled="loading" @click="search">
            <i v-if="!loading" class="fa-solid fa-search" aria-hidden="true"></i>
            {{ loading ? "搜索中…" : "搜索" }}
          </AppButton>
        </div>
      </div>
    </header>

    <p v-if="error" class="magnet-page__error">
      <i class="fa-solid fa-circle-exclamation" aria-hidden="true"></i>
      {{ error }}
    </p>

    <div v-if="!loading && !error && !searched" class="magnet-page__empty">
      <i class="fa-solid fa-magnet" aria-hidden="true"></i>
      <p>输入番号或关键词，搜索磁力资源</p>
    </div>
    <div v-else-if="!loading && !error && searched && results.length === 0" class="magnet-page__empty">
      <i class="fa-solid fa-inbox" aria-hidden="true"></i>
      <p>没有找到结果，换个关键词试试</p>
    </div>

    <div v-if="loading" class="magnet-page__loading">
      <BusySpinner :size="22" />
      <span>正在搜索…</span>
    </div>

    <div v-if="results.length" class="magnet-page__stats">
      <span class="magnet-page__stats-item">
        <i class="fa-solid fa-list" aria-hidden="true"></i>
        共 {{ results.length }} 条结果
      </span>
    </div>

    <ul v-if="results.length" class="magnet-page__list">
      <li v-for="r in results" :key="r.id" class="magnet-page__row">
        <div class="magnet-page__main">
          <div class="magnet-page__name" :title="r.name">{{ r.name }}</div>
          <div class="magnet-page__meta">
            <span v-if="r.category" class="magnet-page__tag magnet-page__tag--cat">
              <i class="fa-solid fa-folder-open" aria-hidden="true"></i>{{ r.category }}
            </span>
            <span class="magnet-page__tag magnet-page__tag--size">
              <i class="fa-solid fa-database" aria-hidden="true"></i>{{ formatSize(r.size) }}
            </span>
            <span class="magnet-page__tag magnet-page__tag--seed">
              <i class="fa-solid fa-arrow-up" aria-hidden="true"></i>{{ r.seeders }}
            </span>
            <span class="magnet-page__tag magnet-page__tag--leech">
              <i class="fa-solid fa-arrow-down" aria-hidden="true"></i>{{ r.leechers }}
            </span>
            <span class="magnet-page__tag magnet-page__tag--date">
              <i class="fa-regular fa-clock" aria-hidden="true"></i>{{ formatDate(r.date) }}
            </span>
          </div>
        </div>
        <div class="magnet-page__actions">
          <AppButton type="button" variant="secondary" size="sm" @click="copyMagnet(r)">
            <i class="fa-solid fa-copy" aria-hidden="true"></i>复制
          </AppButton>
          <AppButton
            type="button"
            variant="primary"
            size="sm"
            :disabled="!!qbPushing[r.id]"
            @click="pushToQB(r)"
          >
            <i class="fa-solid fa-download" aria-hidden="true"></i>
            {{ qbPushing[r.id] ? "推送中…" : "下载到 qB" }}
          </AppButton>
          <AppButton type="button" variant="secondary" size="sm" @click="openOffline(r)">
            <i class="fa-solid fa-cloud-arrow-down" aria-hidden="true"></i>离线
          </AppButton>
          <a
            v-if="r.view_url"
            :href="r.view_url"
            target="_blank"
            rel="noopener"
            class="magnet-page__link"
          >
            <i class="fa-solid fa-external-link-alt" aria-hidden="true"></i>详情
          </a>
        </div>
      </li>
    </ul>
    <MagnetOfflineModal :open="offlineOpen" :magnet="offlineMagnet" :magnet-name="offlineName" @close="offlineOpen = false" />
  </div>
</template>

<style scoped>
.magnet-page {
  max-width: 960px;
  margin: 0 auto;
  min-height: 100vh;
  min-height: 100dvh;
  padding: 28px 20px calc(20px + env(safe-area-inset-bottom, 0px));
  box-sizing: border-box;
}

.magnet-page__head {
  margin-bottom: 20px;
}

/* 横幅：左侧标题信息 + 右侧内嵌搜索框，左右两端对齐，不留白 */
.magnet-page__hero {
  position: relative;
  overflow: hidden;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 24px;
  padding: 24px 28px;
  border-radius: var(--radius-md);
  background: var(--brand-gradient);
  box-shadow: var(--shadow-card);
}

/* 右上角装饰光晕，让横幅看起来饱满有层次 */
.magnet-page__hero::before {
  content: "";
  position: absolute;
  z-index: 0;
  right: -70px;
  top: -90px;
  width: 260px;
  height: 260px;
  border-radius: 50%;
  background: radial-gradient(circle, rgba(255, 255, 255, 0.22) 0%, rgba(255, 255, 255, 0) 68%);
  pointer-events: none;
}

.magnet-page__hero-info {
  position: relative;
  z-index: 1;
  display: flex;
  align-items: center;
  gap: 16px;
  min-width: 0;
  flex-shrink: 0;
}

.magnet-page__icon {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 48px;
  height: 48px;
  border-radius: var(--radius-sm);
  background: rgba(255, 255, 255, 0.2);
  color: #fff;
  font-size: 20px;
  flex-shrink: 0;
}

.magnet-page__hero-text {
  flex: 1;
  min-width: 0;
}

.magnet-page__title {
  margin: 0;
  font-size: 22px;
  font-weight: 700;
  color: #fff;
}

.magnet-page__desc {
  margin: 2px 0 0;
  font-size: 13px;
  color: rgba(255, 255, 255, 0.85);
}

/* 内嵌搜索区：占据横幅剩余宽度 */
.magnet-page__hero-search {
  position: relative;
  z-index: 1;
  display: flex;
  flex: 1 1 auto;
  align-items: center;
  gap: 10px;
  min-width: 280px;
  max-width: 560px;
  margin-left: auto;
}

.magnet-page__input-wrap {
  flex: 1 1 auto;
  min-width: 0;
  position: relative;
}

.magnet-page__input-icon {
  position: absolute;
  left: 14px;
  top: 50%;
  transform: translateY(-50%);
  color: var(--text-muted);
  font-size: 14px;
  pointer-events: none;
}

.magnet-page__input {
  width: 100%;
  height: 44px;
  padding: 0 14px 0 38px;
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  background: var(--surface);
  color: var(--text);
  font-size: 14px;
  outline: none;
  box-shadow: var(--shadow-soft);
  transition: border-color 0.2s ease, box-shadow 0.2s ease, background-color 0.2s ease;
}
.magnet-page__input:focus {
  border-color: var(--brand);
  box-shadow: 0 0 0 3px var(--accent-soft);
}

/* 横幅内的输入框/按钮：半透明玻璃质感 + 白色按钮，与渐变底色形成对比 */
.magnet-page__hero-search .magnet-page__input {
  border-color: rgba(255, 255, 255, 0.35);
  background: rgba(255, 255, 255, 0.16);
  color: #fff;
  box-shadow: none;
  height: 46px;
}
.magnet-page__hero-search .magnet-page__input::placeholder {
  color: rgba(255, 255, 255, 0.78);
}
.magnet-page__hero-search .magnet-page__input:focus {
  border-color: #fff;
  background: rgba(255, 255, 255, 0.24);
  box-shadow: 0 0 0 3px rgba(255, 255, 255, 0.18);
}
.magnet-page__hero-search .magnet-page__input-icon {
  color: rgba(255, 255, 255, 0.88);
}
.magnet-page__hero-search .btn--primary {
  background: #fff;
  color: var(--brand-strong);
  box-shadow: 0 2px 10px rgba(0, 0, 0, 0.12);
  height: 46px;
  padding-left: 18px;
  padding-right: 18px;
}
.magnet-page__hero-search .btn--primary:not(:disabled):hover {
  background: rgba(255, 255, 255, 0.92);
  box-shadow: 0 4px 14px rgba(0, 0, 0, 0.18);
}
.magnet-page__hero-search .btn--primary:disabled {
  background: #fff;
  color: var(--brand-strong);
  opacity: 0.6;
}

.magnet-page__error {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 0 0 12px;
  padding: 12px 16px;
  border-radius: var(--radius-sm);
  background: rgba(239, 68, 68, 0.08);
  border: 1px solid rgba(239, 68, 68, 0.2);
  color: var(--danger);
  font-size: 13px;
}

.magnet-page__empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 12px;
  color: var(--text-muted);
  font-size: 14px;
  text-align: center;
  padding: 72px 0;
}
.magnet-page__empty i {
  font-size: 40px;
  color: var(--border);
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

.magnet-page__stats {
  margin-bottom: 12px;
}
.magnet-page__stats-item {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  color: var(--text-muted);
}

.magnet-page__list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.magnet-page__row {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 14px 18px;
  border: 1px solid var(--border-soft);
  border-radius: var(--radius-sm);
  background: var(--surface);
  box-shadow: var(--shadow-soft);
  transition: box-shadow 0.2s ease, border-color 0.2s ease;
}
.magnet-page__row:hover {
  border-color: var(--border);
  box-shadow: var(--shadow-card);
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
  gap: 6px;
  margin-top: 6px;
}

.magnet-page__tag {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 2px 8px;
  border-radius: 999px;
  font-size: 11px;
  font-weight: 600;
  white-space: nowrap;
}
.magnet-page__tag i {
  font-size: 10px;
}
.magnet-page__tag--cat {
  background: var(--info-soft);
  color: var(--info);
}
.magnet-page__tag--size {
  background: var(--surface-sunken);
  color: var(--text-regular);
}
.magnet-page__tag--seed {
  background: rgba(16, 185, 129, 0.1);
  color: #059669;
}
.magnet-page__tag--leech {
  background: rgba(239, 68, 68, 0.08);
  color: #dc2626;
}
.magnet-page__tag--date {
  background: var(--surface-sunken);
  color: var(--text-muted);
}

.magnet-page__actions {
  flex: 0 0 auto;
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.magnet-page__link {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  color: var(--brand);
  font-size: 13px;
  font-weight: 500;
  text-decoration: none;
  white-space: nowrap;
}
.magnet-page__link:hover {
  text-decoration: underline;
}

@media (max-width: 640px) {
  .magnet-page {
    padding: 18px 12px calc(16px + env(safe-area-inset-bottom, 0px));
  }

  .magnet-page__hero {
    flex-direction: column;
    align-items: stretch;
    gap: 16px;
    padding: 18px;
  }

  .magnet-page__hero-search {
    max-width: none;
    min-width: 0;
    margin-left: 0;
  }

  .magnet-page__icon {
    width: 40px;
    height: 40px;
    font-size: 16px;
  }

  .magnet-page__title {
    font-size: 18px;
  }

  .magnet-page__row {
    flex-direction: column;
    align-items: stretch;
    gap: 12px;
    padding: 14px;
  }

  .magnet-page__actions {
    display: grid;
    grid-template-columns: 1fr 1fr;
    width: 100%;
  }

  .magnet-page__actions .btn {
    width: 100%;
    justify-content: center;
  }

  .magnet-page__link {
    grid-column: 1 / -1;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    min-height: 30px;
    padding: 4px 9px;
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    background: var(--surface);
    font-size: 13px;
  }

  .magnet-page__link:active {
    background: var(--surface-sunken);
  }
}
</style>