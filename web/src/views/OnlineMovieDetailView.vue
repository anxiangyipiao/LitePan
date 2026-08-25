<script setup lang="ts">
import { ref, onMounted, computed } from "vue";
import { useRoute, useRouter } from "vue-router";
import AppButton from "@/components/base/AppButton.vue";
import BusySpinner from "@/components/base/BusySpinner.vue";
import {
  getTmdbMovieDetails,
  getTmdbTvDetails,
  getTmdbCredits,
  getTmdbImages,
  tmdbImage,
  type TmdbMovieDetails,
  type TmdbTvDetails,
  type TmdbCredits,
} from "@/api/tmdb";
import { searchMagnet, type MagnetSearchResult } from "@/api/magnetSearch";
import { addToQB } from "@/api/qb";
import { getApiErrorMessage } from "@/api/client";
import { formatSize } from "@/utils/format";
import { toast, copyTextToClipboard } from "@/composables/useToast";
import MagnetOfflineModal from "@/components/common/MagnetOfflineModal.vue";

const route = useRoute();
const router = useRouter();

const movieId = computed(() => Number(route.params.id) || 0);
const mediaType = computed(() => (route.query.type as string) || "movie");
const movieTitle = computed(() => (route.query.title as string) || "");

const movie = ref<TmdbMovieDetails | TmdbTvDetails | null>(null);
const credits = ref<TmdbCredits | null>(null);
const images = ref<{ backdrops: { file_path: string }[] } | null>(null);
const loading = ref(true);
const error = ref("");

const magnetKeyword = ref("");
const magnetResults = ref<MagnetSearchResult[]>([]);
const magnetLoading = ref(false);
const magnetSearched = ref(false);
const qbPushing = ref<Record<number, boolean>>({});
const offlineOpen = ref(false);
const offlineMagnet = ref("");
const offlineName = ref("");
const showFullSummary = ref(false);

async function loadMovieDetail() {
  loading.value = true;
  error.value = "";
  try {
    if (mediaType.value === "tv") {
      movie.value = await getTvDetails(movieId.value);
    } else {
      movie.value = await getTmdbMovieDetails(movieId.value);
    }
    // 加载演员和图片
    try {
      credits.value = await getTmdbCredits(movieId.value, mediaType.value as "movie" | "tv");
    } catch {
      // 忽略演员加载失败
    }
    try {
      const imgRes = await getTmdbImages(movieId.value, mediaType.value as "movie" | "tv");
      images.value = { backdrops: imgRes.backdrops || [] };
    } catch {
      // 忽略图片加载失败
    }
    // 初始化磁力搜索关键词
    const title = (movie.value as TmdbMovieDetails)?.title || (movie.value as TmdbTvDetails)?.name || movieTitle.value;
    const originalTitle = (movie.value as TmdbMovieDetails)?.original_title || (movie.value as TmdbTvDetails)?.original_name || "";
    magnetKeyword.value = originalTitle || title;
  } catch (e) {
    error.value = getApiErrorMessage(e, "加载详情失败");
  } finally {
    loading.value = false;
  }
}

// 包装 getTmdbTvDetails 以统一错误处理
async function getTvDetails(id: number) {
  return await getTmdbTvDetails(id);
}

