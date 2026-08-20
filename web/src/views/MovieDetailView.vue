<script setup lang="ts">
import { computed, nextTick, onActivated, ref, watch } from "vue";
import { useRoute, useRouter, onBeforeRouteLeave } from "vue-router";
import { mediaLibraryApi, type MediaLibraryDetail, type MediaLibraryDisc, type MediaLibraryEpisode } from "@/api/mediaLibrary";
import { getApiErrorMessage } from "@/api/client";
import SvgIcon from "@/components/icons/SvgIcon.vue";
import BusySpinner from "@/components/base/BusySpinner.vue";
import MediaPlayer from "@/components/file/MediaPlayer.vue";

const route = useRoute();
const router = useRouter();

const detail = ref<MediaLibraryDetail | null>(null);
const loading = ref(false);
const error = ref("");

// 播放器
const playerOpen = ref(false);
const playerTitle = ref("");
const playerUrl = ref("");
const playerRef = ref<{ enterFullscreen: () => Promise<void>; setupPlayer: () => Promise<void> } | null>(null);

// 背景图全屏查看
const fanartViewUrl = ref("");

function openFanart(url: string) {
  fanartViewUrl.value = url;
}

const overviewExpanded = ref(false);
watch(
  () => detail.value?.id,
  () => {
    overviewExpanded.value = false;
  },
);

const isTV = computed(() => detail.value?.media_type === "tv");

// KeepAlive 滚动记忆：离开前记录（onBeforeRouteLeave 时机 DOM 未变，最可靠）
let savedScrollY = 0;
onBeforeRouteLeave(() => {
  savedScrollY = window.scrollY || 0;
});
onActivated(() => {
  if (savedScrollY > 0) {
    requestAnimationFrame(() => window.scrollTo({ top: savedScrollY }));
  }
});

async function loadDetail() {
  const id = String(route.params.id ?? "");
  const lib = String(route.query.lib ?? "");
  if (!id || !lib) {
    error.value = "缺少影视信息";
    return;
  }
  loading.value = true;
  error.value = "";
  detail.value = null;
  try {
    detail.value = await mediaLibraryApi.detail(lib, id);
  } catch (e) {
    error.value = getApiErrorMessage(e, "详情加载失败");
  } finally {
    loading.value = false;
  }
}

watch(() => [route.params.id, route.query.lib] as const, loadDetail, { immediate: true });

async function openPlayer(title: string, url: string) {
  playerTitle.value = title;
  playerUrl.value = url;
  playerOpen.value = true;
  await nextTick();
  // 在用户点击的同步上下文后立刻尝试全屏（避免 watch 异步丢失手势）
  await playerRef.value?.enterFullscreen();
}

function playMain() {
  if (!detail.value?.play_url) return;
  void openPlayer(detail.value.title, detail.value.play_url);
}

function playEpisode(ep: MediaLibraryEpisode) {
  if (!ep.play_url) return;
  const label = `${detail.value?.title ?? ""} · S${String(ep.season ?? 1).padStart(2, "0")}E${String(ep.episode).padStart(2, "0")}`;
  void openPlayer(label.trim(), ep.play_url);
}

function playDisc(disc: MediaLibraryDisc) {
  if (!disc.play_url) return;
  void openPlayer(`${detail.value?.title ?? ""} · ${disc.label}`.trim(), disc.play_url);
}

function filterByGenre(genre: string) {
  void router.push({ path: "/movies", query: { genre } });
}

function filterByActor(actor: string) {
  void router.push({ path: "/movies", query: { actor } });
}
</script>

