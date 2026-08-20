<script setup lang="ts">
import { computed, nextTick, ref, watch } from "vue";
import "@fortawesome/fontawesome-free/css/all.min.css";
import { fileExtension } from "@/utils/format";
import SvgIcon from "@/components/icons/SvgIcon.vue";

// 影视模式通用播放器：内嵌视频 + 外部播放器（VLC/PotPlayer/IINA/mpv/Skybox）。
// 支持 hls.js / mpegts.js 流；外部播放器走 URL scheme（Skybox 走 WebDAV 指引）。

const props = withDefaults(
  defineProps<{
    open: boolean;
    title: string;
    playUrl: string;
    fullscreenOnOpen?: boolean;
  }>(),
  { fullscreenOnOpen: true },
);

const emit = defineEmits<{ close: [] }>();

const videoRef = ref<HTMLVideoElement | null>(null);
const wrapRef = ref<HTMLDivElement | null>(null);
const playerError = ref(false);
const playerMenuOpen = ref(false);
const skyboxGuideOpen = ref(false);

type HlsLike = import("hls.js").default;
interface MpegtsLike {
  destroy(): void;
  detachMediaElement(): void;
  attachMediaElement(el: HTMLMediaElement): void;
  load(): void;
}
let playerHls: HlsLike | null = null;
let playerMpegts: MpegtsLike | null = null;
let playerSession = 0;

const src = computed(() =>
  props.playUrl ? `${window.location.origin}${props.playUrl}` : "",
);

function isFullscreen() {
  return Boolean(document.fullscreenElement);
}

async function enterFullscreen() {
  const el = wrapRef.value as unknown as HTMLElement | null;
  if (!el || isFullscreen()) return;
  try {
    await el.requestFullscreen?.();
  } catch {
    // 忽略用户手势/策略限制导致的失败，保留弹窗可播放
  }
}

function exitFullscreenIfNeeded() {
  if (!isFullscreen()) return;
  void document.exitFullscreen?.().catch(() => {});
}

const externalPlayers = [
  { name: "VLC", icon: "fa-brands fa-vlc", buildUrl: (url: string) => `vlc://${url}` },
  { name: "PotPlayer", icon: "fa-solid fa-play", buildUrl: (url: string) => `potplayer://${url}` },
  { name: "IINA", icon: "fa-solid fa-play", buildUrl: (url: string) => `iina://weblink?url=${encodeURIComponent(url)}` },
  { name: "mpv", icon: "fa-solid fa-play", buildUrl: (url: string) => `mpv://${url}` },
];
const webdavUrl = computed(() => `http://${window.location.host}/dav`);

function closePlayer() {
  exitFullscreenIfNeeded();
  playerSession += 1;
  playerHls?.destroy();
  playerHls = null;
  if (playerMpegts) {
    playerMpegts.detachMediaElement();
    playerMpegts.destroy();
    playerMpegts = null;
  }
  const v = videoRef.value;
  if (v) {
    v.pause();
    v.removeAttribute("src");
    v.load();
  }
  playerMenuOpen.value = false;
  skyboxGuideOpen.value = false;
  emit("close");
}

async function setupPlayer() {
  const video = videoRef.value;
  if (!video || !src.value) return;
  const session = ++playerSession;
  playerError.value = false;
  const path = src.value.split("?")[0];
  const ext = fileExtension(path);
  if (ext === "m3u8" && !video.canPlayType("application/vnd.apple.mpegurl")) {
    const { default: Hls } = await import("hls.js");
    if (session !== playerSession) return;
    if (Hls.isSupported()) {
      const p = new Hls({ backBufferLength: 60, maxBufferLength: 30, maxMaxBufferLength: 60 });
      playerHls = p;
      p.loadSource(src.value);
      p.attachMedia(video);
      p.on(Hls.Events.ERROR, (_evt: unknown, data: { fatal?: boolean }) => {
        if (data?.fatal) playerError.value = true;
      });
      return;
    }
  }
  if (["flv", "ts", "m2ts"].includes(ext)) {
    const m = await import("mpegts.js");
    if (session !== playerSession) return;
    const mpegts = m.default;
    if (mpegts.getFeatureList().msePlayback) {
      const p = mpegts.createPlayer({ type: ext === "flv" ? "flv" : "mpegts", url: src.value, isLive: false });
      playerMpegts = p;
      p.attachMediaElement(video);
      p.load();
      return;
    }
  }
  video.src = src.value;
  void video.play().catch(() => {});
}

