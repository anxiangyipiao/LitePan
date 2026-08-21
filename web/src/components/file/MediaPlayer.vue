<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from "vue";
import "@fortawesome/fontawesome-free/css/all.min.css";
import { fileExtension } from "@/utils/format";
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
  }
}

function exitFullscreenIfNeeded() {
  if (!isFullscreen()) return;
  void document.exitFullscreen?.().catch(() => {});
}

function lockBody(lock: boolean) {
  document.documentElement.style.overflow = lock ? "hidden" : "";
  document.body.style.overflow = lock ? "hidden" : "";
}

function closePlayer() {
  exitFullscreenIfNeeded();
  lockBody(false);
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

defineExpose({ enterFullscreen, setupPlayer });

watch(
  () => props.open,
  (open) => {
    if (!open) {
      lockBody(false);
      return;
    }
    lockBody(true);
    void nextTick(setupPlayer);
  },
);

function onKey(e: KeyboardEvent) {
  if (e.key === "Escape" && props.open) {
    // 如果在浏览器全屏中，先退全屏，再关弹窗由 closePlayer 处理
    if (isFullscreen()) exitFullscreenIfNeeded();
    else closePlayer();
  }
}

onMounted(() => window.addEventListener("keydown", onKey));
onUnmounted(() => {
  window.removeEventListener("keydown", onKey);
  lockBody(false);
});
</script>

<template>
  <Teleport to="body">
    <div v-if="open" ref="wrapRef" class="mp-screen" @click.self="closePlayer">
      <header class="mp-head">
        <span class="mp-title" :title="title">{{ title }}</span>
        <span class="mp-spacer" />
        <button type="button" class="mp-btn" title="退出" aria-label="关闭播放" @click="closePlayer">
          <i class="fa-solid fa-xmark" aria-hidden="true"></i>
        </button>
      </header>

      <div class="mp-stage" @click.self="closePlayer">
        <video ref="videoRef" class="mp-video" controls autoplay playsinline preload="metadata" @error="playerError = true" />
      </div>
      <p v-if="playerError" class="mp-error">播放失败，请稍后重试</p>
    </div>
  </Teleport>
</template>

<style scoped>
.mp-screen {
  position: fixed;
  inset: 0;
  z-index: 400;
  display: flex;
  flex-direction: column;
  background: #000;
  width: 100vw;
  height: 100vh;
  height: 100dvh;
}

/* 浏览器 Fullscreen API 时让容器真正占满屏幕 */
.mp-screen:fullscreen {
  width: 100vw;
  height: 100vh;
  background: #000;
}

.mp-head {
  flex: 0 0 auto;
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 12px 16px;
  padding-top: calc(12px + env(safe-area-inset-top, 0px));
  color: #e2e8f0;
  background: linear-gradient(to bottom, rgba(0,0,0,0.72), rgba(0,0,0,0.0));
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  z-index: 2;
  pointer-events: none;
}

.mp-title {
  font-size: 14px;
  font-weight: 600;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  text-shadow: 0 1px 8px rgba(0,0,0,0.6);
}

.mp-spacer { flex: 1; }

.mp-btn {
  pointer-events: auto;
  appearance: none;
  border: none;
  width: 36px;
  height: 36px;
  border-radius: 999px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  background: rgba(255,255,255,0.12);
  color: #e2e8f0;
  cursor: pointer;
  backdrop-filter: blur(8px);
}
.mp-btn:hover { background: rgba(255,255,255,0.2); color: #fff; }

.mp-stage {
  flex: 1 1 auto;
  min-height: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  background: #000;
}

.mp-video {
  width: 100%;
  height: 100%;
  max-width: 100vw;
  max-height: 100vh;
  max-height: 100dvh;
  object-fit: contain;
  background: #000;
  outline: none;
}

.mp-error {
  position: absolute;
  left: 50%;
  bottom: calc(24px + env(safe-area-inset-bottom, 0px));
  transform: translateX(-50%);
  margin: 0;
  padding: 8px 14px;
  border-radius: 999px;
  background: rgba(220,38,38,0.92);
  color: #fff;
  font-size: 13px;
}

@media (orientation: landscape) and (max-height: 420px) {
  .mp-head {
    padding-top: 8px;
    background: none;
  }
  .mp-title {
    display: none;
  }
}
</style>