<template>
  <div class="detail-page">
    <div v-if="loading" class="detail-state">
      <BusySpinner :size="26" />
      <span>加载中…</span>
    </div>

    <div v-else-if="error" class="detail-state">
      <p class="detail-error">{{ error }}</p>
    </div>

    <template v-else-if="detail">
      <div
        v-if="detail.backdrop_url"
        class="detail-hero"
        :class="{ 'detail-hero--with-poster': detail.poster_url }"
        :style="{ backgroundImage: `url(${detail.backdrop_url})` }"
      >
        <div class="detail-hero__shade" />
      </div>

      <div class="detail-body" :class="{ 'detail-body--over-hero': detail.backdrop_url && detail.poster_url }">
        <div class="detail-main">
          <img
            v-if="detail.poster_url"
            :src="detail.poster_url"
            :alt="detail.title"
            class="detail-poster"
          />
          <div v-else class="detail-poster detail-poster--empty">
            {{ detail.title.slice(0, 1) }}
          </div>

          <div class="detail-info">
            <h1 class="detail-title">{{ detail.title }}</h1>
            <p class="detail-meta">
              <span>{{ detail.media_type === "tv" ? "剧集" : "电影" }}</span>
              <span v-if="detail.year"> · {{ detail.year }}</span>
              <span v-if="detail.media_type === 'tv' && detail.tv_state === 'updating'"> · 追更中</span>
              <span v-if="detail.ep_tmdb"> · 共 {{ detail.ep_tmdb }} 集</span>
            </p>

            <div v-if="detail.genres?.length" class="detail-genres">
              <button
                v-for="g in detail.genres"
                :key="g"
                type="button"
                class="detail-genre"
                @click="filterByGenre(g)"
              >
                {{ g }}
              </button>
            </div>

            <div v-if="detail.actors?.length" class="detail-actors">
              <button
                v-for="(a, i) in detail.actors"
                :key="i"
                type="button"
                class="detail-actor"
                @click="filterByActor(a)"
              >
                {{ a }}
              </button>
            </div>

            <div v-if="detail.runtime || detail.studio || detail.director" class="detail-facts">
              <span v-if="detail.runtime">时长 {{ detail.runtime }} 分钟</span>
              <span v-if="detail.director">导演 {{ detail.director }}</span>
              <span v-if="detail.studio">{{ detail.studio }}</span>
            </div>

            <p
              v-if="detail.overview"
              class="detail-overview"
              :class="{ 'detail-overview--clamp': !overviewExpanded }"
            >
              {{ detail.overview }}
            </p>
            <button
              v-if="detail.overview && detail.overview.length > 140"
              type="button"
              class="detail-overview-toggle"
              @click="overviewExpanded = !overviewExpanded"
            >
              {{ overviewExpanded ? "收起" : "展开全文" }}
            </button>

            <div v-if="!isTV" class="detail-actions">
              <button
                v-if="detail.play_url"
                type="button"
                class="detail-play"
                @click="playMain"
              >
                <SvgIcon name="play" :size="16" />
                <span>播放</span>
              </button>
              <span v-else-if="!detail.discs?.length" class="detail-nosource">该影视无可用播放源</span>
            </div>
          </div>
        </div>

        <div v-if="!isTV && detail.discs?.length" class="detail-episodes">
          <h3 class="detail-sec">碟片（{{ detail.discs.length }}）</h3>
          <div class="detail-ep-grid">
            <button
              v-for="(d, i) in detail.discs"
              :key="i"
              type="button"
              class="detail-ep"
              :disabled="!d.play_url"
              :title="d.play_url ? '播放' : '无播放源'"
              @click="playDisc(d)"
            >
              <span class="detail-ep-num">{{ d.label }}</span>
              <SvgIcon v-if="d.play_url" name="play" :size="12" />
            </button>
          </div>
        </div>

        <div v-if="isTV && detail.episodes?.length" class="detail-episodes">
          <h3 class="detail-sec">选集（{{ detail.episodes.length }}）</h3>
          <div class="detail-ep-grid">
            <button
              v-for="(ep, i) in detail.episodes"
              :key="i"
              type="button"
              class="detail-ep"
              :disabled="!ep.play_url"
              :title="ep.play_url ? '播放' : '无播放源'"
              @click="playEpisode(ep)"
            >
              <span class="detail-ep-num">
                S{{ String(ep.season ?? 1).padStart(2, "0") }}E{{ String(ep.episode).padStart(2, "0") }}
              </span>
              <SvgIcon v-if="ep.play_url" name="play" :size="12" />
            </button>
          </div>
        </div>

        <div v-if="detail.extra_fanart_urls?.length" class="detail-fanart">
          <h3 class="detail-sec">背景图（{{ detail.extra_fanart_urls.length }}）</h3>
          <div class="detail-fanart-scroll">
            <img
              v-for="(url, i) in detail.extra_fanart_urls"
              :key="i"
              :src="url"
              class="detail-fanart-img"
              loading="lazy"
              decoding="async"
              @click="openFanart(url)"
            />
          </div>
        </div>
      </div>
    </template>

    <!-- 全屏背景图查看器 -->
    <Teleport to="body">
      <Transition name="fanart-overlay">
        <div v-if="fanartViewUrl" class="fanart-overlay" @click="fanartViewUrl = ''">
          <img :src="fanartViewUrl" class="fanart-overlay__img" />
          <button type="button" class="fanart-overlay__close" aria-label="关闭" @click.stop="fanartViewUrl = ''">×</button>
        </div>
      </Transition>
    </Teleport>

    <MediaPlayer
      ref="playerRef"
      :open="playerOpen"
      :title="playerTitle"
      :play-url="playerUrl"
      :fullscreen-on-open="false"
      @close="playerOpen = false"
    />
  </div>