watch(
  () => props.open,
  (open) => {
    if (!open) return;
    void nextTick(async () => {
      await setupPlayer();
      if (props.fullscreenOnOpen) void enterFullscreen();
    });
  },
);

function openExternalPlayer(url: string) {
  window.location.href = url;
  playerMenuOpen.value = false;
}
</script>

<template>
  <div v-if="open" class="mp-mask" @click.self="closePlayer">
    <div ref="wrapRef" class="mp">
      <header class="mp-head">
        <span class="mp-title" :title="title">{{ title }}</span>
        <span class="mp-meta">播放中</span>
        <span class="mp-spacer" />
        <button type="button" class="mp-close" title="全屏" @click="enterFullscreen">
          <i class="fa-solid fa-expand" aria-hidden="true"></i>
        </button>
        <button type="button" class="mp-close" title="关闭" @click="closePlayer">
          <SvgIcon name="sign-out" :size="14" />
        </button>
      </header>

      <video ref="videoRef" class="mp-video" controls playsinline />
      <p v-if="playerError" class="mp-error">播放失败，可尝试外部播放器</p>

      <div class="mp-foot">
        <div class="mp-ext">
          <button
            v-for="p in externalPlayers"
            :key="p.name"
            type="button"
            class="mp-ext-btn"
            :disabled="!src"
            @click="openExternalPlayer(p.buildUrl(src))"
          >
            <i :class="p.icon" aria-hidden="true"></i>
            <span>{{ p.name }}</span>
          </button>
          <button type="button" class="mp-ext-btn" @click="skyboxGuideOpen = !skyboxGuideOpen">
            <i class="fa-solid fa-vr-cardboard" aria-hidden="true"></i>
            <span>Skybox</span>
          </button>
        </div>
        <p v-if="skyboxGuideOpen" class="mp-skybox-guide">
          Quest 端 Skybox 不支持 URL 调用。请在 Skybox → 设置 → 网络 → WebDAV 添加：地址
          <code>{{ webdavUrl }}</code>，账号为管理员账号密码，即可浏览并播放网盘内 VR 视频。
        </p>
      </div>
    </div>
  </div>
</template>

<style scoped>
.mp-mask {
  position: fixed;
  inset: 0;
  z-index: 200;
  background: rgba(0, 0, 0, 0.72);
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px;
}

.mp {
  width: min(960px, 100%);
  max-height: 100%;
  background: #0f172a;
  border-radius: 14px;
  overflow: hidden;
  display: flex;
  flex-direction: column;
}

.mp:fullscreen {
  width: 100vw;
  height: 100vh;
  max-height: none;
  border-radius: 0;
  background: #000;
}

.mp:fullscreen .mp-video {
  flex: 1 1 auto;
  aspect-ratio: auto;
  min-height: 0;
}

.mp-head {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 14px;
  color: #e2e8f0;
}

.mp-title {
  font-size: 15px;
  font-weight: 600;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.mp-meta {
  font-size: 12px;
  color: #94a3b8;
  border: 1px solid #334155;
  border-radius: 999px;
  padding: 1px 8px;
  flex-shrink: 0;
}

.mp-spacer {
  flex: 1;
}

.mp-close {
  appearance: none;
  border: none;
  background: transparent;
  color: #94a3b8;
  cursor: pointer;
  padding: 4px;
}

.mp-close:hover {
  color: #fff;
}

.mp-video {
  width: 100%;
  aspect-ratio: 16 / 9;
  background: #000;
}

.mp-error {
  margin: 0;
  padding: 8px 14px;
  color: #fca5a5;
  font-size: 13px;
}

.mp-foot {
  padding: 12px 14px;
}

.mp-ext {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

.mp-ext-btn {
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

.mp-ext-btn:disabled {
  opacity: 0.5;
  cursor: default;
}

.mp-skybox-guide {
  margin: 12px 0 0;
  font-size: 12px;
  line-height: 1.6;
  color: #94a3b8;
}

.mp-skybox-guide code {
  color: #e2e8f0;
  background: #1e293b;
  padding: 1px 6px;
  border-radius: 4px;
}
</style>
