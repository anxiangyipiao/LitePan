<script setup lang="ts">
import { computed, onActivated, onMounted, ref, watch } from "vue";
import { useRoute, useRouter, onBeforeRouteLeave } from "vue-router";
import "@fortawesome/fontawesome-free/css/all.min.css";
import {
  mediaLibraryApi,
  type MediaLibraryItem,
  type MediaLibraryRoot,
  type MediaLibrarySort,
} from "@/api/mediaLibrary";
import {
  searchTmdb,
  getTmdbPopular,
  getTmdbTopRated,
  getTmdbNowPlaying,
  getTmdbUpcoming,
  getTmdbGenres,
  tmdbImage,
  type TmdbMedia,
  type TmdbSearchResult,
} from "@/api/tmdb";
import { getApiErrorMessage } from "@/api/client";
import { useVirtualPosterWall } from "@/composables/useVirtualPosterWall";
import { useAuthStore } from "@/stores/auth";
import SvgIcon from "@/components/icons/SvgIcon.vue";
import BusySpinner from "@/components/base/BusySpinner.vue";

const ROWS_PER_PAGE = 7;
const route = useRoute();
const router = useRouter();
const auth = useAuthStore();

// 模式切换：local=本地影视库, online=在线选片
const viewMode = ref<"local" | "online">("online");

// ---- 本地影视库 ----
const roots = ref<MediaLibraryRoot[]>([]);
const libId = ref("");
const sort = ref<MediaLibrarySort>("title_asc");
const keyword = ref("");
const searchDraft = ref("");
const genreFilter = ref("");
const actorFilter = ref("");
const facetGenres = ref<string[]>([]);
const facetActors = ref<string[]>([]);
const items = ref<MediaLibraryItem[]>([]);
const total = ref(0);
const page = ref(1);
const loading = ref(false);
const refreshing = ref(false);
const error = ref("");

const wall = useVirtualPosterWall(items);
const pageSize = computed(() => Math.max(1, wall.cols.value || 4) * ROWS_PER_PAGE);
const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)));

// ---- 在线选片 TMDB ----
type OnlineTabKey = "popular" | "top-rated" | "now-playing" | "upcoming" | "tv";
const onlineTabs: { key: OnlineTabKey; label: string }[] = [
  { key: "popular", label: "热门" },
  { key: "top-rated", label: "高分" },
  { key: "now-playing", label: "热映" },
  { key: "upcoming", label: "待映" },
  { key: "tv", label: "剧集" },
];
const onlineTab = ref<OnlineTabKey>("popular");
const onlineItems = ref<TmdbMedia[]>([]);
const onlineLoading = ref(false);
const onlineError = ref("");
const onlinePage = ref(1);
const onlineTotalPages = ref(1);
const onlineKeyword = ref("");
const onlineSearching = ref(false);

// 点海报 → 独立详情页
function goDetail(item: MediaLibraryItem) {
  void router.push({ path: `/movies/${item.id}`, query: { lib: item.lib_id } });
}

// 在线选片 → 详情页
function goOnlineDetail(movie: TmdbMedia) {
  const type = movie.media_type || (onlineTab.value === "tv" ? "tv" : "movie");
  void router.push({
    name: "online-movie-detail",
    params: { id: String(movie.id) },
    query: { type, title: movie.title || movie.name || "" },
  });
}

// KeepAlive 滚动记忆
let savedScrollY = 0;
onBeforeRouteLeave(() => {
  savedScrollY = window.scrollY || 0;
});
onActivated(() => {
  if (savedScrollY > 0) {
    requestAnimationFrame(() => window.scrollTo({ top: savedScrollY }));
  }
});

// ---- 配置弹窗 ----
const configOpen = ref(false);
const configSaving = ref(false);
const rootDraft = ref<MediaLibraryRoot[]>([]);
const batchPaths = ref("");

function openConfig() {
  rootDraft.value = roots.value.map((r) => ({ ...r }));
  batchPaths.value = "";
  configOpen.value = true;
}

function addRootRow() {
  rootDraft.value.push({ id: "", name: "", path: "" });
}

function addBatchPaths() {
  const paths = batchPaths.value
    .split(/\r?\n/)
    .map((p) => p.trim())
    .filter(Boolean);
  if (!paths.length) return;
  for (const p of paths) {
    rootDraft.value.push({ id: "", name: "", path: p });
  }
  batchPaths.value = "";
}

async function saveRoots() {
  configSaving.value = true;
  try {
    const saved = await mediaLibraryApi.saveRoots(
      rootDraft.value.map((r, i) => ({ id: r.id || `lib${i + 1}`, name: r.name, path: r.path })),
    );
    roots.value = saved;
    configOpen.value = false;
    await fetchItems(true);
  } catch (e) {
    error.value = getApiErrorMessage(e, "保存影视库失败");
  } finally {
    configSaving.value = false;
  }
}

// ---- 本地数据 ----
async function loadFacets() {
  try {
    const r = await mediaLibraryApi.facets(libId.value || undefined);
    const incomingGenres = r.genres ?? [];
    const incomingActors = r.actors ?? [];
    facetGenres.value = incomingGenres.includes(genreFilter.value) || !genreFilter.value
      ? incomingGenres
      : [...incomingGenres, genreFilter.value];
    facetActors.value = incomingActors.includes(actorFilter.value) || !actorFilter.value
      ? incomingActors
      : [...incomingActors, actorFilter.value];
  } catch {
    facetGenres.value = [];
    facetActors.value = [];
  }
}

async function loadRoots() {
  try {
    roots.value = await mediaLibraryApi.roots();
  } catch {
    roots.value = [];
  }
}

