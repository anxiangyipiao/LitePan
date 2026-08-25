<script setup lang="ts">
import { ref, onMounted, watch } from "vue";
import { useRouter } from "vue-router";
import BusySpinner from "@/components/base/BusySpinner.vue";
import {
  searchDouban,
  getDoubanHot,
  getDoubanTop250,
  getDoubanRecommend,
  type DoubanHotItem,
} from "@/api/douban";
import { getApiErrorMessage } from "@/api/client";

const router = useRouter();

type TabKey = "hot" | "top250" | "movie" | "tv";

const tabs: { key: TabKey; label: string }[] = [
  { key: "hot", label: "热门" },
  { key: "top250", label: "Top 250" },
  { key: "movie", label: "电影" },
  { key: "tv", label: "剧集" },
];

const activeTab = ref<TabKey>("hot");
const movies = ref<DoubanHotItem[]>([]);
const loading = ref(false);
const error = ref("");
const page = ref(0);
const hasMore = ref(true);
const keyword = ref("");
const searching = ref(false);

const pageSize = 24;

async function loadData(reset = false) {
  if (reset) {
    page.value = 0;
    movies.value = [];
    hasMore.value = true;
  }
  if (!hasMore.value || loading.value) return;

  loading.value = true;
  error.value = "";
  try {
    const start = page.value * pageSize;
    let result: DoubanHotItem[] = [];

    if (activeTab.value === "hot") {
      const res = await getDoubanHot("movie", start, pageSize);
      result = res.movies;
    } else if (activeTab.value === "top250") {
      result = (await getDoubanTop250(start, pageSize)) ?? [];
    } else {
      const type = activeTab.value === "movie" ? "movie" : "tv";
      result = (await getDoubanRecommend(type, start, pageSize)) ?? [];
    }

    if (result.length < pageSize) {
      hasMore.value = false;
    }
    movies.value = [...movies.value, ...result];
    page.value++;
  } catch (e) {
    error.value = getApiErrorMessage(e, "加载失败，请检查网络");
  } finally {
    loading.value = false;
  }
}

async function doSearch() {
  const q = keyword.value.trim();
  if (!q) {
    // 如果清空搜索框，恢复原来的列表
    searching.value = false;
    await loadData(true);
    return;
  }
  searching.value = true;
  loading.value = true;
  error.value = "";
  movies.value = [];
  try {
    const res = await searchDouban(q, 0, pageSize);
    movies.value = res.movies.map((m) => ({
      id: m.id,
      title: m.title,
      rating: m.rating,
      poster: m.poster || m.thumb,
      year: m.year,
      genres: m.genres,
      directors: m.directors,
      actors: m.casts,
    }));
    hasMore.value = movies.value.length >= pageSize;
    page.value = 1;
  } catch (e) {
    error.value = getApiErrorMessage(e, "搜索失败");
  } finally {
    loading.value = false;
  }
}

async function loadMore() {
  if (searching.value) {
    // 搜索模式下加载更多
    loading.value = true;
    error.value = "";
    try {
      const res = await searchDouban(keyword.value.trim(), page.value * pageSize, pageSize);
      const moreMovies = res.movies.map((m) => ({
        id: m.id,
        title: m.title,
        rating: m.rating,
        poster: m.poster || m.thumb,
        year: m.year,
        genres: m.genres,
        directors: m.directors,
        actors: m.casts,
      }));
      if (moreMovies.length < pageSize) {
        hasMore.value = false;
      }
      movies.value = [...movies.value, ...moreMovies];
      page.value++;
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

function goDetail(movie: DoubanHotItem) {
  router.push({ name: "online-movie-detail", params: { id: movie.id }, query: { title: movie.title } });
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
  if (scrollHeight - scrollTop - clientHeight < 200 && !loading.value && hasMore.value) {
    loadMore();
  }
}

onMounted(() => {
  loadData();
});

watch(activeTab, () => {
  loadData(true);
});

function posterSrc(movie: DoubanHotItem): string {
  return movie.poster || `https://img1.doubanio.com/view/photo/s_ratio_poster/public/${movie.id}.jpg`;
}

function placeholderText(title: string): string {
  return title ? title.slice(0, 1) : "?";
}
</script>

<template>
  <div class="om-page" @scroll="handleScroll">
    <header class="om-page__head">
      <h1 class="om-page__title">在线选片</h1>
      <div class="om-page__desc">豆瓣热门影视 · 磁力搜索</div>
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
          <svg v-if="!searching" viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2">
            <circle cx="11" cy="11" r="8" />
            <path d="m21 21-4.3-4.3" />
          </svg>
          <svg v-else viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M12 2v4m0 12v4M4.93 4.93l2.83 2.83m8.48 8.48 2.83 2.83M2 12h4m12 0h4M4.93 19.07l2.83-2.83m8.48-8.48 2.83-2.83" />
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
        :key="movie.id"
        class="om-card"
        @click="goDetail(movie)"
      >
        <div class="om-card__poster">
          <img
            :src="posterSrc(movie)"
            :alt="movie.title"
            loading="lazy"
            @error="($event.target as HTMLImageElement).style.display = 'none'"
          />
          <div class="om-card__placeholder">{{ placeholderText(movie.title) }}</div>
          <div v-if="movie.rating && Number(movie.rating) > 0" class="om-card__rating">
            {{ movie.rating }}
          </div>
        </div>
        <div class="om-card__meta">
          <div class="om-card__title">{{ movie.title }}</div>
          <div class="om-card__sub">
            <span v-if="movie.year">{{ movie.year }}</span>
            <span v-if="movie.genres?.length"> · {{ movie.genres.slice(0, 2).join(" / ") }}</span>
          </div>
          <div v-if="movie.actors?.length" class="om-card__actors">
            {{ movie.actors.slice(0, 3).join(" / ") }}
          </div>
        </div>
      </div>
    </div>

    <div v-if="loading && movies.length" class="om-page__loading om-page__loading--more">
      <BusySpinner :size="22" />
      <span>加载更多…</span>
    </div>

    <div v-if="!hasMore && movies.length" class="om-page__end">— 没有更多了 —</div>
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
  object-fit: cover;
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

.om-card__actors {
  font-size: 11px;
  color: #777;
  margin-top: 3px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  white-space: normal;
  line-height: 1.4;
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