</template>

<style scoped>
.detail-page {
  min-height: 100vh;
  background: var(--bg);
}

.detail-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 10px;
  padding: 80px 20px;
  color: var(--text-muted);
  font-size: 14px;
}

.detail-error {
  margin: 0;
  color: var(--danger);
}

.detail-hero {
  position: relative;
  height: clamp(220px, 36vw, 380px);
  background-size: cover;
  background-position: center 30%;
}

.detail-hero::after {
  content: "";
  position: absolute;
  inset: 0 auto 0 0;
  width: 14%;
  background: linear-gradient(to right, color-mix(in srgb, var(--bg) 18%, transparent), transparent);
  pointer-events: none;
}

.detail-hero__shade {
  position: absolute;
  inset: 0;
  background: linear-gradient(
    to bottom,
    color-mix(in srgb, var(--bg) 6%, transparent) 0%,
    color-mix(in srgb, var(--bg) 22%, transparent) 55%,
    color-mix(in srgb, var(--bg) 54%, transparent) 78%,
    var(--bg) 100%
  );
}

.detail-hero--with-poster {
  margin-bottom: -56px;
}

.detail-body {
  max-width: 1000px;
  margin: 0 auto;
  padding: 20px 24px 40px;
}

.detail-body--over-hero {
  position: relative;
  z-index: 1;
}

.detail-main {
  display: flex;
  gap: 24px;
  align-items: flex-start;
}

.detail-poster {
  flex: 0 0 auto;
  width: 190px;
  aspect-ratio: 2 / 3;
  border-radius: 12px;
  object-fit: contain;
  box-shadow: 0 12px 30px rgba(0, 0, 0, 0.35);
  background: var(--surface-sunken);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 48px;
  font-weight: 700;
  color: var(--text-muted);
}

.detail-info {
  flex: 1;
  min-width: 0;
}

.detail-title {
  margin: 2px 0 8px;
  font-size: clamp(22px, 2.2vw, 28px);
  font-weight: 800;
  line-height: 1.2;
  letter-spacing: -0.02em;
  color: var(--text);
}

.detail-meta {
  margin: 0 0 12px;
  font-size: 13px;
  color: var(--text-muted);
}

.detail-overview {
  margin: 0 0 10px;
  font-size: 14px;
  line-height: 1.75;
  color: var(--text-regular);
  white-space: pre-wrap;
  word-break: break-word;
}

