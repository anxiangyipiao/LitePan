<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from "vue";
import "@fortawesome/fontawesome-free/css/all.min.css";
import { filesApi } from "@/api/files";
import type { FileItem } from "@/api/types";
import { fileKind } from "@/utils/fileIcon";
import { fileExtension } from "@/utils/format";

const props = defineProps<{
  accountId: number;
  files: FileItem[];
  initialFileId: string;
}>();

const emit = defineEmits<{
  close: [];
  download: [file: FileItem];
}>();

const videoRef = ref<HTMLVideoElement | null>(null);
const wrapRef = ref<HTMLDivElement | null>(null);
const mediaLoading = ref(true);
const mediaError = ref(false);
const notice = ref("");
let noticeTimer: number | undefined;

let mediaSession = 0;
let hlsPlayer: { destroy(): void } | null = null;
let mpegtsPlayer: { destroy(): void; detachMediaElement(): void } | null = null;

const episodes = computed(() =>
  props.files
    .filter((file) => !file.is_dir && fileKind(file) === "video")
    .sort((left, right) =>
      left.name.localeCompare(right.name, undefined, { numeric: true, sensitivity: "base" }),
    ),
);

const initialIndex = episodes.value.findIndex((file) => file.id === props.initialFileId);
const currentIndex = ref(initialIndex >= 0 ? initialIndex : 0);
const currentFile = computed(() => episodes.value[currentIndex.value] ?? null);
const mediaURL = computed(() => {
  const file = currentFile.value;
  return file ? filesApi.previewURL(props.accountId, file.id, file.name) : "";
});

function fileStem(name: string) {
  return name.replace(/\.[^.]+$/, "");
}

function isFullscreen() {
  return Boolean(document.fullscreenElement);
}