let seq = 0;
async function fetchItems(reset: boolean) {
  if (reset) page.value = 1;
  const targetPage = reset ? 1 : page.value;
  const s = ++seq;
  loading.value = true;
  if (reset) error.value = "";
  try {
    const res = await mediaLibraryApi.items({
      lib: libId.value || undefined,
      sort: sort.value,
      keyword: keyword.value.trim() || undefined,
      genre: genreFilter.value || undefined,
      actor: actorFilter.value || undefined,
      limit: pageSize.value,
      offset: (targetPage - 1) * pageSize.value,
    });
    if (s !== seq) return;
    items.value = res.items;
    total.value = res.total;
    const maxPage = Math.max(1, Math.ceil(total.value / pageSize.value));
    if (targetPage > maxPage) {
      page.value = maxPage;
      const fix = await mediaLibraryApi.items({
        lib: libId.value || undefined,
        sort: sort.value,
        keyword: keyword.value.trim() || undefined,
        genre: genreFilter.value || undefined,
        actor: actorFilter.value || undefined,
        limit: pageSize.value,
        offset: (maxPage - 1) * pageSize.value,
      });
      if (s !== seq) return;
      items.value = fix.items;
      total.value = fix.total;
    }
  } catch (e) {
    if (s !== seq) return;
    error.value = getApiErrorMessage(e, "影视库加载失败");
  } finally {
    if (s === seq) loading.value = false;
  }
}

async function refresh() {
  refreshing.value = true;
  try {
    await mediaLibraryApi.refresh({
      lib: libId.value || undefined,
      sort: sort.value,
      keyword: keyword.value.trim() || undefined,
      genre: genreFilter.value || undefined,
      actor: actorFilter.value || undefined,
    });
    await fetchItems(true);
  } catch (e) {
    error.value = getApiErrorMessage(e, "刷新影视库失败");
  } finally {
    refreshing.value = false;
  }
}

function goPage(p: number) {
  const n = Math.min(totalPages.value, Math.max(1, p));
  if (n === page.value) return;
  page.value = n;
  void fetchItems(false);
  requestAnimationFrame(() => window.scrollTo({ top: 0, behavior: "smooth" }));
}

watch([libId, sort, genreFilter, actorFilter], () => void fetchItems(true));
watch(libId, () => void loadFacets());

function submitSearch() {
  keyword.value = searchDraft.value.trim();
  void fetchItems(true);
}

function clearFacetFilters() {
  keyword.value = "";
  searchDraft.value = "";
  genreFilter.value = "";
  actorFilter.value = "";
  void router.replace({ path: "/movies", query: {} });
  void fetchItems(true);
}

function applyQueryFiltersFromRoute() {
  genreFilter.value = String(route.query.genre ?? "");
  actorFilter.value = String(route.query.actor ?? "");
}

// ---- 在线选片 TMDB ----
async function loadOnlineData(reset = false) {
  if (reset) {
    onlinePage.value = 1;
    onlineItems.value = [];
    onlineTotalPages.value = 1;
  }
  if (onlinePage.value > onlineTotalPages.value && onlineTotalPages.value > 0) return;
  if (onlineLoading.value) return;

  onlineLoading.value = true;
  onlineError.value = "";
  try {
    let result: TmdbSearchResult;
    if (onlineTab.value === "popular") {
      result = await getTmdbPopular("movie", onlinePage.value);
    } else if (onlineTab.value === "top-rated") {
      result = await getTmdbTopRated("movie", onlinePage.value);
    } else if (onlineTab.value === "now-playing") {
      result = await getTmdbNowPlaying(onlinePage.value);
    } else if (onlineTab.value === "upcoming") {
      result = await getTmdbUpcoming(onlinePage.value);
    } else {
      result = await getTmdbPopular("tv", onlinePage.value);
    }
    onlineTotalPages.value = result.total_pages || 1;
    onlineItems.value = [...onlineItems.value, ...(result.results || [])];
    onlinePage.value++;
  } catch (e) {
    onlineError.value = getApiErrorMessage(e, "加载失败，请检查网络和 TMDB API Key 配置");
  } finally {
    onlineLoading.value = false;
  }
}

async function searchOnline() {
  const q = onlineKeyword.value.trim();
  if (!q) {
    onlineSearching.value = false;
    await loadOnlineData(true);
    return;
  }
  onlineSearching.value = true;
  onlineLoading.value = true;
  onlineError.value = "";
  onlineItems.value = [];
  try {
    const result = await searchTmdb(q, 1);
    onlineItems.value = result.results || [];
    onlineTotalPages.value = result.total_pages || 1;
    onlinePage.value = 2;
  } catch (e) {
    onlineError.value = getApiErrorMessage(e, "搜索失败");
  } finally {
    onlineLoading.value = false;
  }
}

async function loadMoreOnline() {
  if (onlineSearching.value) {
    onlineLoading.value = true;
    onlineError.value = "";
    try {
      const q = onlineKeyword.value.trim();
      const nextPage = Math.floor(onlineItems.value.length / 20) + 1;
      const result = await searchTmdb(q, nextPage);
      const more = result.results || [];
      if (more.length < 20) onlineTotalPages.value = nextPage;
      onlineItems.value = [...onlineItems.value, ...more];
    } catch (e) {
      onlineError.value = getApiErrorMessage(e, "加载更多失败");
    } finally {
      onlineLoading.value = false;
    }
  } else {
    await loadOnlineData();
  }
}

async function switchOnlineTab(tab: OnlineTabKey) {
  onlineTab.value = tab;
  onlineSearching.value = false;
  onlineKeyword.value = "";
  await loadOnlineData(true);
}

function onlinePosterSrc(movie: TmdbMedia): string {
  return tmdbImage.poster(movie.poster_path, "w342");
}

function onlinePlaceholderText(movie: TmdbMedia): string {
  const title = movie.title || movie.name || "";
  return title ? title.slice(0, 1) : "?";
}

function onlineRatingClass(rating: number): string {
  if (rating >= 8) return "ml-online__rating--high";
  if (rating >= 6) return "ml-online__rating--mid";
  return "ml-online__rating--low";
}

function onlineYear(movie: TmdbMedia): string {
  const date = movie.release_date || movie.first_air_date || "";
  return date ? date.slice(0, 4) : "";
}

