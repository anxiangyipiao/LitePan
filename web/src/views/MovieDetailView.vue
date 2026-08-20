<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useRoute } from "vue-router";
import { mediaLibraryApi, type MediaLibraryDetail, type MediaLibraryEpisode } from "@/api/mediaLibrary";
import { getApiErrorMessage } from "@/api/client";
import SvgIcon from "@/components/icons/SvgIcon.vue";
import BusySpinner from "@/components/base/BusySpinner.vue";
import MediaPlayer from "@/components/file/MediaPlayer.vue";

const route = useRoute();

const detail = ref<MediaLibraryDetail | null>(null);
const loading = ref(false);
const error = ref("");

// 播放器
const playerOpen = ref(false);
const playerTitle = ref("");
const playerUrl = ref("");

const isTV = computed(() => detail.value?.media_type === "tv");

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

function playMain() {
  if (!detail.value?.play_url) return;
  playerTitle.value = detail.value.title;
  playerUrl.value = detail.value.play_url;
  playerOpen.value = true;
}

function playEpisode(ep: MediaLibraryEpisode) {
  if (!ep.play_url) return;
  const label = `${detail.value?.title ?? ""} · S${String(ep.season ?? 1).padStart(2, "0")}E${String(ep.episode).padStart(2, "0")}`;
  playerTitle.value = label.trim();
  playerUrl.value = ep.play_url;
  playerOpen.value = true;
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
      <RouterLink to="/movies" class="detail-back" title="返回影视">
        ← 返回影视
      </RouterLink>

      <div v-if="detail.backdrop_url" class="detail-hero" :style="{ backgroundImage: `url(${detail.backdrop_url})` }">
        <div class="detail-hero__shade" />
      </div>

      <div class="detail-body">
        <div class="detail-main">
          <img
            v-if="detail.poster_url"
            :src="detail.poster_url"
            :alt="detail.title"
            class="detail-poster"
          />
          <div v-else class="detail-poster detail-poster--empty">{{ detail.title.slice(0, 1) }}</div>

          <div class="detail-info">
            <h1 class="detail-title">{{ detail.title }}</h1>
            <p class="detail-meta">
              <span>{{ detail.media_type === "tv" ? "剧集" : "电影" }}</span>
              <span v-if="detail.year"> · {{ detail.year }}</span>
              <span v-if="detail.media_type === 'tv' && detail.tv_state === 'updating'"> · 追更中</span>
              <span v-if="detail.ep_tmdb"> · 共 {{ detail.ep_tmdb }} 集</span>
            </p>

            <div v-if="detail.genres?.length" class="detail-genres">
              <span v-for="g in detail.genres" :key="g" class="detail-genre">{{ g }}</span>
            </div>

            <div v-if="detail.runtime || detail.studio || detail.director || detail.tmdb_id" class="detail-facts">
              <span v-if="detail.runtime">时长 {{ detail.runtime }} 分钟</span>
              <span v-if="detail.director">导演 {{ detail.director }}</span>
              <span v-if="detail.studio">{{ detail.studio }}</span>
              <a
                v-if="detail.tmdb_id"
                :href="`https://www.themoviedb.org/${detail.media_type === 'tv' ? 'tv' : 'movie'}/${detail.tmdb_id}`"
                target="_blank"
                rel="noopener"
                class="detail-tmdb"
              >
                TMDB ↗
              </a>
            </div>

            <p v-if="detail.overview" class="detail-overview">{{ detail.overview }}</p>

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
              <span v-else class="detail-nosource">该影视无可用播放源</span>
            </div>
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
      </div>
    </template>

    <MediaPlayer
      :open="playerOpen"
      :title="playerTitle"
      :play-url="playerUrl"
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

.detail-back {
  position: sticky;
  top: 0;
  z-index: 2;
  display: inline-flex;
  align-items: center;
  padding: 14px 20px 8px;
  color: var(--text-regular);
  text-decoration: none;
  font-size: 14px;
  font-weight: 600;
  background: var(--bg);
}

.detail-back:hover {
  color: var(--brand);
}

.detail-hero {
  position: relative;
  height: 280px;
  background-size: cover;
  background-position: center 30%;
}

.detail-hero__shade {
  position: absolute;
  inset: 0;
  background: linear-gradient(to bottom, transparent 20%, var(--bg) 100%);
}

.detail-body {
  max-width: 900px;
  margin: 0 auto;
  padding: 0 20px 32px;
  margin-top: -8px;
}

.detail-main {
  display: flex;
  gap: 24px;
  align-items: flex-start;
}

.detail-poster {
  flex: 0 0 auto;
  width: 180px;
  aspect-ratio: 2 / 3;
  border-radius: 12px;
  object-fit: cover;
  box-shadow: 0 12px 30px rgba(0, 0, 0, 0.3);
  background: var(--surface-sunken);
  margin-top: -72px;
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
  margin: 4px 0 6px;
  font-size: 24px;
  font-weight: 700;
  color: var(--text);
}

.detail-meta {
  margin: 0 0 10px;
  font-size: 13px;
  color: var(--text-muted);
}

.detail-overview {
  margin: 0 0 18px;
  font-size: 14px;
  line-height: 1.7;
  color: var(--text-regular);
}

.detail-genres {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin: 0 0 8px;
}

.detail-genre {
  padding: 2px 10px;
  border-radius: 999px;
  background: color-mix(in srgb, var(--brand) 12%, var(--surface));
  color: var(--brand-strong, var(--brand));
  font-size: 12px;
  font-weight: 600;
}

.detail-facts {
  display: flex;
  flex-wrap: wrap;
  gap: 4px 12px;
  margin: 0 0 10px;
  font-size: 13px;
  color: var(--text-muted);
}

.detail-tmdb {
  color: var(--brand);
  text-decoration: none;
  font-weight: 600;
}

.detail-tmdb:hover {
  text-decoration: underline;
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
  padding: 10px 28px;
  border: none;
  border-radius: 999px;
  background: var(--brand);
  color: #fff;
  font-size: 15px;
  font-weight: 600;
  cursor: pointer;
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
  border-radius: 8px;
  background: var(--surface-sunken);
  color: var(--text-regular);
  font-size: 13px;
  cursor: pointer;
}

.detail-ep:disabled {
  opacity: 0.45;
  cursor: default;
}

.detail-ep-num {
  font-weight: 600;
}

@media (max-width: 640px) {
  .detail-main {
    flex-direction: column;
  }

  .detail-poster {
    width: 120px;
    margin-top: -48px;
  }

  .detail-hero {
    height: 180px;
  }
}
</style>
