<script setup lang="ts">
import { ref, onMounted, watch } from "vue";
import { useRouter } from "vue-router";
import BusySpinner from "@/components/base/BusySpinner.vue";
import {
  searchTmdb,
  getTmdbPopular,
  getTmdbTopRated,
  getTmdbNowPlaying,
  getTmdbUpcoming,
  tmdbImage,
  type TmdbMedia,
  type TmdbSearchResult,
} from "@/api/tmdb";
import { getApiErrorMessage } from "@/api/client";

const router = useRouter();

type TabKey = "popular" | "top-rated" | "now-playing" | "upcoming" | "movie" | "tv";

const tabs: { key: TabKey; label: string }[] = [
  { key: "popular", label: "热门" },
  { key: "top-rated", label: "高分" },
  { key: "now-playing", label: "热映" },
  { key: "upcoming", label: "待映" },
  { key: "tv", label: "剧集" },
];

const activeTab = ref<TabKey>("popular");
const movies = ref<TmdbMedia[]>([]);
const loading = ref(false);
const error = ref("");
const page = ref(1);
const totalPages = ref(1);
const keyword = ref("");
const searching = ref(false);

async function loadData(reset = false) {
  if (reset) {
    page.value = 1;
    movies.value = [];
    totalPages.value = 1;
  }
  if (page.value > totalPages.value && totalPages.value > 0) return;
  if (loading.value) return;

  loading.value = true;
  error.value = "";
  try {
    let result: TmdbSearchResult;

    if (activeTab.value === "popular") {
      result = await getTmdbPopular("movie", page.value);
    } else if (activeTab.value === "top-rated") {
      result = await getTmdbTopRated("movie", page.value);
    } else if (activeTab.value === "now-playing") {
      result = await getTmdbNowPlaying(page.value);
    } else if (activeTab.value === "upcoming") {
      result = await getTmdbUpcoming(page.value);
    } else {
      result = await getTmdbPopular("tv", page.value);
    }

    totalPages.value = result.total_pages || 1;
    movies.value = [...movies.value, ...(result.results || [])];
    page.value++;
  } catch (e) {
    error.value = getApiErrorMessage(e, "加载失败，请检查网络和 TMDB API Key 配置");
  } finally {
    loading.value = false;
  }
}

async function doSearch() {
  const q = keyword.value.trim();
  if (!q) {
    searching.value = false;
    await loadData(true);
    return;
  }
  searching.value = true;
  loading.value = true;
  error.value = "";
  movies.value = [];
  try {
    const result = await searchTmdb(q, 1);
    movies.value = result.results || [];
    totalPages.value = result.total_pages || 1;
    page.value = 2;
  } catch (e) {
    error.value = getApiErrorMessage(e, "搜索失败");
  } finally {
    loading.value = false;
  }
}

async function loadMore() {
  if (searching.value) {
    loading.value = true;
    error.value = "";
    try {
      const q = keyword.value.trim();
      const nextPage = Math.floor(movies.value.length / 20) + 1;
      const result = await searchTmdb(q, nextPage);
      const more = result.results || [];
      if (more.length < 20) {
        totalPages.value = nextPage;
      }
      movies.value = [...movies.value, ...more];
    } catch (e) {
      error.value = getApiErrorMessage(e, "加载更多失败");
    } finally {
      loading.value = false;
    }
  } else {
    await loadData();
  }
}

async function switchTab(tab: TabKey) {
  activeTab.value = tab;
  searching.value = false;
  keyword.value = "";
  await loadData(true);
}

function goDetail(movie: TmdbMedia) {
  const type = movie.media_type || (activeTab.value === "tv" ? "tv" : "movie");
  router.push({
    name: "online-movie-detail",
    params: { id: String(movie.id) },
    query: { type, title: movie.title || movie.name || "" },
  });
}

function retry() {
  if (searching.value) {
    doSearch();
  } else {
    loadData(true);
  }
}

function handleScroll(e: Event) {
  const target = e.target as HTMLElement;
  if (!target) return;
  const { scrollTop, scrollHeight, clientHeight } = target;
  if (scrollHeight - scrollTop - clientHeight < 200 && !loading.value) {
    if (searching.value) {
      if (page.value <= totalPages.value) loadMore();
    } else {
      if (page.value <= totalPages.value) loadMore();
    }
  }
}

onMounted(() => {
  loadData();
});

watch(activeTab, () => {
  loadData(true);
});

function posterSrc(movie: TmdbMedia): string {
  return tmdbImage.poster(movie.poster_path, "w342");
}

function placeholderText(movie: TmdbMedia): string {
  const title = movie.title || movie.name || "";
  return title ? title.slice(0, 1) : "?";
}