function handleOnlineScroll(e: Event) {
  const target = e.target as HTMLElement;
  if (!target) return;
  const { scrollTop, scrollHeight, clientHeight } = target;
  if (scrollHeight - scrollTop - clientHeight < 200 && !onlineLoading.value) {
    if (onlineSearching.value) {
      if (onlinePage.value <= onlineTotalPages.value) loadMoreOnline();
    } else {
      if (onlinePage.value <= onlineTotalPages.value) loadMoreOnline();
    }
  }
}

// 切换模式
function switchViewMode(mode: "local" | "online") {
  viewMode.value = mode;
  if (mode === "online" && onlineItems.value.length === 0) {
    loadOnlineData();
  }
}

onMounted(() => {
  applyQueryFiltersFromRoute();
  void loadRoots().then(() => {
    void loadFacets();
    void fetchItems(true);
  });
  // 默认打开在线选片，主动触发一次加载，避免空白等待
  if (onlineItems.value.length === 0) {
    void loadOnlineData(true);
  }
  void loadTmdbGenres();
});

// TMDB 分类表（id → 中文/英文名称），用于把在线选片卡片的 genre_ids 解析为真实名称
const tmdbGenres = ref<Map<number, string>>(new Map());
async function loadTmdbGenres() {
  try {
    const [movie, tv] = await Promise.all([
      getTmdbGenres("movie").catch(() => ({ genres: [] as { id: number; name: string }[] })),
      getTmdbGenres("tv").catch(() => ({ genres: [] as { id: number; name: string }[] })),
    ]);
    const map = new Map<number, string>();
    for (const g of movie.genres || []) map.set(g.id, g.name);
    for (const g of tv.genres || []) map.set(g.id, g.name);
    tmdbGenres.value = map;
  } catch {
    // 静默失败：名称解析不出来就退回到不显示分类
  }
}

// 从 genre_ids 解析出最多 max 个真实分类名（同名去重）
function onlineGenres(movie: TmdbMedia, max = 2): string[] {
  const ids = movie.genre_ids || [];
  const names: string[] = [];
  const seen = new Set<string>();
  for (const id of ids) {
    if (names.length >= max) break;
    const n = tmdbGenres.value.get(id);
    if (n && !seen.has(n)) {
      seen.add(n);
      names.push(n);
    }
  }
  return names;
}

watch(
  () => [String(route.query.genre ?? ""), String(route.query.actor ?? "")],
  ([g, a]) => {
    if (g !== genreFilter.value) genreFilter.value = g;
    if (a !== actorFilter.value) actorFilter.value = a;
  },
);

watch(pageSize, (n, o) => {
  if (n === o || !o) return;
  const firstIndex = (page.value - 1) * (o as number);
  page.value = Math.floor(firstIndex / (n as number)) + 1;
  void fetchItems(false);
});

watch(onlineTab, () => {
  loadOnlineData(true);
});

const typeLabel = (t: string) => (t === "tv" ? "剧集" : "电影");
const subtitle = (item: MediaLibraryItem) => {
  const parts: string[] = [typeLabel(item.media_type)];
  if (item.year) parts.push(String(item.year));
  if (item.media_type === "tv" && item.tv_state === "updating") parts.push("追更中");
  return parts.join(" · ");
};
</script>

