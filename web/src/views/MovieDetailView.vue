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
  background: linear-gradient(180deg, #0a0a0f 0%, #14141c 100%);
}

.detail-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 12px;
  padding: 100px 20px;
  color: #888;
  font-size: 14px;
}

.detail-error {
  margin: 0;
  color: #f87171;
  padding: 12px 20px;
  background: rgba(220, 38, 38, 0.15);
  border-radius: 10px;
  border: 1px solid rgba(220, 38, 38, 0.3);
}

.detail-hero {
  position: relative;
  height: clamp(260px, 40vw, 420px);
  background-size: cover;
  background-position: center 25%;
}

.detail-hero::after {
  content: "";
  position: absolute;
  inset: 0 auto 0 0;
  width: 20%;
  background: linear-gradient(to right, #0a0a0f 0%, transparent 100%);
  pointer-events: none;
}

.detail-hero::before {
  content: "";
  position: absolute;
  inset: auto 0 0 0;
  height: 30%;
  background: linear-gradient(to top, #0a0a0f 0%, transparent 100%);
  pointer-events: none;
}

.detail-hero__shade {
  position: absolute;
  inset: 0;
  background: linear-gradient(
    to bottom,
    rgba(10, 10, 15, 0.3) 0%,
    rgba(10, 10, 15, 0.5) 50%,
    #0a0a0f 100%
  );
}

.detail-hero--with-poster {
  margin-bottom: -70px;
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
  gap: 28px;
  align-items: flex-start;
}

.detail-poster {
  flex: 0 0 auto;
  width: 200px;
  aspect-ratio: 2 / 3;
  border-radius: 14px;
  object-fit: cover;
  box-shadow: 0 20px 50px rgba(0, 0, 0, 0.6), 0 0 30px rgba(255, 215, 0, 0.1);
  background: #1a1a24;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 56px;
  font-weight: 700;
  color: #555;
  border: 3px solid rgba(255, 215, 0, 0.2);
  transition: all 0.3s ease;
}

.detail-poster:hover {
  border-color: rgba(255, 215, 0, 0.5);
  box-shadow: 0 25px 60px rgba(0, 0, 0, 0.7), 0 0 40px rgba(255, 215, 0, 0.2);
}

.detail-info {
  flex: 1;
  min-width: 0;
}

.detail-title {
  margin: 4px 0 10px;
  font-size: clamp(24px, 2.4vw, 32px);
  font-weight: 800;
  line-height: 1.2;
  letter-spacing: -0.02em;
  color: #f0f0f0;
  text-shadow: 0 2px 10px rgba(0, 0, 0, 0.3);
}

.detail-meta {
  margin: 0 0 14px;
  font-size: 14px;
  color: #aaa;
}

.detail-overview {
  margin: 0 0 12px;
  font-size: 14px;
  line-height: 1.8;
  color: #c0c0c0;
  white-space: pre-wrap;
  word-break: break-word;
}

.detail-overview--clamp {
  display: -webkit-box;
  -webkit-line-clamp: 3;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.detail-overview-toggle {
  margin: 0 0 16px;
  padding: 0;
  border: none;
  background: transparent;
  color: #ffd700;
  font-size: 13px;
  font-weight: 600;
  cursor: pointer;
  transition: color 0.2s ease;
}

.detail-overview-toggle:hover {
  color: #ffe44d;
}

.detail-genres {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin: 0 0 10px;
}

.detail-genre {
  padding: 4px 12px;
  border: 1px solid rgba(255, 215, 0, 0.3);
  border-radius: 999px;
  background: rgba(255, 215, 0, 0.08);
  color: #ffd700;
  font-size: 12px;
  font-weight: 600;
  cursor: pointer;
  transition: all 0.2s ease;
}

.detail-genre:hover {
  border-color: rgba(255, 215, 0, 0.6);
  background: rgba(255, 215, 0, 0.15);
  transform: translateY(-1px);
}

.detail-actors {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin: 0 0 10px;
}

.detail-actor {
  padding: 4px 12px;
  border: 1px solid rgba(255, 255, 255, 0.12);
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.05);
  color: #c0c0c0;
  font-size: 12px;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.2s ease;
}

.detail-actor:hover {
  border-color: rgba(255, 215, 0, 0.4);
  color: #ffd700;
  background: rgba(255, 215, 0, 0.08);
  transform: translateY(-1px);
}

.detail-fanart {
  margin-top: 30px;
}

.detail-fanart-scroll {
  display: flex;
  align-items: center;
  gap: 12px;
  overflow-x: auto;
  scroll-snap-type: x mandatory;
  -webkit-overflow-scrolling: touch;
  scrollbar-width: thin;
  padding-bottom: 8px;
}

.detail-fanart-img {
  flex: 0 0 auto;
  width: 340px;
  max-height: 220px;
  border-radius: 12px;
  object-fit: cover;
  cursor: pointer;
  scroll-snap-align: start;
  background: #1a1a24;
  border: 2px solid transparent;
  transition: all 0.3s ease;
}

.detail-fanart-img:hover {
  transform: scale(1.03);
  border-color: rgba(255, 215, 0, 0.4);
  box-shadow: 0 10px 30px rgba(0, 0, 0, 0.4);
}

.fanart-overlay {
  position: fixed;
  inset: 0;
  z-index: 10000;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(0, 0, 0, 0.92);
  backdrop-filter: blur(8px);
  cursor: zoom-out;
}

.fanart-overlay__img {
  max-width: 95vw;
  max-height: 92vh;
  border-radius: 12px;
  object-fit: contain;
  box-shadow: 0 25px 60px rgba(0, 0, 0, 0.5);
}

.fanart-overlay__close {
  position: absolute;
  top: 20px;
  right: 24px;
  width: 44px;
  height: 44px;
  display: flex;
  align-items: center;
  justify-content: center;
  border: none;
  border-radius: 50%;
  background: rgba(255, 255, 255, 0.1);
  color: #fff;
  font-size: 24px;
  cursor: pointer;
  transition: all 0.2s ease;
}

.fanart-overlay__close:hover {
  background: rgba(255, 255, 255, 0.2);
  transform: scale(1.1);
}

.fanart-overlay-enter-active,
.fanart-overlay-leave-active {
  transition: opacity 0.25s ease;
}

.fanart-overlay-enter-from,
.fanart-overlay-leave-to {
  opacity: 0;
}

.detail-facts {
  display: flex;
  flex-wrap: wrap;
  gap: 6px 16px;
  margin: 0 0 12px;
  font-size: 13px;
  color: #999;
}

.detail-actions {
  display: flex;
  gap: 12px;
  align-items: center;
  margin-top: 8px;
}

.detail-play {
  display: inline-flex;
  align-items: center;
  gap: 10px;
  padding: 14px 36px;
  border: none;
  border-radius: 999px;
  background: linear-gradient(135deg, #ffd700 0%, #ffaa00 100%);
  color: #000;
  font-size: 16px;
  font-weight: 700;
  letter-spacing: 0.02em;
  box-shadow: 0 12px 30px rgba(255, 215, 0, 0.35);
  cursor: pointer;
  transition: all 0.25s ease;
}

.detail-play:hover {
  transform: translateY(-2px);
  box-shadow: 0 16px 40px rgba(255, 215, 0, 0.45);
  filter: brightness(1.05);
}

.detail-play:active {
  transform: translateY(0);
}

.detail-nosource {
  font-size: 14px;
  color: #777;
  padding: 12px 16px;
  background: rgba(255, 255, 255, 0.04);
  border-radius: 10px;
  border: 1px solid rgba(255, 255, 255, 0.08);
}

.detail-episodes {
  margin-top: 32px;
}

.detail-sec {
  margin: 0 0 14px;
  font-size: 16px;
  font-weight: 700;
  color: #e0e0e0;
}

.detail-ep-grid {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
}

.detail-ep {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  min-height: 48px;
  padding: 10px 16px;
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: 12px;
  background: rgba(255, 255, 255, 0.04);
  color: #c0c0c0;
  font-size: 13px;
  font-weight: 500;
  cursor: pointer;
  touch-action: manipulation;
  transition: all 0.2s ease;
}

.detail-ep:hover:not(:disabled) {
  border-color: rgba(255, 215, 0, 0.5);
  background: rgba(255, 215, 0, 0.1);
  color: #ffd700;
  transform: translateY(-2px);
  box-shadow: 0 8px 20px rgba(0, 0, 0, 0.3);
}

.detail-ep:disabled {
  opacity: 0.35;
  cursor: default;
}

.detail-ep:focus-visible {
  outline: 2px solid rgba(255, 215, 0, 0.5);
  outline-offset: 2px;
}

.detail-ep-num {
  font-weight: 700;
  font-size: 14px;
}

@media (max-width: 640px) {
  .detail-main {
    flex-direction: column;
    align-items: center;
    text-align: center;
  }

  .detail-poster {
    width: 150px;
    margin-top: -50px;
  }

  .detail-hero {
    height: 200px;
  }

  .detail-hero--with-poster {
    margin-bottom: -50px;
  }

  .detail-body {
    padding: 16px 16px calc(20px + env(safe-area-inset-bottom, 0));
  }

  .detail-title {
    font-size: 22px;
  }

  .detail-genres,
  .detail-actors {
    justify-content: center;
  }

  .detail-actions {
    position: sticky;
    bottom: calc(10px + env(safe-area-inset-bottom, 0));
    z-index: 2;
    padding: 12px 16px;
    margin: 16px -16px 0;
    background: rgba(15, 15, 25, 0.92);
    backdrop-filter: blur(12px);
    border-top: 1px solid rgba(255, 255, 255, 0.1);
    justify-content: center;
  }

  .detail-play {
    width: 100%;
    justify-content: center;
    padding: 14px 24px;
  }

  .detail-fanart-img {
    width: 260px;
    max-height: 180px;
  }
}
</style>