function ratingClass(rating: number): string {
  if (rating >= 8) return "om-card__rating--high";
  if (rating >= 6) return "om-card__rating--mid";
  return "om-card__rating--low";
}

function year(movie: TmdbMedia): string {
  const date = movie.release_date || movie.first_air_date || "";
  return date ? date.slice(0, 4) : "";
}
</script>

<template>
  <div class="om-page" @scroll="handleScroll">
    <header class="om-page__head">
      <h1 class="om-page__title">在线选片</h1>
      <p class="om-page__desc">TMDB · 热门影视</p>
    </header>

    <div class="om-page__bar">
      <div class="om-tabs">
        <button
          v-for="tab in tabs"
          :key="tab.key"
          class="om-tab"
          :class="{ 'om-tab--active': activeTab === tab.key && !searching }"
          @click="switchTab(tab.key)"
        >
          {{ tab.label }}
        </button>
      </div>
      <div class="om-search">
        <input
          v-model="keyword"
          class="om-search__input"
          type="search"
          placeholder="搜索影视..."
          @keydown.enter="doSearch"
        />
        <button class="om-search__btn" @click="doSearch">
          <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2">
            <circle cx="11" cy="11" r="8" />
            <path d="m21 21-4.3-4.3" />
          </svg>
        </button>
      </div>
    </div>

    <p v-if="error" class="om-page__error">
      {{ error }}
      <button class="om-page__retry" @click="retry">重试</button>
    </p>

    <div v-if="!loading && !error && movies.length === 0" class="om-page__empty">
      <div class="om-page__empty-icon">🎬</div>
      <p class="om-page__empty-title">{{ searching ? "没有找到结果" : "暂无内容" }}</p>
      <p v-if="searching" class="om-page__empty-sub">换个关键词试试</p>
    </div>

    <div v-if="loading && movies.length === 0" class="om-page__loading">
      <BusySpinner :size="28" />
      <span>加载中…</span>
    </div>

    <div v-if="movies.length" class="om-grid">
      <div
        v-for="movie in movies"
        :key="`${movie.media_type || activeTab}-${movie.id}`"
        class="om-card"
        @click="goDetail(movie)"
      >
        <div class="om-card__poster">
          <img
            :src="posterSrc(movie)"
            :alt="movie.title || movie.name"
            loading="lazy"
            @error="($event.target as HTMLImageElement).style.display = 'none'"
          />
          <div class="om-card__placeholder">{{ placeholderText(movie) }}</div>
          <div
            v-if="movie.vote_average && movie.vote_average > 0"
            class="om-card__rating"
            :class="ratingClass(movie.vote_average)"
          >
            {{ movie.vote_average.toFixed(1) }}
          </div>
        </div>
        <div class="om-card__meta">
          <div class="om-card__title">{{ movie.title || movie.name }}</div>
          <div class="om-card__sub">
            <span v-if="year(movie)">{{ year(movie) }}</span>
            <span v-if="movie.genre_ids?.length"> · {{ movie.genre_ids.length }} 类型</span>
          </div>
        </div>
      </div>
    </div>

    <div v-if="loading && movies.length" class="om-page__loading om-page__loading--more">
      <BusySpinner :size="22" />
      <span>加载更多…</span>
    </div>

    <div v-if="!loading && page > totalPages && movies.length" class="om-page__end">— 没有更多了 —</div>
  </div>
</template>