<template>
  <div class="ml-page">
    <!-- 模式切换 -->
    <div class="ml-mode-switch">
      <button
        class="ml-mode-btn"
        :class="{ 'ml-mode-btn--active': viewMode === 'local' }"
        @click="switchViewMode('local')"
      >
        <SvgIcon name="folder" :size="16" />
        本地影视库
      </button>
      <button
        class="ml-mode-btn"
        :class="{ 'ml-mode-btn--active': viewMode === 'online' }"
        @click="switchViewMode('online')"
      >
        <SvgIcon name="compass" :size="16" />
        在线选片
      </button>
    </div>

    <!-- 本地影视库 -->
    <template v-if="viewMode === 'local'">
      <header class="ml-topbar">
        <div class="ml-topbar__primary">

          <select v-model="libId" class="ml-control ml-control--lib" aria-label="选择影视库">
            <option value="">全部库</option>
            <option v-for="r in roots" :key="r.id" :value="r.id">{{ r.name }}</option>
          </select>

          <select v-model="sort" class="ml-control" aria-label="排序">
            <option value="title_asc">名称</option>
            <option value="year_desc">年份 ↓</option>
            <option value="year_asc">年份 ↑</option>
          </select>

          <form class="ml-search" @submit.prevent="submitSearch">
            <input v-model="searchDraft" class="ml-search-input" placeholder="搜索片名…" />
          </form>

          <button
            type="button"
            class="ml-icon-btn"
            title="刷新索引"
            aria-label="刷新索引"
            :disabled="refreshing"
            @click="refresh"
          >
            <SvgIcon name="refresh" :size="15" />
          </button>
          <button
            v-if="auth.isAdmin"
            type="button"
            class="ml-icon-btn"
            title="配置影视库"
            aria-label="配置影视库"
            @click="openConfig"
          >
            <SvgIcon name="settings" :size="15" />
          </button>
        </div>

        <div v-if="facetGenres.length || facetActors.length" class="ml-topbar__facets">
          <select v-if="facetGenres.length" v-model="genreFilter" class="ml-control" aria-label="按分类筛选">
            <option value="">全部分类</option>
            <option v-for="g in facetGenres" :key="g" :value="g">{{ g }}</option>
          </select>

          <select v-if="facetActors.length" v-model="actorFilter" class="ml-control" aria-label="按演员筛选">
            <option value="">全部演员</option>
            <option v-for="a in facetActors" :key="a" :value="a">{{ a }}</option>
          </select>

          <span v-if="genreFilter || actorFilter" class="ml-facet-count">
            已选：{{ [genreFilter, actorFilter].filter(Boolean).join(" · ") }}
            <button type="button" class="ml-facet-clear" @click="clearFacetFilters">清除</button>
          </span>
        </div>
      </header>

      <p v-if="error" class="ml-error">{{ error }}</p>

      <div v-if="roots.length === 0 && !loading" class="ml-empty">
        <p class="ml-empty-title">尚未配置影视库</p>
        <p class="ml-empty-sub">
          影视模式读取服务器本地刮削输出目录（.strm / nfo / 海报所在）。
          <template v-if="auth.isAdmin">点右上角 ⚙ 配置根目录。</template>
          <template v-else>请联系管理员配置。</template>
        </p>
      </div>

      <div v-else-if="items.length === 0 && !loading" class="ml-empty">
        <p class="ml-empty-title">
          {{ keyword || genreFilter || actorFilter ? "没有匹配的影视" : "影视库为空" }}
        </p>
        <p v-if="!keyword && !genreFilter && !actorFilter" class="ml-empty-sub">该库还没有可展示的条目。</p>
        <p v-else class="ml-empty-sub">
          当前筛选（分类/演员/关键词）无匹配，试试
          <button type="button" class="ml-empty-link" @click="clearFacetFilters">清除筛选</button>
          或
          <button type="button" class="ml-empty-link" @click="refresh">刷新索引</button>。
        </p>
      </div>

      <div v-else>
        <div :ref="wall.rootEl" class="ml-wall-root">
          <div class="ml-wall-phantom" :style="{ height: `${wall.totalHeight.value}px` }">
            <div
              class="ml-wall"
              :style="{ ...wall.gridStyle.value, transform: `translateY(${wall.offsetY.value}px)` }"
            >
              <article
                v-for="item in wall.visibleItems.value"
                :key="item.lib_id + item.tmdb_id + item.folder_name"
                class="ml-card"
                role="button"
                tabindex="0"
                :title="`查看详情：${item.title}`"
                @click="goDetail(item)"
                @keydown.enter="goDetail(item)"
              >
                <div class="ml-card__poster">
                  <img
                    v-if="item.poster_url"
                    :src="item.poster_url"
                    :alt="item.title"
                    loading="lazy"
                    decoding="async"
                  />
                  <div v-else class="ml-card__placeholder">{{ item.title.slice(0, 1) }}</div>
                  <span v-if="item.media_type === 'tv' && item.tv_state === 'updating'" class="ml-card__badge">
                    追更
                  </span>
                  <span v-if="!item.play_url" class="ml-card__badge ml-card__badge--muted">无源</span>
                </div>
                <div class="ml-card__meta">
                  <div class="ml-card__title" :title="item.title">{{ item.title }}</div>
                  <div class="ml-card__sub">{{ subtitle(item) }}</div>
                </div>
              </article>
            </div>
          </div>
        </div>

        <nav v-if="totalPages > 1" class="ml-pager" aria-label="分页">
          <button type="button" class="ml-pager-btn" :disabled="page <= 1" @click="goPage(page - 1)">上一页</button>
          <template v-for="p in totalPages" :key="p">
            <button
              v-if="p === 1 || p === totalPages || Math.abs(p - page) <= 2"
              type="button"
              class="ml-pager-btn"
              :class="{ 'ml-pager-btn--active': p === page }"
              @click="goPage(p)"
            >{{ p }}</button>
            <span v-else-if="p === page - 3 || p === page + 3" class="ml-pager-ellipsis">…</span>
          </template>
          <button type="button" class="ml-pager-btn" :disabled="page >= totalPages" @click="goPage(page + 1)">下一页</button>
          <span class="ml-pager-info">{{ total }} 部 · 第 {{ page }}/{{ totalPages }} 页 · 每页 {{ pageSize }}（7 行）</span>
        </nav>
        <p v-else class="ml-pager-info ml-pager-info--single">{{ total }} 部 · 每页 {{ pageSize }}（7 行）</p>
      </div>
    </template>

    <!-- 在线选片 TMDB -->
    <template v-else>
      <div class="ml-online" @scroll="handleOnlineScroll">
        <header class="ml-topbar">
          <div class="ml-topbar__primary">
            <div class="ml-online__tabs">
              <button
                v-for="tab in onlineTabs"
                :key="tab.key"
                class="ml-online__tab"
                :class="{ 'ml-online__tab--active': onlineTab === tab.key && !onlineSearching }"
                @click="switchOnlineTab(tab.key)"
              >
                {{ tab.label }}
              </button>
            </div>
            <form class="ml-search" @submit.prevent="searchOnline">
              <input v-model="onlineKeyword" class="ml-search-input" placeholder="搜索影视..." />
            </form>
          </div>
        </header>

        <p v-if="onlineError" class="ml-error">
          {{ onlineError }}
          <button class="ml-online__retry" @click="onlineSearching ? searchOnline() : loadOnlineData(true)">重试</button>
        </p>

        <div v-if="!onlineLoading && !onlineError && onlineItems.length === 0" class="ml-empty">
          <div class="ml-empty__icon">🎬</div>
          <p class="ml-empty-title">{{ onlineSearching ? "没有找到结果" : "暂无内容" }}</p>
          <p v-if="onlineSearching" class="ml-empty-sub">换个关键词试试</p>
        </div>

        <div v-if="onlineLoading && onlineItems.length === 0" class="ml-online__loading">
          <BusySpinner :size="28" />
          <span>加载中…</span>
        </div>

        <div v-if="onlineItems.length" class="ml-online__grid">
          <div
            v-for="movie in onlineItems"
            :key="`${movie.media_type || onlineTab}-${movie.id}`"
            class="ml-online__card"
            @click="goOnlineDetail(movie)"
          >
            <div class="ml-online__poster">
              <img
                :src="onlinePosterSrc(movie)"
                :alt="movie.title || movie.name"
                loading="lazy"
                @error="($event.target as HTMLImageElement).style.display = 'none'"
              />
              <div class="ml-online__placeholder">{{ onlinePlaceholderText(movie) }}</div>
              <div
                v-if="movie.vote_average && movie.vote_average > 0"
                class="ml-online__rating"
                :class="onlineRatingClass(movie.vote_average)"
              >
                {{ movie.vote_average.toFixed(1) }}
              </div>
            </div>
            <div class="ml-online__meta">
              <div class="ml-online__title">{{ movie.title || movie.name }}</div>
              <div class="ml-online__sub">
                <span v-if="onlineYear(movie)">{{ onlineYear(movie) }}</span>
                <span v-if="onlineGenres(movie).length"> · {{ onlineGenres(movie).join(" · ") }}</span>
              </div>
            </div>
          </div>
        </div>

        <div v-if="onlineLoading && onlineItems.length" class="ml-online__loading ml-online__loading--more">
          <BusySpinner :size="22" />
          <span>加载更多…</span>
        </div>

        <div v-if="!onlineLoading && onlinePage > onlineTotalPages && onlineItems.length" class="ml-online__end">— 没有更多了 —</div>
      </div>
    </template>

    <!-- 配置弹窗 -->
    <div v-if="configOpen" class="ml-config-mask" @click.self="configOpen = false">
      <div class="ml-config">
        <h3 class="ml-config-title">影视库配置</h3>
        <p class="ml-config-desc">填写服务器本地刮削输出目录（绝对路径）。多个路径会自动聚合展示在「全部库」。</p>

        <div class="ml-config-batch">
          <textarea
            v-model="batchPaths"
            class="ml-config-textarea"
            rows="3"
            placeholder="批量添加路径：每行一个绝对路径，如&#10;/data/strm/movies&#10;/data/strm/tv&#10;D:\media\movies"
          ></textarea>
          <button type="button" class="ml-config-add" @click="addBatchPaths">批量添加</button>
        </div>

        <div v-for="(r, i) in rootDraft" :key="i" class="ml-config-row">
          <input v-model="r.name" class="ml-config-input ml-config-name" placeholder="库名（如：电影库）" />
          <input v-model="r.path" class="ml-config-input" placeholder="/data/strm/movies 或 D:\\media\\movies" />
          <button type="button" class="ml-config-del" title="删除" @click="rootDraft.splice(i, 1)">×</button>
        </div>
        <button type="button" class="ml-config-add" @click="addRootRow">+ 添加根目录</button>
        <div class="ml-config-actions">
          <button type="button" class="ml-config-save" :disabled="configSaving" @click="saveRoots">
            {{ configSaving ? "保存中…" : "保存" }}
          </button>
          <button type="button" class="ml-config-cancel" @click="configOpen = false">取消</button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.ml-page {
  min-height: 100vh;
  padding: 16px;
  max-width: 1280px;
  margin: 0 auto;
  box-sizing: border-box;
  background: linear-gradient(180deg, var(--bg) 0%, var(--bg-muted) 100%);
  --ml-gold: #b45309;
  --ml-gold-soft: rgba(217, 119, 6, 0.1);
  --ml-gold-border: rgba(217, 119, 6, 0.35);
  --ml-gold-grad: linear-gradient(135deg, #f59e0b, #d97706);
}

.ml-topbar {
  display: flex;
  flex-direction: column;
  gap: 10px;
  margin-bottom: 16px;
}

.ml-topbar__primary {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
  background: var(--surface);
  backdrop-filter: blur(12px);
  -webkit-backdrop-filter: blur(12px);
  border-radius: 12px;
  padding: 12px 16px;
  border: 1px solid var(--border-soft);
  box-shadow: var(--shadow-card);
}

.ml-topbar__facets {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.ml-facet-count {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 6px 12px;
  border-radius: 999px;
  background: rgba(255, 215, 0, 0.12);
  color: #ffd700;
  font-size: 12px;
  border: 1px solid rgba(255, 215, 0, 0.25);
}

.ml-facet-clear {
  border: none;
  background: transparent;
  color: #ffd700;
  font-size: 12px;
  font-weight: 600;
  cursor: pointer;
}

.ml-title {
  margin: 0;
  font-size: 20px;
  font-weight: 700;
  color: #f0f0f0;
}

.ml-control {
  height: 34px;
  padding: 0 10px;
  border: 1px solid var(--border);
  border-radius: 8px;
  background: var(--surface-sunken);
  color: var(--text);
  font-size: 13px;
  min-width: 110px;
  transition: border-color 0.2s ease, background 0.2s ease;
}

.ml-control:hover {
  border-color: var(--ml-gold-border);
  background: var(--surface);
}

.ml-control:focus {
  outline: none;
  border-color: var(--ml-gold-border);
  background: var(--surface);
}

.ml-control option {
  background: var(--surface);
  color: var(--text);
}

.ml-control--lib {
  min-width: 140px;
}

.ml-search {
  flex: 1 1 200px;
  min-width: 160px;
}

.ml-search-input {
  width: 100%;
  height: 34px;
  padding: 0 14px;
  border: 1px solid var(--border);
  border-radius: 8px;
  background: var(--surface-sunken);
  color: var(--text);
  font-size: 13px;
  transition: border-color 0.2s ease, background 0.2s ease;
}

.ml-search-input::placeholder {
  color: var(--text-muted);
}

.ml-search-input:focus {
  outline: none;
  border-color: var(--ml-gold-border);
  background: var(--surface);
}

.ml-icon-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 34px;
  height: 34px;
  border: 1px solid var(--border);
  border-radius: 8px;
  background: var(--surface-sunken);
  color: var(--text-muted);
  cursor: pointer;
  transition: all 0.2s ease;
}

.ml-icon-btn:hover:not(:disabled) {
  border-color: var(--ml-gold-border);
  background: var(--ml-gold-soft);
  color: var(--ml-gold);
}

.ml-icon-btn:disabled {
  opacity: 0.4;
  cursor: default;
}

.ml-error {
  margin: 0 0 12px;
  padding: 12px 16px;
  border-radius: 10px;
  background: rgba(220, 38, 38, 0.15);
  border: 1px solid rgba(220, 38, 38, 0.3);
  color: #fca5a5;
  font-size: 13px;
}

.ml-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
  padding: 100px 20px;
  color: var(--text-muted);
  text-align: center;
}

