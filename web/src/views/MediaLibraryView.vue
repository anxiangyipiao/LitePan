<script setup lang="ts">
import { nextTick, onActivated, onMounted, onUnmounted, ref, watch } from "vue";
import { useRouter, onBeforeRouteLeave } from "vue-router";
import "@fortawesome/fontawesome-free/css/all.min.css";
import {
  mediaLibraryApi,
  type MediaLibraryItem,
  type MediaLibraryRoot,
  type MediaLibrarySort,
} from "@/api/mediaLibrary";
import { getApiErrorMessage } from "@/api/client";
import { useVirtualPosterWall } from "@/composables/useVirtualPosterWall";
import { useAuthStore } from "@/stores/auth";
import SvgIcon from "@/components/icons/SvgIcon.vue";
import BusySpinner from "@/components/base/BusySpinner.vue";

const PAGE = 120;
const router = useRouter();
const auth = useAuthStore();

const roots = ref<MediaLibraryRoot[]>([]);
const libId = ref("");
const typeFilter = ref<"" | "movie" | "tv">("");
const sort = ref<MediaLibrarySort>("title_asc");
const keyword = ref("");
const searchDraft = ref("");
const genreFilter = ref("");
const actorFilter = ref("");
const facetGenres = ref<string[]>([]);
const facetActors = ref<string[]>([]);
const items = ref<MediaLibraryItem[]>([]);
const total = ref(0);
const loading = ref(false);
const refreshing = ref(false);
const error = ref("");

const wall = useVirtualPosterWall(items);

// 点海报 → 独立详情页
function goDetail(item: MediaLibraryItem) {
  void router.push({ path: `/movies/${item.id}`, query: { lib: item.lib_id } });
}

// KeepAlive 滚动记忆：离开前记录窗口滚动位置（onBeforeRouteLeave 时机 DOM 未变，最可靠），返回时恢复
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

// 批量添加：每行一个绝对路径，自动生成根目录行
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