async function enterFullscreen() {
  const el = wrapRef.value as unknown as HTMLElement | null;
  if (!el || isFullscreen()) return;
  try {
    await el.requestFullscreen?.();
  } catch {
    // ignore
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

function showNotice(message: string) {
  notice.value = message;
  window.clearTimeout(noticeTimer);
  noticeTimer = window.setTimeout(() => {
    notice.value = "";
  }, 1500);
}

function closePlayer() {
  exitFullscreenIfNeeded();
  lockBody(false);
  destroyMediaAdapters();
  const v = videoRef.value;
  if (v) {
    v.pause();
    v.removeAttribute("src");
    v.load();
  }
  emit("close");
}

function destroyMediaAdapters() {
  mediaSession += 1;
  hlsPlayer?.destroy();
  hlsPlayer = null;
  if (mpegtsPlayer) {
    mpegtsPlayer.detachMediaElement();
    mpegtsPlayer.destroy();
    mpegtsPlayer = null;
  }
}

async function setupMediaSource() {
  const video = videoRef.value;
  const file = currentFile.value;
  if (!video || !file) return;
  destroyMediaAdapters();
  const session = mediaSession;
  const url = mediaURL.value;
  const ext = fileExtension(file.name);

  if (ext === "m3u8" && !video.canPlayType("application/vnd.apple.mpegurl")) {
    const { default: Hls } = await import("hls.js");
    if (session !== mediaSession || video !== videoRef.value) return;
    if (!Hls.isSupported()) throw new Error("当前浏览器不支持 HLS 播放");
    const player = new Hls({
      backBufferLength: 60,
      maxBufferLength: 30,
      maxMaxBufferLength: 60,
    });
    player.loadSource(url);
    player.attachMedia(video);
    player.on(Hls.Events.ERROR, (_evt: unknown, data: { fatal?: boolean }) => {
      if (data?.fatal) mediaError.value = true;
    });
    hlsPlayer = player as unknown as { destroy(): void };
    return;
  }

  if (["flv", "ts", "m2ts"].includes(ext)) {
    const module = await import("mpegts.js");
    const mpegts = module.default;
    if (session !== mediaSession || video !== videoRef.value) return;
    if (!mpegts.getFeatureList().msePlayback) throw new Error("当前浏览器不支持 FLV/MPEG-TS 播放");
    const player = mpegts.createPlayer({ type: ext === "flv" ? "flv" : "mpegts", url, isLive: false });
    player.attachMediaElement(video);
    player.load();
    mpegtsPlayer = player as unknown as { destroy(): void; detachMediaElement(): void };
    return;
  }

  video.src = url;
  video.load();
}

async function playCurrent() {
  await nextTick();
  try {
    await setupMediaSource();
  } catch (reason) {
    mediaLoading.value = false;
    mediaError.value = true;
    showNotice(reason instanceof Error ? reason.message : "视频初始化失败");
    return;
  }
  await videoRef.value?.play().catch(() => undefined);
}

async function selectEpisode(index: number) {
  if (index < 0 || index >= episodes.value.length) return;
  if (index === currentIndex.value) return;
  currentIndex.value = index;
  mediaError.value = false;
  mediaLoading.value = true;
  const file = episodes.value[index];
  showNotice(fileStem(file.name));
  await playCurrent();
}

function playAdjacent(direction: -1 | 1) {
  const target = currentIndex.value + direction;
  if (target < 0 || target >= episodes.value.length) {
    showNotice(direction > 0 ? "已经是最后一个视频" : "已经是第一个视频");
    return;
  }
  void selectEpisode(target);
}

function handleMediaReady() {
  mediaLoading.value = false;
  mediaError.value = false;
}

function handleMediaWaiting() {
  mediaLoading.value = true;
}

function handleMediaError() {
  mediaLoading.value = false;
  mediaError.value = true;
}

function downloadCurrent() {
  if (currentFile.value) emit("download", currentFile.value);
}

function isEditableTarget(target: EventTarget | null) {
  const element = target instanceof HTMLElement ? target : null;
  return !!element?.closest("input, textarea, select, [contenteditable='true']");
}

function handleKeydown(event: KeyboardEvent) {
  if (isEditableTarget(event.target)) return;
  const key = event.key.toLowerCase();
  if (key === "escape") {
    event.preventDefault();
    if (isFullscreen()) exitFullscreenIfNeeded();
    else closePlayer();
    return;
  }
  if (["arrowleft", "arrowright", " "].includes(key)) event.preventDefault();
  if (key === "arrowleft") {
    const v = videoRef.value;
    if (v) v.currentTime = Math.max(0, v.currentTime - 10);
  } else if (key === "arrowright") {
    const v = videoRef.value;
    if (v) {
      const d = Number.isFinite(v.duration) ? v.duration : Infinity;
      v.currentTime = Math.min(d, v.currentTime + 10);
    }
  } else if (key === " ") {
    const v = videoRef.value;
    if (!v) return;
    if (v.paused) void v.play().catch(() => undefined);
    else v.pause();
  } else if (key === "n") playAdjacent(1);
  else if (key === "p") playAdjacent(-1);
}

watch(currentFile, () => {
  // currentIndex 变化已在 selectEpisode 中触发 playCurrent，这里兜底文件列表变化
});

defineExpose({ enterFullscreen });

onMounted(() => {
  lockBody(true);
  window.addEventListener("keydown", handleKeydown);
  void playCurrent();
});

onUnmounted(() => {
  window.clearTimeout(noticeTimer);
  destroyMediaAdapters();
  window.removeEventListener("keydown", handleKeydown);
  lockBody(false);
});
</script>

<template>
  <Teleport to="body">
    <div ref="wrapRef" class="mp-screen" @click.self="closePlayer">
      <header class="mp-head">
        <div class="mp-head__left">
          <span class="mp-title" :title="currentFile?.name ?? ''">{{ currentFile?.name ?? "" }}</span>
          <span v-if="episodes.length > 1" class="mp-count">第 {{ currentIndex + 1 }} 集 / 共 {{ episodes.length }} 集</span>
        </div>
        <span class="mp-spacer" />
        <div v-if="episodes.length > 1" class="mp-nav">
          <button
            type="button"
            class="mp-nav__btn"
            :disabled="currentIndex <= 0"
            aria-label="上一个视频"
            title="上一个 (P)"
            @click="playAdjacent(-1)"
          >
            <i class="fa-solid fa-chevron-left" aria-hidden="true" />
          </button>
          <button
            type="button"
            class="mp-nav__btn"
            :disabled="currentIndex >= episodes.length - 1"
            aria-label="下一个视频"
            title="下一个 (N)"
            @click="playAdjacent(1)"
          >
            <i class="fa-solid fa-chevron-right" aria-hidden="true" />
          </button>
        </div>
        <button type="button" class="mp-btn" title="退出" aria-label="关闭播放" @click="closePlayer">
          <i class="fa-solid fa-xmark" aria-hidden="true"></i>
        </button>
      </header>

      <div class="mp-stage" @click.self="closePlayer">
        <video
          v-if="currentFile"
          ref="videoRef"
          :key="currentFile.id"
          class="mp-video"
          controls
          autoplay
          playsinline
          preload="metadata"
          @canplay="handleMediaReady"
          @playing="handleMediaReady"
          @waiting="handleMediaWaiting"
          @error="handleMediaError"
          @ended="playAdjacent(1)"
        />
      </div>

      <Transition name="mp-notice">
        <div v-if="notice" class="mp-notice" role="status">{{ notice }}</div>
      </Transition>

      <div v-if="mediaLoading && !mediaError" class="mp-loading" aria-label="正在加载视频">
        <i class="fa-solid fa-circle-notch fa-spin" aria-hidden="true" />
        <span>正在加载视频…</span>
      </div>

      <div v-if="mediaError" class="mp-error-wrap" role="alert">
        <i class="fa-solid fa-circle-exclamation" aria-hidden="true" />
        <strong>浏览器无法直接播放这个视频</strong>
        <span>可能是视频封装或编码不受支持，可下载后用本地播放器打开。</span>
        <button type="button" class="mp-error__action" @click="downloadCurrent">下载视频</button>
      </div>
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
  background: linear-gradient(to bottom, rgba(0, 0, 0, 0.72), rgba(0, 0, 0, 0));
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  z-index: 2;
  pointer-events: none;
}

.mp-head__left {
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.mp-title {
  font-size: 14px;
  font-weight: 600;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  text-shadow: 0 1px 8px rgba(0, 0, 0, 0.6);
}

.mp-count {
  font-size: 11px;
  color: rgba(226, 232, 240, 0.72);
  text-shadow: 0 1px 6px rgba(0, 0, 0, 0.6);
}

.mp-spacer {
  flex: 1;
}

.mp-nav {
  pointer-events: auto;
  display: inline-flex;
  gap: 6px;
}

.mp-nav__btn {
  appearance: none;
  border: none;
  width: 36px;
  height: 36px;
  border-radius: 999px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  background: rgba(255, 255, 255, 0.12);
  color: #e2e8f0;
  cursor: pointer;
  backdrop-filter: blur(8px);
  transition: background 0.15s ease, opacity 0.15s ease;
}

.mp-nav__btn:hover:not(:disabled) {
  background: rgba(255, 255, 255, 0.2);
  color: #fff;
}

.mp-nav__btn:disabled {
  opacity: 0.35;
  cursor: default;
}

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
  background: rgba(255, 255, 255, 0.12);
  color: #e2e8f0;
  cursor: pointer;
  backdrop-filter: blur(8px);
}

.mp-btn:hover {
  background: rgba(255, 255, 255, 0.2);
  color: #fff;
}

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

.mp-notice {
  position: absolute;
  left: 50%;
  bottom: calc(96px + env(safe-area-inset-bottom, 0px));
  transform: translateX(-50%);
  padding: 9px 14px;
  border: 1px solid rgba(255, 255, 255, 0.18);
  border-radius: 8px;
  background: rgba(3, 11, 25, 0.85);
  color: #e2e8f0;
  font-size: 13px;
  white-space: nowrap;
  backdrop-filter: blur(14px);
  box-shadow: 0 12px 38px rgba(0, 0, 0, 0.32);
}

.mp-notice-enter-active,
.mp-notice-leave-active {
  transition: opacity 150ms ease, transform 150ms ease;
}

.mp-notice-enter-from,
.mp-notice-leave-to {
  opacity: 0;
  transform: translate(-50%, 6px);
}

.mp-loading {
  position: absolute;
  left: 50%;
  top: 50%;
  transform: translate(-50%, -50%);
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 13px 17px;
  border: 1px solid rgba(255, 255, 255, 0.15);
  border-radius: 10px;
  background: rgba(3, 11, 25, 0.88);
  color: #e2e8f0;
  font-size: 13px;
  box-shadow: 0 18px 55px rgba(0, 0, 0, 0.36);
}

.mp-loading i {
  color: #1687ff;
  font-size: 16px;
}

.mp-error-wrap {
  position: absolute;
  left: 50%;
  top: 50%;
  transform: translate(-50%, -50%);
  width: min(440px, calc(100vw - 32px));
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  padding: 24px;
  text-align: center;
  border: 1px solid rgba(255, 255, 255, 0.15);
  border-radius: 10px;
  background: rgba(3, 11, 25, 0.88);
  box-shadow: 0 18px 55px rgba(0, 0, 0, 0.36);
}

.mp-error-wrap > i {
  color: #ffb45e;
  font-size: 30px;
}

.mp-error-wrap strong {
  color: #f1f5f9;
  font-size: 15px;
}

.mp-error-wrap span {
  color: #9eb0c8;
  font-size: 13px;
  line-height: 1.7;
}

.mp-error__action {
  margin-top: 6px;
  padding: 9px 18px;
  border: 1px solid #268bff;
  border-radius: 8px;
  background: #187ce0;
  color: #fff;
  font-weight: 650;
  cursor: pointer;
}

@media (orientation: landscape) and (max-height: 420px) {
  .mp-head {
    padding-top: 8px;
    background: none;
  }

  .mp-count {
    display: none;
  }
}
</style>