.ml-empty-title {
  margin: 0;
  font-size: 18px;
  font-weight: 600;
  color: var(--text);
}

.ml-empty-sub {
  margin: 0;
  font-size: 14px;
  color: var(--text-muted);
  max-width: 400px;
  line-height: 1.6;
}

/* 海报墙 */
.ml-wall-root {
  position: relative;
}

.ml-wall-phantom {
  position: relative;
  width: 100%;
}

.ml-wall {
  display: grid;
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  will-change: transform;
  gap: 8px;
}

.ml-card {
  min-width: 0;
  border-radius: 8px;
  cursor: pointer;
  outline: none;
  transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);
  border: 2px solid transparent;
  overflow: hidden;
  background: var(--surface);
  box-shadow: var(--shadow-card);
}

.ml-card:hover,
.ml-card:focus-visible {
  transform: scale(1.05) translateY(-8px);
  border-color: var(--ml-gold-border);
  box-shadow: 0 20px 40px rgba(15, 23, 42, 0.14), 0 0 20px var(--ml-gold-soft);
  z-index: 10;
}

.ml-card--disabled {
  cursor: default;
  opacity: 0.6;
}

.ml-card__poster {
  position: relative;
  aspect-ratio: 2 / 3;
  border-radius: 6px;
  overflow: hidden;
  background: var(--surface-sunken);
}