// ---- 数据 ----
async function loadFacets() {
  try {
    const r = await mediaLibraryApi.facets(libId.value || undefined);
    const incomingGenres = r.genres ?? [];
    const incomingActors = r.actors ?? [];
    // 保留已选值，即使当前筛选项让它暂时不出现在 facets 里
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
  const s = ++seq;
  if (reset) {
    loading.value = true;
    error.value = "";
  }
  try {
    const res = await mediaLibraryApi.items({
      lib: libId.value || undefined,
      type: typeFilter.value || undefined,
      sort: sort.value,
      keyword: keyword.value.trim() || undefined,
      genre: genreFilter.value || undefined,
      actor: actorFilter.value || undefined,
      limit: PAGE,
      offset: reset ? 0 : items.value.length,
    });
    if (s !== seq) return;
    items.value = reset ? res.items : [...items.value, ...res.items];
    total.value = res.total;
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
      type: typeFilter.value || undefined,
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

function hasMore() {
  return items.value.length < total.value;
}

// ---- 加载更多哨兵 ----
const loadMoreEl = ref<HTMLElement | null>(null);
let loadMoreObserver: IntersectionObserver | null = null;

function bindLoadMore(el: unknown) {
  loadMoreEl.value = el instanceof Element ? (el as HTMLElement) : null;
  void nextTick(updateLoadMoreObserver);
}

async function updateLoadMoreObserver() {
  loadMoreObserver?.disconnect();
  loadMoreObserver = null;
  if (!hasMore() || !loadMoreEl.value) return;
  loadMoreObserver = new window.IntersectionObserver(
    (entries) => {
      if (!entries.some((entry) => entry.isIntersecting)) return;
      void fetchItems(false);
    },
    { root: null, rootMargin: "400px 0px", threshold: 0 },
  );
  loadMoreObserver.observe(loadMoreEl.value);
}

// ---- 过滤联动 ----
watch([libId, typeFilter, sort, genreFilter, actorFilter], () => void fetchItems(true));
watch(libId, () => void loadFacets());

function submitSearch() {
  keyword.value = searchDraft.value.trim();
  void fetchItems(true);
}

onMounted(() => {
  void loadRoots().then(() => {
    void loadFacets();
    void fetchItems(true);
  });
});

onUnmounted(() => {
  loadMoreObserver?.disconnect();
  loadMoreObserver = null;
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
    <header class="ml-topbar">
      <h1 class="ml-title">影视</h1>

      <select v-model="libId" class="ml-control" aria-label="选择影视库">
        <option value="">全部库</option>
        <option v-for="r in roots" :key="r.id" :value="r.id">{{ r.name }}</option>
      </select>

      <div class="ml-type-toggle" role="group" aria-label="类型筛选">
        <button
          type="button"
          class="ml-type-btn"
          :class="{ 'is-active': typeFilter === '' }"
          @click="typeFilter = ''"
        >
          全部
        </button>
        <button
          type="button"
          class="ml-type-btn"
          :class="{ 'is-active': typeFilter === 'movie' }"
          @click="typeFilter = 'movie'"
        >
          电影
        </button>
        <button
          type="button"
          class="ml-type-btn"
          :class="{ 'is-active': typeFilter === 'tv' }"
          @click="typeFilter = 'tv'"
        >
          剧集
        </button>
      </div>

      <select v-model="sort" class="ml-control" aria-label="排序">
        <option value="title_asc">名称</option>
        <option value="year_desc">年份 ↓</option>
        <option value="year_asc">年份 ↑</option>
      </select>

      <select v-model="genreFilter" class="ml-control" aria-label="按分类筛选">
        <option value="">全部分类</option>
        <option v-for="g in facetGenres" :key="g" :value="g">{{ g }}</option>
      </select>

      <select v-model="actorFilter" class="ml-control" aria-label="按演员筛选">
        <option value="">全部演员</option>
        <option v-for="a in facetActors" :key="a" :value="a">{{ a }}</option>
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
    </header>

    <p v-if="error" class="ml-error">{{ error }}</p>

    <div
      v-if="roots.length === 0 && !loading"
      class="ml-empty"
    >
      <p class="ml-empty-title">尚未配置影视库</p>
      <p class="ml-empty-sub">
        影视模式读取服务器本地刮削输出目录（.strm / nfo / 海报所在）。
        <template v-if="auth.isAdmin">点右上角 ⚙ 配置根目录。</template>
        <template v-else>请联系管理员配置。</template>
      </p>
    </div>

    <div v-else-if="items.length === 0 && !loading" class="ml-empty">
      <p class="ml-empty-title">{{ keyword ? "没有匹配的影视" : "影视库为空" }}</p>
      <p v-if="!keyword" class="ml-empty-sub">该库还没有可展示的条目。</p>
    </div>

    <div v-else :ref="wall.rootEl" class="ml-wall-root">
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
      <div
        v-if="hasMore()"
        :ref="bindLoadMore"
        class="ml-wall-foot"
        aria-hidden="true"
      >
        <BusySpinner :size="18" />
        <span>加载更多…</span>
      </div>
    </div>

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
}

.ml-topbar {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
  margin-bottom: 16px;
}

.ml-title {
  margin: 0;
  font-size: 20px;
  font-weight: 700;
  color: var(--text, #111827);
}

.ml-control {
  height: 34px;
  padding: 0 10px;
  border: 1px solid var(--border-soft, #e2e8f0);
  border-radius: 8px;
  background: var(--surface, #fff);
  color: var(--text-regular, #334155);
  font-size: 13px;
}

.ml-type-toggle {
  display: inline-flex;
  border: 1px solid var(--border-soft, #e2e8f0);
  border-radius: 8px;
  overflow: hidden;
}

.ml-type-btn {
  appearance: none;
  border: none;
  background: var(--surface, #fff);
  color: var(--text-muted, #64748b);
  padding: 7px 12px;
  font-size: 13px;
  cursor: pointer;
}

.ml-type-btn.is-active {
  background: var(--brand, #4f8ef7);
  color: #fff;
}

.ml-search {
  flex: 1 1 200px;
  min-width: 160px;
}

.ml-search-input {
  width: 100%;
  height: 34px;
  padding: 0 10px;
  border: 1px solid var(--border-soft, #e2e8f0);
  border-radius: 8px;
  background: var(--surface, #fff);
  color: var(--text-regular, #334155);
  font-size: 13px;
}

.ml-icon-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 34px;
  height: 34px;
  border: 1px solid var(--border-soft, #e2e8f0);
  border-radius: 8px;
  background: var(--surface, #fff);
  color: var(--text-regular, #334155);
  cursor: pointer;
}

.ml-icon-btn:disabled {
  opacity: 0.5;
  cursor: default;
}

.ml-error {
  margin: 0 0 12px;
  padding: 8px 12px;
  border-radius: 8px;
  background: var(--danger-soft, #fee2e2);
  color: var(--danger, #dc2626);
  font-size: 13px;
}

.ml-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  padding: 80px 20px;
  color: var(--text-muted, #64748b);
  text-align: center;
}

.ml-empty-title {
  margin: 0;
  font-size: 16px;
  font-weight: 600;
  color: var(--text-regular, #334155);
}

.ml-empty-sub {
  margin: 0;
  font-size: 13px;
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
}

.ml-card {
  min-width: 0;
  border-radius: 12px;
  cursor: pointer;
  outline: none;
  transition: transform 0.15s ease, box-shadow 0.15s ease;
}

.ml-card:hover,
.ml-card:focus-visible {
  transform: translateY(-2px);
  box-shadow: 0 8px 20px rgba(0, 0, 0, 0.18);
}

.ml-card--disabled {
  cursor: default;
  opacity: 0.75;
}

.ml-card__poster {
  position: relative;
  aspect-ratio: 2 / 3;
  border-radius: 10px;
  overflow: hidden;
  background: var(--surface-sunken, #f1f5f9);
}

.ml-card__poster img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  display: block;
}

.ml-card__placeholder {
  width: 100%;
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 40px;
  font-weight: 700;
  color: var(--text-muted, #94a3b8);
  background: linear-gradient(160deg, var(--surface-sunken, #f1f5f9), var(--surface-muted, #e2e8f0));
}

.ml-card__badge {
  position: absolute;
  top: 8px;
  right: 8px;
  padding: 2px 8px;
  border-radius: 999px;
  font-size: 11px;
  font-weight: 600;
  color: #fff;
  background: rgba(79, 142, 247, 0.92);
}

.ml-card__badge--muted {
  background: rgba(100, 116, 139, 0.9);
}

.ml-card__meta {
  padding: 8px 4px 0;
}

.ml-card__title {
  color: var(--text, #111827);
  font-size: 14px;
  font-weight: 600;
  line-height: 1.35;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.ml-card__sub {
  color: var(--text-muted, #64748b);
  font-size: 12px;
  line-height: 1.4;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.ml-wall-foot {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 18px 0 4px;
  color: var(--text-muted, #64748b);
  font-size: 13px;
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
  background: rgba(0, 0, 0, 0.5);
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 20px;
}

.ml-config {
  width: min(560px, 100%);
  max-height: 90vh;
  overflow: auto;
  background: var(--surface, #fff);
  border-radius: 14px;
  padding: 20px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.ml-config-title {
  margin: 0;
  font-size: 16px;
  color: var(--text, #111827);
}

.ml-config-desc {
  margin: 0;
  font-size: 12px;
  color: var(--text-muted, #64748b);
}

.ml-config-batch {
  display: flex;
  gap: 8px;
  align-items: flex-end;
}

.ml-config-textarea {
  flex: 1;
  min-width: 0;
  padding: 8px 10px;
  border: 1px solid var(--border-soft, #e2e8f0);
  border-radius: 8px;
  font-size: 12px;
  line-height: 1.6;
  resize: vertical;
  background: var(--surface-sunken, #f8fafc);
  color: var(--text-regular, #334155);
}

.ml-config-row {
  display: flex;
  gap: 8px;
}

.ml-config-input {
  flex: 1;
  min-width: 0;
  height: 34px;
  padding: 0 10px;
  border: 1px solid var(--border-soft, #e2e8f0);
  border-radius: 8px;
  font-size: 13px;
  background: var(--surface-sunken, #f8fafc);
  color: var(--text-regular, #334155);
}

.ml-config-name {
  flex: 0 0 110px;
}

.ml-config-del {
  width: 34px;
  height: 34px;
  border: 1px solid var(--border-soft, #e2e8f0);
  border-radius: 8px;
  background: var(--surface, #fff);
  color: var(--danger, #dc2626);
  cursor: pointer;
  font-size: 16px;
}

.ml-config-add {
  align-self: flex-start;
  padding: 6px 12px;
  border: 1px dashed var(--border, #cbd5e1);
  border-radius: 8px;
  background: transparent;
  color: var(--brand, #4f8ef7);
  font-size: 13px;
  cursor: pointer;
}

.ml-config-actions {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  margin-top: 4px;
}

.ml-config-save,
.ml-config-cancel {
  padding: 7px 16px;
  border-radius: 8px;
  font-size: 13px;
  cursor: pointer;
}

.ml-config-save {
  border: none;
  background: var(--brand, #4f8ef7);
  color: #fff;
}

.ml-config-save:disabled {
  opacity: 0.6;
}

.ml-config-cancel {
  border: 1px solid var(--border-soft, #e2e8f0);
  background: var(--surface, #fff);
  color: var(--text-regular, #334155);
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
</style>