.detail-overview--clamp {
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.detail-overview-toggle {
  margin: 0 0 14px;
  padding: 0;
  border: none;
  background: transparent;
  color: var(--brand-strong, var(--brand));
  font-size: 12px;
  font-weight: 600;
  cursor: pointer;
}

.detail-overview-toggle:hover {
  text-decoration: underline;
}

.detail-genres {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin: 0 0 8px;
}

.detail-genre {
  padding: 2px 10px;
  border: 1px solid transparent;
  border-radius: 999px;
  background: color-mix(in srgb, var(--brand) 12%, var(--surface));
  color: var(--brand-strong, var(--brand));
  font-size: 12px;
  font-weight: 600;
  cursor: pointer;
  transition: border-color 0.15s ease, background 0.15s ease;
}

.detail-genre:hover {
  border-color: color-mix(in srgb, var(--brand) 30%, transparent);
  background: color-mix(in srgb, var(--brand) 18%, var(--surface));
}

.detail-actors {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin: 0 0 8px;
}

.detail-actor {
  padding: 2px 10px;
  border: 1px solid var(--border-soft, #e2e8f0);
  border-radius: 999px;
  background: var(--surface-sunken);
  color: var(--text-regular);
  font-size: 12px;
  font-weight: 500;
  cursor: pointer;
  transition: border-color 0.15s ease, color 0.15s ease, background 0.15s ease;
}

.detail-actor:hover {
  border-color: color-mix(in srgb, var(--brand) 26%, var(--border-soft, #e2e8f0));
  color: var(--brand-strong, var(--brand));
  background: color-mix(in srgb, var(--brand) 10%, var(--surface-sunken));
}

.detail-fanart {
  margin-top: 26px;
}

.detail-fanart-scroll {
  display: flex;
  align-items: center;
  gap: 10px;
  overflow-x: auto;
  scroll-snap-type: x mandatory;
  -webkit-overflow-scrolling: touch;
  scrollbar-width: thin;
  padding-bottom: 4px;
}

.detail-fanart-img {
  flex: 0 0 auto;
  width: 320px;
  max-height: 220px;
  border-radius: 10px;
  object-fit: contain;
  cursor: pointer;
  scroll-snap-align: start;
  background: var(--surface-sunken);
  transition: transform 0.18s ease;
}

.detail-fanart-img:hover {
  transform: scale(1.02);
}

.fanart-overlay {
  position: fixed;
  inset: 0;
  z-index: 10000;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(0, 0, 0, 0.85);
  cursor: zoom-out;
}

.fanart-overlay__img {
  max-width: 95vw;
  max-height: 90vh;
  border-radius: 8px;
  object-fit: contain;
}

.fanart-overlay__close {
  position: absolute;
  top: 16px;
  right: 20px;
  width: 40px;
  height: 40px;
  display: flex;
  align-items: center;
  justify-content: center;
  border: none;
  border-radius: 50%;
  background: rgba(255, 255, 255, 0.15);
  color: #fff;
  font-size: 24px;
  cursor: pointer;
}

.fanart-overlay__close:hover {
  background: rgba(255, 255, 255, 0.25);
}

.fanart-overlay-enter-active,
.fanart-overlay-leave-active {
  transition: opacity 0.2s ease;
}

.fanart-overlay-enter-from,
.fanart-overlay-leave-to {
  opacity: 0;
}

.detail-facts {
  display: flex;
  flex-wrap: wrap;
  gap: 4px 12px;
  margin: 0 0 10px;
  font-size: 13px;
  color: var(--text-muted);
}

.detail-actions {
  display: flex;
  gap: 10px;
  align-items: center;
}

.detail-play {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 11px 30px;
  border: none;
  border-radius: 999px;
  background: linear-gradient(135deg, var(--brand) 0%, color-mix(in srgb, var(--brand) 86%, #111827) 100%);
  color: #fff;
  font-size: 15px;
  font-weight: 700;
  letter-spacing: 0.01em;
  box-shadow: 0 10px 24px rgba(0, 0, 0, 0.18);
  cursor: pointer;
  transition: transform 0.15s ease, box-shadow 0.15s ease, filter 0.15s ease;
}

.detail-play:hover {
  transform: translateY(-1px);
  filter: brightness(1.04);
  box-shadow: 0 14px 28px rgba(0, 0, 0, 0.22);
}

.detail-play:active {
  transform: translateY(0);
}

.detail-nosource {
  font-size: 13px;
  color: var(--text-muted);
}

.detail-episodes {
  margin-top: 26px;
}

.detail-sec {
  margin: 0 0 12px;
  font-size: 14px;
  font-weight: 600;
  color: var(--text-regular);
}

.detail-ep-grid {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.detail-ep {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 8px 14px;
  border: 1px solid var(--border-soft);
  border-radius: 10px;
  background: var(--surface-sunken);
  color: var(--text-regular);
  font-size: 13px;
  font-weight: 500;
  cursor: pointer;
  transition: border-color 0.15s ease, background 0.15s ease, color 0.15s ease, transform 0.15s ease;
}

.detail-ep:hover:not(:disabled) {
  border-color: color-mix(in srgb, var(--brand) 26%, var(--border-soft));
  background: color-mix(in srgb, var(--brand) 10%, var(--surface-sunken));
  color: var(--brand-strong, var(--brand));
  transform: translateY(-1px);
}

.detail-ep:disabled {
  opacity: 0.45;
  cursor: default;
}

.detail-ep:focus-visible {
  outline: 2px solid color-mix(in srgb, var(--brand) 36%, transparent);
  outline-offset: 2px;
}

.detail-ep-num {
  font-weight: 700;
}

@media (max-width: 640px) {
  .detail-main {
    flex-direction: column;
  }

  .detail-poster {
    width: 130px;
  }

  .detail-hero {
    height: 190px;
  }

  .detail-hero--with-poster {
    margin-bottom: -36px;
  }

  .detail-body {
    padding: 10px 14px calc(16px + env(safe-area-inset-bottom, 0));
  }

  .detail-actions {
    position: sticky;
    bottom: calc(10px + env(safe-area-inset-bottom, 0));
    z-index: 2;
    padding: 10px 12px;
    margin: 14px -14px 0;
    background: color-mix(in srgb, var(--surface) 92%, transparent);
    backdrop-filter: blur(10px);
    border-top: 1px solid var(--border-soft);
  }

  .detail-fanart-img {
    width: 240px;
    max-height: 170px;
  }
}
</style>