.ml-card__poster img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  display: block;
  transition: transform 0.4s ease;
}

.ml-card:hover .ml-card__poster img {
  transform: scale(1.08);
}

.ml-card__placeholder {
  width: 100%;
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 40px;
  font-weight: 700;
  color: var(--text-muted);
  background: linear-gradient(160deg, var(--surface-sunken), var(--surface-muted));
}

.ml-card__badge {
  position: absolute;
  top: 8px;
  right: 8px;
  padding: 3px 10px;
  border-radius: 999px;
  font-size: 11px;
  font-weight: 700;
  color: #fff;
  background: var(--ml-gold-grad);
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

.ml-card__badge--muted {
  background: rgba(100, 116, 139, 0.85);
  color: #fff;
}

.ml-card__meta {
  padding: 10px 6px 8px;
}

.ml-card__title {
  color: var(--text);
  font-size: 13px;
  font-weight: 600;
  line-height: 1.35;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.ml-card__sub {
  color: var(--text-muted);
  font-size: 11px;
  line-height: 1.4;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  margin-top: 2px;
}

.ml-pager {
  display: flex;
  align-items: center;
  justify-content: center;
  flex-wrap: wrap;
  gap: 8px;
  padding: 24px 0 calc(4px + env(safe-area-inset-bottom, 0px));
}

.ml-pager-btn {
  min-width: 44px;
  min-height: 44px;
  height: 44px;
  padding: 0 12px;
  border: 1px solid var(--border);
  border-radius: 10px;
  background: var(--surface);
  color: var(--text-regular);
  font-size: 13px;
  font-weight: 500;
  cursor: pointer;
  touch-action: manipulation;
  transition: all 0.2s ease;
}

.ml-pager-btn:hover:not(:disabled) {
  border-color: var(--ml-gold-border);
  background: var(--ml-gold-soft);
  color: var(--ml-gold);
}

.ml-pager-btn:disabled {
  opacity: 0.3;
  cursor: default;
}

.ml-pager-btn--active {
  background: var(--ml-gold-grad);
  border-color: var(--ml-gold);
  color: #fff;
  font-weight: 700;
}

.ml-pager-ellipsis {
  padding: 0 4px;
  color: var(--text-muted);
}

.ml-pager-info {
  margin-left: 12px;
  color: var(--text-muted);
  font-size: 12px;
}

.ml-pager-info--single {
  display: block;
  text-align: center;
  padding: 20px 0 2px;
  color: var(--text-muted);
}

/* 播放器 */
.ml-player-mask {
  position: fixed;
  inset: 0;
  z-index: 200;
  background: rgba(0, 0, 0, 0.72);
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px;
}

.ml-player {
  width: min(960px, 100%);
  max-height: 100%;
  background: #0f172a;
  border-radius: 14px;
  overflow: hidden;
  display: flex;
  flex-direction: column;
}

.ml-player-head {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 14px;
  color: #e2e8f0;
}

.ml-player-title {
  font-size: 15px;
  font-weight: 600;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ml-player-meta {
  font-size: 12px;
  color: #94a3b8;
  border: 1px solid #334155;
  border-radius: 999px;
  padding: 1px 8px;
  flex-shrink: 0;
}

.ml-player-spacer {
  flex: 1;
}

.ml-player-close {
  appearance: none;
  border: none;
  background: transparent;
  color: #94a3b8;
  cursor: pointer;
  padding: 4px;
}

.ml-player-close:hover {
  color: #fff;
}

.ml-player-video {
  width: 100%;
  aspect-ratio: 16 / 9;
  background: #000;
}

.ml-player-error {
  margin: 0;
  padding: 8px 14px;
  color: #fca5a5;
  font-size: 13px;
}

.ml-player-foot {
  padding: 12px 14px;
}

.ml-player-ext {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

.ml-ext-btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 6px 12px;
  border-radius: 8px;
  border: 1px solid #334155;
  background: #1e293b;
  color: #e2e8f0;
  font-size: 13px;
  cursor: pointer;
}

.ml-ext-btn:disabled {
  opacity: 0.5;
  cursor: default;
}

.ml-skybox-guide {
  margin: 12px 0 0;
  font-size: 12px;
  line-height: 1.6;
  color: #94a3b8;
}

.ml-skybox-guide code {
  color: #e2e8f0;
  background: #1e293b;
  padding: 1px 6px;
  border-radius: 4px;
}

/* 配置弹窗 */
.ml-config-mask {
  position: fixed;
  inset: 0;
  z-index: 210;
  background: rgba(0, 0, 0, 0.75);
  backdrop-filter: blur(4px);
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 20px;
}

.ml-config {
  width: min(560px, 100%);
  max-height: 90vh;
  overflow: auto;
  background: var(--surface);
  border-radius: 16px;
  padding: 24px;
  display: flex;
  flex-direction: column;
  gap: 14px;
  border: 1px solid var(--border-soft);
  box-shadow: var(--shadow-pop);
}

.ml-config-title {
  margin: 0;
  font-size: 18px;
  color: var(--text);
  font-weight: 700;
}

.ml-config-desc {
  margin: 0;
  font-size: 13px;
  color: var(--text-muted);
  line-height: 1.6;
}

.ml-config-batch {
  display: flex;
  gap: 10px;
  align-items: flex-end;
}

.ml-config-textarea {
  flex: 1;
  min-width: 0;
  padding: 10px 12px;
  border: 1px solid var(--border);
  border-radius: 10px;
  font-size: 13px;
  line-height: 1.6;
  resize: vertical;
  background: var(--surface-sunken);
  color: var(--text);
  transition: border-color 0.2s ease;
}

.ml-config-textarea:focus {
  outline: none;
  border-color: var(--ml-gold-border);
}

.ml-config-row {
  display: flex;
  gap: 10px;
}

.ml-config-input {
  flex: 1;
  min-width: 0;
  height: 38px;
  padding: 0 12px;
  border: 1px solid var(--border);
  border-radius: 10px;
  font-size: 13px;
  background: var(--surface-sunken);
  color: var(--text);
  transition: border-color 0.2s ease;
}

.ml-config-input:focus {
  outline: none;
  border-color: var(--ml-gold-border);
}

.ml-config-name {
  flex: 0 0 120px;
}

.ml-config-del {
  width: 38px;
  height: 38px;
  border: 1px solid rgba(220, 38, 38, 0.3);
  border-radius: 10px;
  background: rgba(220, 38, 38, 0.1);
  color: #f87171;
  cursor: pointer;
  font-size: 18px;
  transition: all 0.2s ease;
}

.ml-config-del:hover {
  background: rgba(220, 38, 38, 0.2);
  border-color: rgba(220, 38, 38, 0.5);
}

.ml-config-add {
  align-self: flex-start;
  padding: 8px 16px;
  border: 1px dashed var(--ml-gold-border);
  border-radius: 10px;
  background: transparent;
  color: var(--ml-gold);
  font-size: 13px;
  font-weight: 600;
  cursor: pointer;
  transition: all 0.2s ease;
}

.ml-config-add:hover {
  background: var(--ml-gold-soft);
  border-color: var(--ml-gold-border);
}

.ml-config-actions {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
  margin-top: 8px;
}

.ml-config-save,
.ml-config-cancel {
  padding: 10px 20px;
  border-radius: 10px;
  font-size: 14px;
  font-weight: 600;
  cursor: pointer;
  transition: all 0.2s ease;
}

.ml-config-save {
  border: none;
  background: var(--ml-gold-grad);
  color: #fff;
}

.ml-config-save:hover:not(:disabled) {
  transform: translateY(-1px);
  box-shadow: 0 8px 20px var(--ml-gold-soft);
}

.ml-config-save:disabled {
  opacity: 0.5;
  cursor: default;
}

.ml-config-cancel {
  border: 1px solid var(--border);
  background: var(--surface-sunken);
  color: var(--text-regular);
}

.ml-config-cancel:hover {
  background: var(--surface-hover);
  border-color: var(--border);
}

/* ---- 详情页 ---- */
.ml-detail-mask {
  position: fixed;
  inset: 0;
  z-index: 205;
  background: rgba(0, 0, 0, 0.6);
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 20px;
}

.ml-detail {
  position: relative;
  width: min(760px, 100%);
  max-height: 92vh;
  overflow: auto;
  background: var(--surface, #fff);
  border-radius: 16px;
  box-shadow: 0 20px 60px rgba(0, 0, 0, 0.4);
}

.ml-detail-close {
  position: absolute;
  top: 12px;
  right: 12px;
  z-index: 2;
  width: 34px;
  height: 34px;
  border: none;
  border-radius: 999px;
  background: rgba(0, 0, 0, 0.45);
  color: #fff;
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  justify-content: center;
}

.ml-detail-hero {
  position: relative;
  height: 240px;
  background-size: cover;
  background-position: center 30%;
}

.ml-detail-hero__shade {
  position: absolute;
  inset: 0;
  background: linear-gradient(to bottom, transparent 20%, var(--surface, #fff) 100%);
}

.ml-detail-body {
  padding: 0 24px 24px;
  margin-top: -8px;
}

.ml-detail-main {
  display: flex;
  gap: 20px;
  align-items: flex-start;
}

.ml-detail-poster {
  flex: 0 0 auto;
  width: 168px;
  aspect-ratio: 2 / 3;
  border-radius: 12px;
  object-fit: cover;
  box-shadow: 0 12px 30px rgba(0, 0, 0, 0.3);
  background: var(--surface-sunken, #f1f5f9);
  margin-top: -64px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 48px;
  font-weight: 700;
  color: var(--text-muted, #94a3b8);
}

.ml-detail-info {
  flex: 1;
  min-width: 0;
}

.ml-detail-title {
  margin: 4px 0 6px;
  font-size: 22px;
  font-weight: 700;
  color: var(--text, #111827);
}

.ml-detail-meta {
  margin: 0 0 10px;
  font-size: 13px;
  color: var(--text-muted, #64748b);
}

.ml-detail-overview {
  margin: 0 0 16px;
  font-size: 14px;
  line-height: 1.7;
  color: var(--text-regular, #334155);
  display: -webkit-box;
  -webkit-line-clamp: 6;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.ml-detail-actions {
  display: flex;
  gap: 10px;
  align-items: center;
}

.ml-detail-play {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 10px 26px;
  border: none;
  border-radius: 999px;
  background: var(--brand, #4f8ef7);
  color: #fff;
  font-size: 15px;
  font-weight: 600;
  cursor: pointer;
}

.ml-detail-nosource {
  font-size: 13px;
  color: var(--text-muted, #94a3b8);
}

.ml-detail-episodes {
  margin-top: 22px;
}

.ml-detail-sec {
  margin: 0 0 12px;
  font-size: 14px;
  font-weight: 600;
  color: var(--text-regular, #334155);
}

.ml-detail-ep-grid {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.ml-detail-ep {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 7px 12px;
  border: 1px solid var(--border-soft, #e2e8f0);
  border-radius: 8px;
  background: var(--surface-sunken, #f8fafc);
  color: var(--text-regular, #334155);
  font-size: 13px;
  cursor: pointer;
}

.ml-detail-ep:disabled {
  opacity: 0.45;
  cursor: default;
}

.ml-detail-ep-num {
  font-weight: 600;
}

.ml-empty-link {
  padding: 0;
  border: none;
  background: transparent;
  color: var(--brand, #4f8ef7);
  font-size: 13px;
  cursor: pointer;
  text-decoration: underline;
  text-underline-offset: 2px;
}

.ml-topbar__primary .ml-search {
  flex: 1 1 200px;
  min-width: 160px;
}

@media (max-width: 640px) {
  .ml-page {
    padding: 12px;
  }

  .ml-type-btn {
    padding: 6px 9px;
  }

  .ml-detail-body {
    padding: 0 16px 16px;
  }

  .ml-detail-main {
    flex-direction: column;
  }

  .ml-detail-poster {
    width: 120px;
  }

  .ml-detail-hero {
    height: 160px;
  }
}

/* 模式切换 */
.ml-mode-switch {
  display: flex;
  gap: 4px;
  background: var(--surface);
  backdrop-filter: blur(12px);
  -webkit-backdrop-filter: blur(12px);
  border-radius: 12px;
  padding: 4px;
  margin-bottom: 16px;
  border: 1px solid var(--border-soft);
  box-shadow: var(--shadow-card);
  width: fit-content;
}

.ml-mode-btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 8px 16px;
  border: none;
  border-radius: 8px;
  background: transparent;
  color: var(--text-muted);
  font-size: 13px;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.2s ease;
}

.ml-mode-btn:hover {
  color: var(--ml-gold);
  background: var(--ml-gold-soft);
}

.ml-mode-btn--active {
  background: var(--ml-gold-soft);
  color: var(--ml-gold);
  font-weight: 600;
}

/* 在线选片 */
.ml-online {
  min-height: 100vh;
  overflow-y: auto;
  max-height: calc(100dvh - 120px);
}

.ml-online__tabs {
  display: flex;
  gap: 4px;
  background: var(--surface-sunken);
  border-radius: 8px;
  padding: 3px;
}

.ml-online__tab {
  padding: 6px 14px;
  border: none;
  border-radius: 6px;
  background: transparent;
  color: var(--text-muted);
  font-size: 13px;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.2s ease;
}

.ml-online__tab:hover {
  color: var(--ml-gold);
  background: var(--ml-gold-soft);
}

.ml-online__tab--active {
  background: var(--ml-gold-soft);
  color: var(--ml-gold);
  font-weight: 600;
}

.ml-online__retry {
  border: none;
  background: var(--ml-gold-soft);
  color: var(--ml-gold);
  padding: 4px 12px;
  border-radius: 6px;
  font-size: 12px;
  cursor: pointer;
}

.ml-online__loading {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 12px;
  color: var(--text-muted);
  font-size: 13px;
  padding: 60px 0;
}

.ml-online__loading--more {
  padding: 20px 0;
}

.ml-online__end {
  text-align: center;
  color: var(--text-muted);
  font-size: 13px;
  padding: 30px 0;
}

.ml-online__grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(140px, 1fr));
  gap: 16px 12px;
}

.ml-online__card {
  cursor: pointer;
  border-radius: 8px;
  overflow: hidden;
  background: var(--surface);
  border: 2px solid transparent;
  box-shadow: var(--shadow-card);
  transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);
}

.ml-online__card:hover {
  transform: scale(1.05) translateY(-6px);
  border-color: var(--ml-gold-border);
  box-shadow: 0 20px 40px rgba(15, 23, 42, 0.14), 0 0 20px var(--ml-gold-soft);
  z-index: 10;
}

.ml-online__poster {
  position: relative;
  aspect-ratio: 2 / 3;
  background: var(--surface-sunken);
}

.ml-online__poster img {
  position: relative;
  z-index: 2;
  width: 100%;
  height: 100%;
  object-fit: contain;
  display: block;
  transition: transform 0.4s ease;
}

.ml-online__card:hover .ml-online__poster img {
  transform: scale(1.08);
}

.ml-online__placeholder {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 40px;
  font-weight: 700;
  color: var(--text-muted);
  background: linear-gradient(160deg, var(--surface-sunken), var(--surface-muted));
  z-index: 1;
}

.ml-online__rating {
  position: absolute;
  top: 6px;
  right: 6px;
  z-index: 3;
  padding: 2px 8px;
  border-radius: 999px;
  font-size: 11px;
  font-weight: 700;
  color: #fff;
  background: var(--ml-gold-grad);
}

.ml-online__rating--low {
  background: linear-gradient(135deg, #ef4444, #dc2626);
  color: #fff;
}

.ml-online__rating--mid {
  background: linear-gradient(135deg, #f59e0b, #d97706);
  color: #000;
}

.ml-online__meta {
  padding: 8px 6px 10px;
}

.ml-online__title {
  font-size: 13px;
  font-weight: 600;
  color: var(--text);
  line-height: 1.35;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.ml-online__sub {
  font-size: 11px;
  color: var(--text-muted);
  margin-top: 2px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

@media (max-width: 640px) {
  .ml-online__grid {
    grid-template-columns: repeat(auto-fill, minmax(110px, 1fr));
    gap: 12px 8px;
  }

  .ml-mode-btn {
    padding: 6px 9px;
  }
}
</style>