<style scoped>
.om-page {
  min-height: 100vh;
  padding: 16px;
  max-width: 1280px;
  margin: 0 auto;
  box-sizing: border-box;
  background: linear-gradient(180deg, #0a0a0f 0%, #14141c 100%);
  overflow-y: auto;
  height: calc(100dvh - 60px);
}

.om-page__head {
  margin-bottom: 16px;
}

.om-page__title {
  margin: 0;
  font-size: 22px;
  font-weight: 700;
  color: #f0f0f0;
}

.om-page__desc {
  margin: 4px 0 0;
  font-size: 13px;
  color: #888;
}

.om-page__bar {
  display: flex;
  gap: 12px;
  flex-wrap: wrap;
  margin-bottom: 20px;
  background: rgba(15, 15, 25, 0.85);
  backdrop-filter: blur(12px);
  -webkit-backdrop-filter: blur(12px);
  border-radius: 12px;
  padding: 12px 16px;
  border: 1px solid rgba(255, 255, 255, 0.08);
}

.om-tabs {
  display: flex;
  gap: 4px;
  background: rgba(255, 255, 255, 0.05);
  border-radius: 8px;
  padding: 3px;
}

.om-tab {
  padding: 6px 14px;
  border: none;
  border-radius: 6px;
  background: transparent;
  color: #aaa;
  font-size: 13px;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.2s ease;
}

.om-tab:hover {
  color: #ffd700;
  background: rgba(255, 215, 0, 0.1);
}

.om-tab--active {
  background: rgba(255, 215, 0, 0.15);
  color: #ffd700;
  font-weight: 600;
}

.om-search {
  display: flex;
  align-items: center;
  gap: 4px;
  background: rgba(255, 255, 255, 0.05);
  border-radius: 8px;
  flex: 1;
  min-width: 180px;
}

.om-search__input {
  flex: 1;
  height: 36px;
  padding: 0 12px;
  border: none;
  background: transparent;
  color: #e0e0e0;
  font-size: 13px;
  outline: none;
}

.om-search__input::placeholder {
  color: rgba(255, 255, 255, 0.35);
}

.om-search__btn {
  width: 36px;
  height: 36px;
  border: none;
  background: transparent;
  color: #888;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: color 0.2s ease;
}

.om-search__btn:hover {
  color: #ffd700;
}

.om-page__error {
  margin: 0 0 16px;
  padding: 12px 16px;
  border-radius: 10px;
  background: rgba(220, 38, 38, 0.15);
  border: 1px solid rgba(220, 38, 38, 0.3);
  color: #fca5a5;
  font-size: 13px;
  display: flex;
  align-items: center;
  gap: 10px;
}

.om-page__retry {
  border: none;
  background: rgba(255, 215, 0, 0.2);
  color: #ffd700;
  padding: 4px 12px;
  border-radius: 6px;
  font-size: 12px;
  cursor: pointer;
}

.om-page__empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  padding: 80px 20px;
  color: #888;
  text-align: center;
}

.om-page__empty-icon {
  font-size: 48px;
}

.om-page__empty-title {
  margin: 0;
  font-size: 16px;
  font-weight: 600;
  color: #c0c0c0;
}

.om-page__empty-sub {
  margin: 0;
  font-size: 13px;
  color: #777;
}

.om-page__loading {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 12px;
  color: #888;
  font-size: 13px;
  padding: 60px 0;
}

.om-page__loading--more {
  padding: 20px 0;
}

.om-page__end {
  text-align: center;
  color: #666;
  font-size: 13px;
  padding: 30px 0;
}

/* 海报网格 */
.om-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(140px, 1fr));
  gap: 16px 12px;
}

.om-card {
  cursor: pointer;
  border-radius: 8px;
  overflow: hidden;
  background: rgba(255, 255, 255, 0.03);
  border: 2px solid transparent;
  transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);
}

.om-card:hover {
  transform: scale(1.05) translateY(-6px);
  border-color: rgba(255, 215, 0, 0.6);
  box-shadow: 0 20px 40px rgba(0, 0, 0, 0.5), 0 0 20px rgba(255, 215, 0, 0.15);
  z-index: 10;
}

.om-card__poster {
  position: relative;
  aspect-ratio: 2 / 3;
  background: #1a1a24;
}

.om-card__poster img {
  width: 100%;
  height: 100%;
  object-fit: contain;
  display: block;
  transition: transform 0.4s ease;
}

.om-card:hover .om-card__poster img {
  transform: scale(1.08);
}

.om-card__placeholder {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 40px;
  font-weight: 700;
  color: #555;
  background: linear-gradient(160deg, #1a1a24, #252532);
  z-index: 1;
}

.om-card__rating {
  position: absolute;
  top: 6px;
  right: 6px;
  padding: 2px 8px;
  border-radius: 999px;
  font-size: 11px;
  font-weight: 700;
  color: #000;
  background: linear-gradient(135deg, #ffd700, #ffaa00);
}

.om-card__rating--low {
  background: linear-gradient(135deg, #ef4444, #dc2626);
  color: #fff;
}

.om-card__rating--mid {
  background: linear-gradient(135deg, #f59e0b, #d97706);
  color: #000;
}

.om-card__meta {
  padding: 8px 6px 10px;
}

.om-card__title {
  font-size: 13px;
  font-weight: 600;
  color: #e8e8e8;
  line-height: 1.35;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.om-card__sub {
  font-size: 11px;
  color: #888;
  margin-top: 2px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

@media (max-width: 640px) {
  .om-grid {
    grid-template-columns: repeat(auto-fill, minmax(110px, 1fr));
    gap: 12px 8px;
  }

  .om-page__bar {
    flex-direction: column;
  }

  .om-tabs {
    width: 100%;
    justify-content: space-around;
  }

  .om-search {
    width: 100%;
  }
}
</style>