async function doMagnetSearch() {
  const q = magnetKeyword.value.trim();
  if (!q) {
    toast.warning("请输入搜索关键词");
    return;
  }
  magnetLoading.value = true;
  try {
    magnetResults.value = (await searchMagnet(q)) ?? [];
    magnetSearched.value = true;
  } catch (e) {
    toast.error(getApiErrorMessage(e, "磁力搜索失败"));
  } finally {
    magnetLoading.value = false;
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
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
}

function goBack() {
  router.back();
}

function posterSrc(): string {
  const m = movie.value;
  if (!m) return "";
  return tmdbImage.poster(m.poster_path, "w500");
}

function backdropSrc(): string {
  const m = movie.value;
  if (!m) return "";
  return tmdbImage.backdrop(m.backdrop_path, "w780");
}

const summaryText = computed(() => {
  return movie.value?.overview || "";
});

const title = computed(() => {
  const m = movie.value;
  if (!m) return "";
  return (m as TmdbMovieDetails)?.title || (m as TmdbTvDetails)?.name || "";
});

const originalTitle = computed(() => {
  const m = movie.value;
  if (!m) return "";
  return (m as TmdbMovieDetails)?.original_title || (m as TmdbTvDetails)?.original_name || "";
});

const releaseDate = computed(() => {
  const m = movie.value;
  if (!m) return "";
  return (m as TmdbMovieDetails)?.release_date || (m as TmdbTvDetails)?.first_air_date || "";
});

const runtime = computed(() => {
  const m = movie.value as TmdbMovieDetails;
  return m?.runtime;
});

const genres = computed(() => {
  return movie.value?.genres || [];
});

const castList = computed(() => {
  return credits.value?.cast?.slice(0, 8) || [];
});

const director = computed(() => {
  const crew = credits.value?.crew || [];
  return crew.find((c) => c.job === "Director")?.name || "";
});

onMounted(() => {
  loadMovieDetail();
});
</script>

<template>
  <div class="omd-page">
    <button class="omd-page__back" @click="goBack">
      <svg viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="2">
        <path d="m15 18-6-6 6-6" />
      </svg>
      返回
    </button>

    <div v-if="loading" class="omd-page__loading">
      <BusySpinner :size="32" />
      <span>加载中…</span>
    </div>

    <div v-else-if="error" class="omd-page__error">
      <p>{{ error }}</p>
      <AppButton variant="primary" @click="loadMovieDetail">重试</AppButton>
    </div>

    <template v-else-if="movie">
      <!-- 英雄区 -->
      <div class="omd-hero">
        <div class="omd-hero__backdrop" :style="{ backgroundImage: backdropSrc() ? `url(${backdropSrc()})` : 'none' }"></div>
        <div class="omd-hero__shade"></div>
        <div class="omd-hero__content">
          <div class="omd-poster">
            <img v-if="posterSrc()" :src="posterSrc()" :alt="title" />
            <div v-else class="omd-poster__placeholder">{{ title?.slice(0, 1) || "?" }}</div>
          </div>
          <div class="omd-info">
            <h1 class="omd-title">{{ title }}</h1>
            <p v-if="originalTitle && originalTitle !== title" class="omd-original">
              {{ originalTitle }}
            </p>
            <div class="omd-meta">
              <span v-if="movie.vote_average && movie.vote_average > 0" class="omd-rating">
                <svg viewBox="0 0 24 24" width="13" height="13" fill="#ffd700" style="display:inline-block;vertical-align:-1px;">
                  <polygon points="12,2 15,8.5 22,9.3 17,14.1 18.2,21 12,17.8 5.8,21 7,14.1 2,9.3 9,8.5" />
                </svg>
                {{ movie.vote_average.toFixed(1) }}
              </span>
              <span v-if="releaseDate">{{ releaseDate }}</span>
              <span v-if="runtime">{{ runtime }} 分钟</span>
            </div>
            <div v-if="genres.length" class="omd-genres">
              <span v-for="g in genres" :key="g.id" class="omd-genre">{{ g.name }}</span>
            </div>
            <div v-if="director" class="omd-people">
              <span class="omd-people-label">导演</span>
              {{ director }}
            </div>
            <div v-if="castList.length" class="omd-people">
              <span class="omd-people-label">主演</span>
              {{ castList.map((c) => c.name).join(" / ") }}
            </div>
          </div>
        </div>
      </div>

      <!-- 简介 -->
      <div v-if="summaryText" class="omd-section">
        <h2 class="omd-section__title">简介</h2>
        <p class="omd-summary" :class="{ 'omd-summary--clamp': !showFullSummary }">
          {{ summaryText }}
        </p>
        <button v-if="summaryText.length > 150" class="omd-summary__toggle" @click="showFullSummary = !showFullSummary">
          {{ showFullSummary ? "收起" : "展开" }}
        </button>
      </div>

      <!-- 磁力搜索 -->
      <div class="omd-section">
        <h2 class="omd-section__title">磁力搜索</h2>
        <div class="omd-magnet-bar">
          <input
            v-model="magnetKeyword"
            class="omd-magnet-input"
            type="search"
            placeholder="输入关键词搜索磁力..."
            @keydown.enter="doMagnetSearch"
          />
          <AppButton variant="primary" :disabled="magnetLoading" @click="doMagnetSearch">
            {{ magnetLoading ? "搜索中…" : "搜索" }}
          </AppButton>
        </div>

        <div v-if="magnetLoading" class="omd-magnet-loading">
          <BusySpinner :size="22" />
          <span>搜索中…</span>
        </div>

        <div v-else-if="magnetSearched && magnetResults.length === 0" class="omd-magnet-empty">
          没有找到结果，换个关键词试试
        </div>

        <ul v-if="magnetResults.length" class="omd-magnet-list">
          <li v-for="r in magnetResults" :key="r.id" class="omd-magnet-row">
            <div class="omd-magnet-main">
              <div class="omd-magnet-name" :title="r.name">{{ r.name }}</div>
              <div class="omd-magnet-meta">
                <span v-if="r.category" class="omd-magnet-meta-item">{{ r.category }}</span>
                <span class="omd-magnet-meta-item">{{ formatSize(r.size) }}</span>
                <span class="omd-magnet-meta-item" :title="'做种 ' + r.seeders">↑{{ r.seeders }}</span>
                <span class="omd-magnet-meta-item" :title="'下载 ' + r.leechers">↓{{ r.leechers }}</span>
                <span class="omd-magnet-meta-item">{{ formatDate(r.date) }}</span>
              </div>
            </div>
            <div class="omd-magnet-actions">
              <AppButton size="sm" variant="secondary" @click="copyMagnet(r)">复制</AppButton>
              <AppButton size="sm" variant="primary" :disabled="!!qbPushing[r.id || 0]" @click="pushToQB(r)">
                {{ qbPushing[r.id || 0] ? "推送中…" : "下载到 qB" }}
              </AppButton>
              <AppButton size="sm" variant="secondary" @click="openOffline(r)">离线到网盘</AppButton>
            </div>
          </li>
        </ul>
      </div>
    </template>

    <MagnetOfflineModal :open="offlineOpen" :magnet="offlineMagnet" :magnet-name="offlineName" @close="offlineOpen = false" />
  </div>
</template>

<style scoped>
.omd-page {
  min-height: 100vh;
  background: linear-gradient(180deg, #0a0a0f 0%, #14141c 100%);
  padding-bottom: 40px;
}

.omd-page__back {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  margin: 16px;
  padding: 8px 14px;
  border: 1px solid rgba(255, 255, 255, 0.12);
  border-radius: 8px;
  background: rgba(255, 255, 255, 0.05);
  color: #c0c0c0;
  font-size: 13px;
  cursor: pointer;
  transition: all 0.2s ease;
}

.omd-page__back:hover {
  border-color: rgba(255, 215, 0, 0.4);
  color: #ffd700;
  background: rgba(255, 215, 0, 0.08);
}

.omd-page__loading {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 16px;
  padding: 100px 20px;
  color: #888;
  font-size: 14px;
}

.omd-page__error {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 16px;
  padding: 80px 20px;
  color: #f87171;
  text-align: center;
}

/* 英雄区 */
.omd-hero {
  position: relative;
  min-height: 380px;
  overflow: hidden;
}

.omd-hero__backdrop {
  position: absolute;
  inset: 0;
  background-size: cover;
  background-position: center 25%;
  filter: blur(30px) brightness(0.4);
  transform: scale(1.1);
}

.omd-hero__shade {
  position: absolute;
  inset: 0;
  background: linear-gradient(to bottom, rgba(10, 10, 15, 0.5) 0%, #0a0a0f 100%);
}

.omd-hero__content {
  position: relative;
  z-index: 1;
  display: flex;
  gap: 28px;
  padding: 40px 32px;
  max-width: 1000px;
  margin: 0 auto;
}

.omd-poster {
  flex: 0 0 auto;
  width: 200px;
  aspect-ratio: 2 / 3;
  border-radius: 14px;
  overflow: hidden;
  box-shadow: 0 20px 50px rgba(0, 0, 0, 0.6), 0 0 30px rgba(255, 215, 0, 0.1);
  border: 3px solid rgba(255, 215, 0, 0.2);
}

.omd-poster img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.omd-poster__placeholder {
  width: 100%;
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 64px;
  font-weight: 700;
  color: #555;
  background: linear-gradient(160deg, #1a1a24, #252532);
}

.omd-info {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  justify-content: flex-end;
}

.omd-title {
  margin: 0;
  font-size: clamp(24px, 3vw, 36px);
  font-weight: 800;
  color: #f0f0f0;
  text-shadow: 0 2px 10px rgba(0, 0, 0, 0.5);
}

.omd-original {
  margin: 6px 0 0;
  font-size: 14px;
  color: #999;
}

.omd-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 8px 16px;
  margin: 12px 0;
  font-size: 14px;
  color: #aaa;
}

.omd-rating {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  color: #ffd700;
  font-weight: 700;
  font-size: 16px;
}

.omd-genres {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin: 8px 0;
}

.omd-genre {
  padding: 4px 12px;
  border: 1px solid rgba(255, 215, 0, 0.3);
  border-radius: 999px;
  background: rgba(255, 215, 0, 0.08);
  color: #ffd700;
  font-size: 12px;
  font-weight: 600;
}

.omd-people {
  margin: 4px 0;
  font-size: 13px;
  color: #bbb;
}

.omd-people-label {
  color: #777;
  margin-right: 8px;
}

/* 区块 */
.omd-section {
  max-width: 1000px;
  margin: 0 auto;
  padding: 24px 32px;
}

.omd-section__title {
  margin: 0 0 14px;
  font-size: 18px;
  font-weight: 700;
  color: #e0e0e0;
}

.omd-summary {
  margin: 0;
  font-size: 14px;
  line-height: 1.8;
  color: #bbb;
  white-space: pre-wrap;
}

.omd-summary--clamp {
  display: -webkit-box;
  -webkit-line-clamp: 3;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.omd-summary__toggle {
  margin-top: 8px;
  padding: 0;
  border: none;
  background: transparent;
  color: #ffd700;
  font-size: 13px;
  font-weight: 600;
  cursor: pointer;
}

.omd-summary__toggle:hover {
  color: #ffe44d;
}

/* 磁力搜索 */
.omd-magnet-bar {
  display: flex;
  gap: 10px;
  margin-bottom: 16px;
}

.omd-magnet-input {
  flex: 1;
  height: 40px;
  padding: 0 14px;
  border: 1px solid rgba(255, 255, 255, 0.12);
  border-radius: 10px;
  background: rgba(255, 255, 255, 0.05);
  color: #e0e0e0;
  font-size: 14px;
  outline: none;
  transition: border-color 0.2s ease;
}

.omd-magnet-input:focus {
  border-color: rgba(255, 215, 0, 0.5);
}

.omd-magnet-input::placeholder {
  color: rgba(255, 255, 255, 0.35);
}

.omd-magnet-loading {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 30px 0;
  color: #888;
  font-size: 13px;
}

.omd-magnet-empty {
  padding: 30px 0;
  text-align: center;
  color: #777;
  font-size: 13px;
}

.omd-magnet-list {
  list-style: none;
  margin: 0;
  padding: 0;
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 12px;
  overflow: hidden;
}

.omd-magnet-row {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px 16px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.06);
}

.omd-magnet-row:last-child {
  border-bottom: none;
}

.omd-magnet-main {
  flex: 1;
  min-width: 0;
}

.omd-magnet-name {
  font-size: 14px;
  font-weight: 600;
  color: #e0e0e0;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.omd-magnet-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 4px 12px;
  margin-top: 4px;
  font-size: 12px;
  color: #888;
}

.omd-magnet-meta-item {
  white-space: nowrap;
}

.omd-magnet-actions {
  flex: 0 0 auto;
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

@media (max-width: 640px) {
  .omd-hero__content {
    flex-direction: column;
    align-items: center;
    text-align: center;
    padding: 24px 16px;
  }

  .omd-poster {
    width: 150px;
  }

  .omd-meta,
  .omd-genres {
    justify-content: center;
  }

  .omd-section {
    padding: 20px 16px;
  }

  .omd-magnet-row {
    flex-direction: column;
    align-items: stretch;
    gap: 10px;
  }

  .omd-magnet-actions {
    display: grid;
    grid-template-columns: 1fr 1fr;
  }

  .omd-magnet-actions .btn {
    width: 100%;
  }
}
</style>
