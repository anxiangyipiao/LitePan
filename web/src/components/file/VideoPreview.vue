<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref } from "vue";
import "media-chrome";
import "media-chrome/dist/lang/zh-CN.js";
import { setLanguage } from "media-chrome/dist/utils/i18n.js";
import "@fortawesome/fontawesome-free/css/all.min.css";
import { filesApi } from "@/api/files";
import type { FileItem } from "@/api/types";
import { useBodyScrollLock } from "@/composables/useBodyScrollLock";
import { fileKind } from "@/utils/fileIcon";
import { fileExtension } from "@/utils/format";
import { decodeTextBytes } from "@/utils/textEncoding";
import PreviewHeader from "./PreviewHeader.vue";
import BusySpinner from "@/components/base/BusySpinner.vue";

const props = defineProps<{
  accountId: number;
  files: FileItem[];
  initialFileId: string;
}>();

setLanguage("zh-CN");

const emit = defineEmits<{
  close: [];
  download: [file: FileItem];
}>();

const videoRef = ref<HTMLVideoElement | null>(null);
const episodeListRef = ref<HTMLElement | null>(null);
const subtitleMenuRef = ref<HTMLElement | null>(null);
const queueVisible = ref(true);
const subtitleMenuOpen = ref(false);
const mediaLoading = ref(true);
const mediaError = ref(false);
const mediaPlaying = ref(false);
const notice = ref("");
let noticeTimer: number | undefined;
let mediaSession = 0;
let hlsPlayer: { destroy(): void } | null = null;
let mpegtsPlayer: { destroy(): void; detachMediaElement(): void } | null = null;
let subtitleTrack: TextTrack | null = null;
let subtitleController: AbortController | null = null;
let pgsRenderer: { dispose(): void } | null = null;
let subtitleSession = 0;

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

// 外部播放器（VR 视频统一走 Skybox，网页端不再内嵌 VR 播放）
const playerMenuOpen = ref(false);
const playerMenuRef = ref<HTMLElement | null>(null);
// true 时播放器弹窗切换为「Skybox 连接指引」（Skybox 走 WebDAV 网络源，不走 URL scheme）
const skyboxGuideOpen = ref(false);
const externalPlayers = [
  { name: "VLC", icon: "fa-brands fa-vlc", buildUrl: (url: string) => `vlc://${url}` },
  { name: "PotPlayer", icon: "fa-solid fa-play", buildUrl: (url: string) => `potplayer://${url}` },
  { name: "IINA", icon: "fa-solid fa-play", buildUrl: (url: string) => `iina://weblink?url=${encodeURIComponent(url)}` },
  { name: "mpv", icon: "fa-solid fa-play", buildUrl: (url: string) => `mpv://${url}` },
];
const webdavUrl = computed(() => `http://${window.location.host}/dav`);
function openExternalPlayer(url: string) {
  window.location.href = url;
  playerMenuOpen.value = false;
  skyboxGuideOpen.value = false;
}
function togglePlayerMenu() {
  if (playerMenuOpen.value) skyboxGuideOpen.value = false;
  playerMenuOpen.value = !playerMenuOpen.value;
}
function openSkyboxGuide() {
  skyboxGuideOpen.value = true;
}
async function copyWebdavUrl() {
  try {
    await navigator.clipboard.writeText(webdavUrl.value);
    showNotice("WebDAV 地址已复制");
  } catch {
    showNotice("复制失败，请手动复制");
  }
}

const selectedSubtitleId = ref("");

function fileStem(name: string) {
  return name.replace(/\.[^.]+$/, "");
}

function normalizedStem(name: string) {
  return fileStem(name).toLowerCase().replace(/[._\s-]+/g, " ").trim();
}

function subtitleLabel(name: string, videoName: string) {
  const suffix = fileStem(name).slice(fileStem(videoName).length).replace(/^[._\s-]+/, "").toLowerCase();
  if (/^(zh|zh-cn|chs|sc|简|简中)/.test(suffix)) return "简体中文";
  if (/^(zh-tw|cht|tc|繁|繁中)/.test(suffix)) return "繁體中文";
  if (/^(en|eng|english)/.test(suffix)) return "English";
  if (/^(ja|jpn|jp)/.test(suffix)) return "日本語";
  if (/^(ko|kor|kr)/.test(suffix)) return "한국어";
  return suffix ? suffix.toUpperCase() : "默认字幕";
}

const subtitleCandidates = computed(() => {
  const video = currentFile.value;
  if (!video) return [];
  const target = normalizedStem(video.name);
  const exact: FileItem[] = [];
  for (const file of props.files) {
    if (file.is_dir || !["srt", "vtt", "sup"].includes(fileExtension(file.name))) continue;
    const stem = normalizedStem(file.name);
    if (stem === target || stem.startsWith(`${target} `)) exact.push(file);
  }
  const candidates = exact.length > 0
    ? exact
    : episodes.value.length === 1
      ? props.files.filter((file) => !file.is_dir && ["srt", "vtt", "sup"].includes(fileExtension(file.name))).slice(0, 1)
      : [];
  return candidates.map((file) => ({
    file,
    format: fileExtension(file.name) as "srt" | "vtt" | "sup",
    label: subtitleLabel(file.name, video.name),
  }));
});

const selectedSubtitleLabel = computed(() =>
  subtitleCandidates.value.find(({ file }) => file.id === selectedSubtitleId.value)?.label || "字幕",
);

function selectSubtitle(fileId: string) {
  selectedSubtitleId.value = fileId;
  subtitleMenuOpen.value = false;
  showNotice(fileId ? `已切换字幕：${selectedSubtitleLabel.value}` : "字幕已关闭");
  void loadSelectedSubtitle();
}

function handleDocumentPointerDown(event: PointerEvent) {
  const target = event.target instanceof Node ? event.target : null;
  if (subtitleMenuOpen.value && target && !subtitleMenuRef.value?.contains(target)) {
    subtitleMenuOpen.value = false;
  }
  if (playerMenuOpen.value && target && !playerMenuRef.value?.contains(target)) {
    playerMenuOpen.value = false;
    skyboxGuideOpen.value = false;
  }
}

function clearSubtitleTrack() {
  subtitleSession += 1;
  subtitleController?.abort();
  subtitleController = null;
  pgsRenderer?.dispose();
  pgsRenderer = null;
  if (subtitleTrack) {
    subtitleTrack.mode = "disabled";
    Array.from(subtitleTrack.cues || []).forEach((cue) => subtitleTrack?.removeCue(cue));
    subtitleTrack = null;
  }
}

async function loadSelectedSubtitle() {
  clearSubtitleTrack();
  const session = subtitleSession;
  const video = videoRef.value;
  const selected = subtitleCandidates.value.find(({ file }) => file.id === selectedSubtitleId.value);
  if (!video || !selected) return;
  const controller = new AbortController();
  subtitleController = controller;
  try {
    if (selected.format === "sup") {
      showNotice("正在加载 SUP 字幕…");
      const response = await fetch(
        filesApi.proxyPreviewURL(props.accountId, selected.file.id, selected.file.name),
        { credentials: "include", signal: controller.signal },
      );
      if (!response.ok) throw new Error(`字幕读取失败 (${response.status})`);
      const subContent = await response.arrayBuffer();
      const { PgsRenderer } = await import("libbitsub");
      if (controller.signal.aborted || session !== subtitleSession || video !== videoRef.value) return;
      const renderer = new PgsRenderer({
        video,
        subContent,
        cacheLimit: 32,
        prefetchWindow: { before: 1, after: 2 },
        onLoading: () => {
          const canvases = video.parentElement?.querySelectorAll("canvas");
          const canvas = canvases?.item((canvases.length || 1) - 1);
          if (canvas) {
              // 字幕画布禁止随控制栏淡出隐藏。
            canvas.setAttribute("noautohide", "");
            canvas.style.zIndex = "4";
            canvas.style.pointerEvents = "none";
          }
        },
        onLoaded: () => {
          if (session === subtitleSession) showNotice(`SUP 字幕已加载：${selected.label}`);
        },
        onError: (error) => {
          if (session === subtitleSession) showNotice(`SUP 字幕加载失败：${error.message}`);
        },
      });
      if (session !== subtitleSession) {
        renderer.dispose();
        return;
      }
      pgsRenderer = renderer;
      return;
    }
    const result = await filesApi.textPreviewBytes(
      props.accountId,
      selected.file.id,
      selected.file.name,
      selected.file.size,
      controller.signal,
    );
    const { text } = decodeTextBytes(result.bytes, "auto", result.truncated);
    const { parseText } = await import("media-captions");
    const parsed = await parseText(text, { type: selected.format });
    if (controller.signal.aborted || session !== subtitleSession || video !== videoRef.value) return;
    const track = video.addTextTrack("subtitles", selected.label, "");
    for (const cue of parsed.cues) {
      track.addCue(new VTTCue(cue.startTime, cue.endTime, cue.text));
    }
    track.mode = "showing";
    subtitleTrack = track;
  } catch (reason) {
    if (!controller.signal.aborted && session === subtitleSession) {
      showNotice(reason instanceof Error ? `字幕加载失败：${reason.message}` : "字幕加载失败");
    }
  }
}

function resetSubtitleSelection() {
  clearSubtitleTrack();
  subtitleMenuOpen.value = false;
  selectedSubtitleId.value = subtitleCandidates.value[0]?.file.id || "";
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
    const player = new Hls();
    player.loadSource(url);
    player.attachMedia(video);
    hlsPlayer = player;
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
    mpegtsPlayer = player;
    return;
  }

  video.src = url;
  video.load();
}

function episodeMeta(file: FileItem, index: number) {
  const stem = fileStem(file.name);
  const seasonEpisode = stem.match(/s(\d{1,2})e(\d{1,3})/i);
  const simpleEpisode = stem.match(/(?:^|[\s._-])(?:ep?|第)\s*0*(\d{1,3})(?:集)?(?:[\s._-]|$)/i);
  const code = seasonEpisode
    ? `S${seasonEpisode[1].padStart(2, "0")}E${seasonEpisode[2].padStart(2, "0")}`
    : simpleEpisode
      ? `E${simpleEpisode[1].padStart(2, "0")}`
      : `视频 ${String(index + 1).padStart(2, "0")}`;
  const title = stem
    .replace(seasonEpisode?.[0] ?? simpleEpisode?.[0] ?? "", " ")
    .replace(/[._]+/g, " ")
    .replace(/\s+/g, " ")
    .trim();
  return { code, title: title || stem };
}

function showNotice(message: string) {
  notice.value = message;
  window.clearTimeout(noticeTimer);
  noticeTimer = window.setTimeout(() => {
    notice.value = "";
  }, 1500);
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
  void loadSelectedSubtitle();
}

async function selectEpisode(index: number) {
  if (index < 0 || index >= episodes.value.length) return;
  if (index === currentIndex.value) return;
  currentIndex.value = index;
  mediaError.value = false;
  mediaLoading.value = true;
  mediaPlaying.value = false;
  const file = episodes.value[index];
  resetSubtitleSelection();
  showNotice(`正在播放 ${fileStem(file.name)}`);
  await playCurrent();
  scrollCurrentIntoView();
}

function playAdjacent(direction: -1 | 1) {
  const target = currentIndex.value + direction;
  if (target < 0 || target >= episodes.value.length) {
    showNotice(direction > 0 ? "已经是最后一个视频" : "已经是第一个视频");
    return;
  }
  void selectEpisode(target);
}

function scrollCurrentIntoView() {
  void nextTick(() => {
    episodeListRef.value
      ?.querySelector<HTMLElement>(".episode-card.is-active")
      ?.scrollIntoView({ behavior: "smooth", block: "nearest", inline: "center" });
  });
}

function scrollEpisodeList(direction: -1 | 1) {
  const list = episodeListRef.value;
  if (!list) return;
  list.scrollBy({
    left: direction * Math.max(320, list.clientWidth * 0.75),
    behavior: "smooth",
  });
}

function adjustTime(seconds: number) {
  const video = videoRef.value;
  if (!video) return;
  const duration = Number.isFinite(video.duration) ? video.duration : Infinity;
  video.currentTime = Math.max(0, Math.min(duration, video.currentTime + seconds));
  showNotice(seconds > 0 ? `快进 ${seconds} 秒` : `后退 ${Math.abs(seconds)} 秒`);
}

function adjustVolume(delta: number) {
  const video = videoRef.value;
  if (!video) return;
  video.volume = Math.max(0, Math.min(1, video.volume + delta));
  if (video.volume > 0) video.muted = false;
  showNotice(`音量 ${Math.round(video.volume * 100)}%`);
}

function togglePlayback() {
  const video = videoRef.value;
  if (!video) return;
  if (video.paused) void video.play().catch(() => undefined);
  else video.pause();
}

function toggleMute() {
  const video = videoRef.value;
  if (!video) return;
  video.muted = !video.muted;
  showNotice(video.muted ? "已静音" : "已取消静音");
}

function isEditableTarget(target: EventTarget | null) {
  const element = target instanceof HTMLElement ? target : null;
  return !!element?.closest("input, textarea, select, [contenteditable='true']");
}

function handleKeydown(event: KeyboardEvent) {
  if (event.key === "Escape" && (subtitleMenuOpen.value || playerMenuOpen.value)) {
    event.preventDefault();
    subtitleMenuOpen.value = false;
    playerMenuOpen.value = false;
    skyboxGuideOpen.value = false;
    return;
  }
  if (isEditableTarget(event.target)) return;
  const key = event.key.toLowerCase();
  if (["arrowleft", "arrowright", "arrowup", "arrowdown", " "].includes(key)) {
    event.preventDefault();
  }
  if (key === "escape") emit("close");
  else if (key === "arrowleft") adjustTime(-10);
  else if (key === "arrowright") adjustTime(10);
  else if (key === "arrowup") adjustVolume(0.05);
  else if (key === "arrowdown") adjustVolume(-0.05);
  else if (key === " ") togglePlayback();
  else if (key === "n") playAdjacent(1);
  else if (key === "p") playAdjacent(-1);
  else if (key === "m") toggleMute();
}

function handleMediaReady() {
  mediaLoading.value = false;
  mediaError.value = false;
}

function handleMediaPlaying() {
  handleMediaReady();
  mediaPlaying.value = true;
}

function handleMediaWaiting() {
  mediaLoading.value = true;
  mediaPlaying.value = false;
}

function handleMediaError() {
  mediaLoading.value = false;
  mediaError.value = true;
  mediaPlaying.value = false;
}

function downloadCurrent() {
  if (currentFile.value) emit("download", currentFile.value);
}

useBodyScrollLock();

onMounted(() => {
  window.addEventListener("keydown", handleKeydown);
  document.addEventListener("pointerdown", handleDocumentPointerDown);
  scrollCurrentIntoView();
  resetSubtitleSelection();
  void playCurrent();
});

onUnmounted(() => {
  window.clearTimeout(noticeTimer);
  clearSubtitleTrack();
  destroyMediaAdapters();
  window.removeEventListener("keydown", handleKeydown);
  document.removeEventListener("pointerdown", handleDocumentPointerDown);
});
</script>

<template>
  <Teleport to="body">
    <main class="file-preview video-preview" role="dialog" aria-modal="true" aria-label="视频预览">
      <PreviewHeader
        :file-name="currentFile?.name"
        :status="`正在播放第${currentIndex + 1}集/共${episodes.length}集`"
        download-label="下载当前视频"
        @close="emit('close')"
        @download="downloadCurrent"
      />

      <section class="video-preview__stage">
        <media-controller class="video-preview__controller">
          <video
            v-if="currentFile"
            ref="videoRef"
            slot="media"
            :key="currentFile.id"
            autoplay
            playsinline
            preload="metadata"
            @canplay="handleMediaReady"
            @playing="handleMediaPlaying"
            @pause="mediaPlaying = false"
            @waiting="handleMediaWaiting"
            @error="handleMediaError"
            @ended="playAdjacent(1)"
          />

          <div class="video-preview__bottom" :class="{ 'is-compact': !queueVisible }" slot="centered-chrome">
            <Transition name="notice">
              <div v-if="notice" class="video-preview__notice" role="status">{{ notice }}</div>
            </Transition>

            <div v-if="mediaLoading && !mediaError" class="video-preview__loading" aria-label="正在加载视频">
              <BusySpinner :size="19" color="#1687ff" />
              <b>正在加载视频…</b>
            </div>

            <div v-if="mediaError" class="video-preview__error" role="alert">
              <i class="fa-solid fa-circle-exclamation" aria-hidden="true" />
              <strong>浏览器无法直接播放这个视频</strong>
              <span>可能是视频封装或编码格式不受当前浏览器支持，可下载后使用本地播放器打开。</span>
              <button type="button" @click="downloadCurrent">下载视频</button>
            </div>

            <div v-if="queueVisible && episodes.length > 1" class="video-preview__queue">
              <button type="button" class="video-preview__queue-arrow" aria-label="向左查看选集" @click="scrollEpisodeList(-1)">
                <i class="fa-solid fa-chevron-left" aria-hidden="true" />
              </button>
              <div ref="episodeListRef" class="video-preview__episodes">
                <button
                  v-for="(episode, index) in episodes"
                  :key="episode.id"
                  type="button"
                  class="episode-card"
                  :class="{ 'is-active': index === currentIndex }"
                  :aria-current="index === currentIndex ? 'true' : undefined"
                  @click="selectEpisode(index)"
                >
                  <span class="episode-card__number">{{ String(index + 1).padStart(2, "0") }}</span>
                  <span class="episode-card__copy">
                    <small>{{ episodeMeta(episode, index).code }}</small>
                    <strong :title="fileStem(episode.name)">{{ episodeMeta(episode, index).title }}</strong>
                  </span>
                  <span
                    v-if="index === currentIndex"
                    class="episode-card__playing"
                    :class="{ 'is-paused': !mediaPlaying }"
                    aria-label="当前播放"
                  >
                    <i v-for="bar in 4" :key="bar" aria-hidden="true" />
                  </span>
                  <span v-else-if="index === currentIndex + 1" class="episode-card__state is-next">下一个</span>
                </button>
              </div>
              <button type="button" class="video-preview__queue-arrow" aria-label="向右查看选集" @click="scrollEpisodeList(1)">
                <i class="fa-solid fa-chevron-right" aria-hidden="true" />
              </button>
            </div>

            <media-control-bar class="video-preview__controls">
              <media-play-button aria-label="播放或暂停" />
              <media-time-display show-duration />
              <media-time-range />
              <media-mute-button aria-label="静音" />
              <media-volume-range />
              <div
                v-if="subtitleCandidates.length"
                ref="subtitleMenuRef"
                class="video-preview__subtitle-menu"
              >
                <button
                  type="button"
                  class="video-preview__subtitle-button"
                  :class="{ 'is-active': subtitleMenuOpen || selectedSubtitleId }"
                  aria-haspopup="menu"
                  :aria-expanded="subtitleMenuOpen"
                  title="选择字幕"
                  @click="subtitleMenuOpen = !subtitleMenuOpen"
                >
                  <i class="fa-regular fa-closed-captioning" aria-hidden="true" />
                  <span>{{ selectedSubtitleLabel }}</span>
                  <i class="fa-solid fa-chevron-up video-preview__subtitle-chevron" aria-hidden="true" />
                </button>

                <Transition name="subtitle-menu">
                  <div v-if="subtitleMenuOpen" class="video-preview__subtitle-popover" role="menu" aria-label="字幕选择">
                    <div class="video-preview__subtitle-heading">
                      <span>字幕</span>
                      <small>{{ subtitleCandidates.length }} 个可用</small>
                    </div>
                    <button
                      type="button"
                      class="video-preview__subtitle-option"
                      :class="{ 'is-selected': !selectedSubtitleId }"
                      role="menuitemradio"
                      :aria-checked="!selectedSubtitleId"
                      @click="selectSubtitle('')"
                    >
                      <span>关闭字幕</span>
                      <i v-if="!selectedSubtitleId" class="fa-solid fa-check" aria-hidden="true" />
                    </button>
                    <button
                      v-for="subtitle in subtitleCandidates"
                      :key="subtitle.file.id"
                      type="button"
                      class="video-preview__subtitle-option"
                      :class="{ 'is-selected': selectedSubtitleId === subtitle.file.id }"
                      role="menuitemradio"
                      :aria-checked="selectedSubtitleId === subtitle.file.id"
                      @click="selectSubtitle(subtitle.file.id)"
                    >
                      <span>{{ subtitle.label }}</span>
                      <small>{{ subtitle.format.toUpperCase() }}</small>
                      <i v-if="selectedSubtitleId === subtitle.file.id" class="fa-solid fa-check" aria-hidden="true" />
                    </button>
                  </div>
                </Transition>
              </div>
              <button
                v-if="episodes.length > 1"
                type="button"
                class="video-preview__queue-toggle"
                :class="{ 'is-active': queueVisible }"
                :aria-label="queueVisible ? '收起选集' : '展开选集'"
                @click="queueVisible = !queueVisible"
              >
                <i class="fa-solid fa-list-ul" aria-hidden="true" />
              </button>
              <media-playback-rate-button rates="0.5 0.75 1 1.25 1.5 2" />
              <div ref="playerMenuRef" class="video-preview__player-menu">
                <button
                  type="button"
                  class="video-preview__player-trigger"
                  :class="{ 'is-active': playerMenuOpen }"
                  aria-label="外部播放器"
                  title="用外部播放器打开"
                  @click.stop="togglePlayerMenu"
                >
                  <i class="fa-solid fa-display" aria-hidden="true"></i>
                </button>
                <Transition name="subtitle-menu">
                  <div v-if="playerMenuOpen && !skyboxGuideOpen" class="video-preview__player-popover" role="menu" aria-label="选择播放器">
                    <div class="video-preview__player-heading">选择播放器</div>
                    <button
                      v-for="player in externalPlayers"
                      :key="player.name"
                      type="button"
                      class="video-preview__player-option"
                      role="menuitem"
                      @click="openExternalPlayer(player.buildUrl(mediaURL))"
                    >
                      <i :class="player.icon" aria-hidden="true"></i>
                      <span>{{ player.name }}</span>
                    </button>
                    <div class="video-preview__player-sep" role="separator" />
                    <button
                      type="button"
                      class="video-preview__player-option"
                      role="menuitem"
                      @click="openSkyboxGuide"
                    >
                      <i class="fa-solid fa-vr-cardboard" aria-hidden="true"></i>
                      <span>Skybox（Quest）</span>
                      <i class="fa-solid fa-chevron-right video-preview__player-chevron" aria-hidden="true"></i>
                    </button>
                  </div>
                  <div v-else-if="playerMenuOpen && skyboxGuideOpen" class="video-preview__player-popover video-preview__skybox-guide" role="dialog" aria-label="Skybox 连接指引">
                    <button
                      type="button"
                      class="video-preview__player-back"
                      @click="skyboxGuideOpen = false"
                    >
                      <i class="fa-solid fa-arrow-left" aria-hidden="true"></i>
                      返回
                    </button>
                    <div class="video-preview__skybox-title">用 Skybox 播放网盘 VR 视频</div>
                    <ol class="video-preview__skybox-steps">
                      <li>
                        管理后台 <b>系统设置 → WebDAV</b> 开启 WebDAV 并设置账号密码（若已开启可跳过）。
                      </li>
                      <li>
                        Skybox（Quest）→ <b>本地网络 / Network</b> → 添加 WebDAV 服务器。
                      </li>
                      <li>地址填下方 URL，账号密码填第 1 步设置的，然后浏览到视频并播放。</li>
                    </ol>
                    <div class="video-preview__skybox-url-row">
                      <code class="video-preview__skybox-url">{{ webdavUrl }}</code>
                      <button
                        type="button"
                        class="video-preview__skybox-copy"
                        @click="copyWebdavUrl"
                      >
                        <i class="fa-regular fa-copy" aria-hidden="true"></i>
                        复制
                      </button>
                    </div>
                    <p class="video-preview__skybox-hint">Skybox 会自动识别 180°/360°/左右立体，或播放时在视角菜单手动切换。</p>
                  </div>
                </Transition>
              </div>
              <media-fullscreen-button aria-label="全屏" />
            </media-control-bar>

            <div class="video-preview__shortcuts" aria-hidden="true">
              <span>← / → 快退快进</span><i>·</i><span>↑ / ↓ 音量</span><i>·</i>
              <span>空格 播放暂停</span><i>·</i><span>P 上一集丨N 下一集</span><i>·</i><span>M 静音切换</span>
            </div>
          </div>

        </media-controller>
      </section>
    </main>
  </Teleport>
</template>

<style scoped>
.video-preview {
  min-height: 560px;
}

.video-preview__queue-arrow,
.video-preview__queue-toggle {
  border: 0;
  background: transparent;
}
.video-preview__subtitle-menu { position: relative; min-width: 0; }

.video-preview__subtitle-button {
  width: 100%;
  height: 38px;
  display: grid;
  grid-template-columns: 18px minmax(0, 1fr) 10px;
  align-items: center;
  gap: 7px;
  padding: 0 9px;
  overflow: hidden;
  color: #c9d7e9;
  border: 1px solid transparent;
  border-radius: 8px;
  outline: none;
  background: transparent;
  font: inherit;
  font-size: 12px;
  cursor: pointer;
  transition: color 150ms ease, border-color 150ms ease, background 150ms ease;
}

.video-preview__subtitle-button:hover,
.video-preview__subtitle-button.is-active {
  color: #49a5ff;
  border-color: rgb(51 151 255 / 22%);
  background: rgb(27 132 241 / 13%);
}
.video-preview__subtitle-button:focus-visible { outline: 2px solid #2698ff; outline-offset: 2px; }

.video-preview__subtitle-button > span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.video-preview__subtitle-button > .fa-closed-captioning { color: #2695ff; font-size: 17px; }
.video-preview__subtitle-chevron { color: #7f91a9; font-size: 8px; transition: transform 150ms ease; }
.video-preview__subtitle-button[aria-expanded="false"] .video-preview__subtitle-chevron { transform: rotate(180deg); }

.video-preview__subtitle-popover {
  position: absolute;
  right: 0;
  bottom: calc(100% + 10px);
  z-index: 30;
  width: max-content;
  min-width: 190px;
  max-width: min(280px, calc(100vw - 24px));
  padding: 7px;
  overflow: hidden;
  color: #eaf3ff;
  border: 1px solid rgb(89 151 224 / 26%);
  border-radius: 11px;
  background: rgb(6 17 34 / 96%);
  box-shadow: 0 16px 40px rgb(0 0 0 / 48%), 0 0 0 1px rgb(0 0 0 / 22%);
  backdrop-filter: blur(18px);
}

.video-preview__subtitle-heading {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 18px;
  padding: 7px 9px 9px;
  color: #f5f9ff;
  font-size: 12px;
  font-weight: 650;
}

.video-preview__subtitle-heading small { color: #71839b; font-size: 10px; font-weight: 500; }
.video-preview__subtitle-option {
  width: 100%;
  min-height: 36px;
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto 14px;
  align-items: center;
  gap: 12px;
  padding: 0 10px;
  color: #9fb0c6;
  text-align: left;
  border: 0;
  border-radius: 7px;
  outline: none;
  background: transparent;
  font: inherit;
  font-size: 12px;
  cursor: pointer;
}

.video-preview__subtitle-option:hover { color: #edf7ff; background: rgb(255 255 255 / 7%); }
.video-preview__subtitle-option:focus-visible { box-shadow: inset 0 0 0 1px #2698ff; }
.video-preview__subtitle-option.is-selected { color: #fff; background: linear-gradient(90deg, rgb(22 126 229 / 42%), rgb(22 126 229 / 18%)); }
.video-preview__subtitle-option small { color: #70849e; font-size: 9px; font-weight: 650; letter-spacing: 0.04em; }
.video-preview__subtitle-option.is-selected small { color: #88c8ff; }
.video-preview__subtitle-option .fa-check { grid-column: 3; color: #45a9ff; font-size: 10px; }
.subtitle-menu-enter-active,
.subtitle-menu-leave-active { transition: opacity 130ms ease, transform 130ms ease; transform-origin: right bottom; }
.subtitle-menu-enter-from,
.subtitle-menu-leave-to { opacity: 0; transform: translateY(5px) scale(0.97); }
.video-preview__queue-toggle:hover { background: rgb(255 255 255 / 12%); }

.video-preview__stage,
.video-preview__controller,
.video-preview__controller video { display: block; width: 100%; height: 100%; }

.video-preview__controller {
  --media-primary-color: #1687ff;
  --media-secondary-color: #f4f8ff;
  --media-control-background: transparent;
  --media-control-hover-background: rgb(255 255 255 / 11%);
  background: #020711;
}

.video-preview__controller video { object-fit: contain; background: #020711; }

.video-preview__bottom {
  position: absolute;
  inset: auto 0 0;
  z-index: 10;
  min-height: 330px;
  display: flex;
  flex-direction: column;
  justify-content: flex-end;
  padding: 78px 36px 20px;
  pointer-events: none;
  line-height: 1.35;
  background: linear-gradient(180deg, transparent 0%, rgb(2 8 18 / 24%) 15%, rgb(2 8 18 / 88%) 57%, #020711 100%);
  transition: min-height 180ms ease;
}

.video-preview__bottom.is-compact { min-height: 145px; }
.video-preview__bottom > * { pointer-events: auto; }

.video-preview__notice {
  position: absolute;
  left: 50%;
  bottom: min(355px, 48vh);
  transform: translateX(-50%);
  padding: 9px 14px;
  border: 1px solid rgb(255 255 255 / 18%);
  border-radius: 8px;
  background: rgb(3 11 25 / 85%);
  box-shadow: 0 12px 38px rgb(0 0 0 / 32%);
  font-size: 13px;
  white-space: nowrap;
  backdrop-filter: blur(14px);
}

.notice-enter-active, .notice-leave-active { transition: opacity 150ms ease, transform 150ms ease; }
.notice-enter-from, .notice-leave-to { opacity: 0; transform: translate(-50%, 6px); }

.video-preview__loading,
.video-preview__error {
  position: fixed;
  left: 50%;
  top: 48%;
  transform: translate(-50%, -50%);
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 13px 17px;
  border: 1px solid rgb(255 255 255 / 15%);
  border-radius: 10px;
  background: rgb(3 11 25 / 88%);
  box-shadow: 0 18px 55px rgb(0 0 0 / 36%);
}

.video-preview__loading b { font-size: 13px; font-weight: 550; }
.video-preview__error {
  width: min(440px, calc(100vw - 32px));
  flex-direction: column;
  text-align: center;
  padding: 24px;
}

.video-preview__error > i { color: #ffb45e; font-size: 30px; }
.video-preview__error strong { font-size: 17px; }
.video-preview__error span { color: #9eb0c8; font-size: 13px; line-height: 1.7; }
.video-preview__error button {
  margin-top: 4px;
  padding: 9px 18px;
  border: 1px solid #268bff;
  border-radius: 8px;
  background: #187ce0;
  font-weight: 650;
}

.video-preview__queue {
  display: grid;
  grid-template-columns: 38px minmax(0, 1fr) 38px;
  align-items: stretch;
  gap: 10px;
  margin-bottom: 15px;
}

.video-preview__queue-arrow { border-radius: 9px; font-size: 22px; opacity: 0.88; }
.video-preview__queue-arrow:hover { color: #3f9dff; background: rgb(255 255 255 / 7%); }
.video-preview__episodes { display: flex; gap: 10px; overflow-x: auto; scrollbar-width: none; }
.video-preview__episodes::-webkit-scrollbar { display: none; }

.episode-card {
  position: relative;
  flex: 0 0 clamp(155px, 15vw, 220px);
  min-width: 0;
  min-height: 78px;
  display: grid;
  grid-template-columns: auto minmax(0, 1fr);
  align-content: center;
  column-gap: 11px;
  padding: 12px 13px;
  overflow: hidden;
  text-align: left;
  border: 1px solid rgb(139 169 213 / 24%);
  border-radius: 9px;
  background: rgb(10 22 41 / 64%);
  backdrop-filter: blur(12px);
  transition: border-color 150ms ease, background 150ms ease, transform 150ms ease;
}

.episode-card:hover { transform: translateY(-2px); border-color: rgb(75 154 255 / 72%); background: rgb(18 40 72 / 78%); }
.episode-card.is-active { border-color: #2794ff; background: rgb(13 54 103 / 84%); box-shadow: 0 0 0 2px rgb(28 139 255 / 52%); }
.episode-card__number { color: #9eb1c9; font-size: 19px; font-weight: 700; }
.episode-card.is-active .episode-card__number { color: #fff; }
.episode-card__copy { min-width: 0; display: flex; flex-direction: column; gap: 3px; }
.episode-card__copy small { color: #91a4be; font-size: 10px; }
.episode-card__copy strong { overflow: hidden; font-size: 14px; font-weight: 650; text-overflow: ellipsis; white-space: nowrap; }
.episode-card__state {
  position: absolute;
  right: 7px;
  top: 6px;
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 3px 6px;
  border-radius: 999px;
  color: #cbe6ff;
  background: #187ce0;
  font-size: 9px;
}
.episode-card__state.is-next { background: rgb(35 123 227 / 48%); }

.episode-card__playing {
  position: absolute;
  right: 9px;
  top: 8px;
  width: 23px;
  height: 18px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 2px;
  color: #78bdff;
}

.episode-card__playing i {
  width: 2px;
  height: 12px;
  border-radius: 2px;
  background: currentColor;
  transform-origin: center;
  animation: episode-equalizer 760ms ease-in-out infinite alternate;
}

.episode-card__playing i:nth-child(2) { animation-duration: 560ms; animation-delay: -180ms; }
.episode-card__playing i:nth-child(3) { animation-duration: 890ms; animation-delay: -330ms; }
.episode-card__playing i:nth-child(4) { animation-duration: 640ms; animation-delay: -90ms; }
.episode-card__playing.is-paused i { animation-play-state: paused; opacity: 0.55; }

@keyframes episode-equalizer {
  0% { transform: scaleY(0.28); }
  45% { transform: scaleY(0.9); }
  100% { transform: scaleY(0.48); }
}

.video-preview__controls {
  display: grid;
  grid-template-columns: 50px auto minmax(180px, 1fr) 44px 105px minmax(104px, 128px) 44px 68px 32px 44px;
  align-items: center;
  gap: 7px;
  min-height: 52px;
}

.video-preview__controls > * { min-width: 0; }
.video-preview__controls media-play-button { --media-button-icon-width: 27px; --media-button-icon-height: 27px; }
.video-preview__controls media-time-display { min-width: 106px; color: #c7d3e4; font-size: 13px; }
.video-preview__controls media-time-range {
  width: 100%;
  --media-range-track-background: rgb(201 215 236 / 40%);
  --media-time-range-buffered-color: rgb(255 255 255 / 24%);
  --media-range-bar-color: #1687ff;
}
.video-preview__controls media-volume-range { --media-range-track-height: 4px; }
.video-preview__queue-toggle { width: 42px; height: 42px; border-radius: 8px; color: #cfdaea; font-size: 17px; }
.video-preview__queue-toggle.is-active { color: #2794ff; }
.video-preview__player-menu {
  position: relative;
  display: flex;
  align-items: center;
}
.video-preview__player-trigger {
  width: 32px;
  height: 32px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: 0;
  border-radius: 6px;
  outline: none;
  background: transparent;
  color: #7f91a9;
  font-size: 14px;
  cursor: pointer;
  transition: color 150ms ease, background 150ms ease;
}
.video-preview__player-trigger:hover,
.video-preview__player-trigger.is-active { color: #c7d3e4; background: rgb(255 255 255 / 11%); }
.video-preview__player-trigger:focus-visible { box-shadow: inset 0 0 0 1px #2698ff; }

.video-preview__player-popover {
  position: absolute;
  right: 0;
  bottom: calc(100% + 10px);
  z-index: 30;
  width: max-content;
  min-width: 160px;
  padding: 7px;
  overflow: hidden;
  color: #eaf3ff;
  border: 1px solid rgb(89 151 224 / 26%);
  border-radius: 11px;
  background: rgb(6 17 34 / 96%);
  box-shadow: 0 16px 40px rgb(0 0 0 / 48%), 0 0 0 1px rgb(0 0 0 / 22%);
  backdrop-filter: blur(18px);
}
.video-preview__player-heading {
  padding: 7px 9px 9px;
  color: #f5f9ff;
  font-size: 12px;
  font-weight: 650;
}
.video-preview__player-option {
  width: 100%;
  display: flex;
  align-items: center;
  gap: 10px;
  min-height: 36px;
  padding: 0 10px;
  color: #9fb0c6;
  text-align: left;
  border: 0;
  border-radius: 7px;
  outline: none;
  background: transparent;
  font: inherit;
  font-size: 12px;
  cursor: pointer;
  transition: color 150ms ease, background 150ms ease;
}
.video-preview__player-option:hover { color: #edf7ff; background: rgb(255 255 255 / 7%); }
.video-preview__player-option:focus-visible { box-shadow: inset 0 0 0 1px #2698ff; }
.video-preview__player-option i { width: 16px; text-align: center; font-size: 13px; }
.video-preview__player-chevron { margin-left: auto; font-size: 11px !important; opacity: 0.6; }
.video-preview__player-sep { height: 1px; margin: 5px 4px; background: rgb(255 255 255 / 9%); }

.video-preview__skybox-guide {
  width: 300px;
  max-width: 86vw;
  padding: 10px 12px 12px;
}
.video-preview__player-back {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 3px 8px;
  margin-bottom: 6px;
  border: none;
  border-radius: 7px;
  color: #9fb0c6;
  background: transparent;
  font-size: 12px;
  cursor: pointer;
}
.video-preview__player-back:hover { color: #edf7ff; background: rgb(255 255 255 / 7%); }
.video-preview__skybox-title { color: #f5f9ff; font-size: 13px; font-weight: 700; margin-bottom: 8px; }
.video-preview__skybox-steps {
  margin: 0 0 10px;
  padding-left: 18px;
  display: flex;
  flex-direction: column;
  gap: 6px;
  color: #b6c6da;
  font-size: 12px;
  line-height: 1.55;
}
.video-preview__skybox-steps b { color: #eaf3ff; font-weight: 650; }
.video-preview__skybox-url-row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 7px 9px;
  border: 1px solid rgb(89 151 224 / 26%);
  border-radius: 9px;
  background: rgb(0 0 0 / 22%);
}
.video-preview__skybox-url {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: #8fd0ff;
  font-size: 12px;
}
.video-preview__skybox-copy {
  flex: none;
  display: inline-flex;
  align-items: center;
  gap: 5px;
  padding: 4px 9px;
  border: none;
  border-radius: 7px;
  color: #eaf3ff;
  background: rgb(22 135 255 / 85%);
  font-size: 12px;
  font-weight: 600;
  cursor: pointer;
}
.video-preview__skybox-copy:hover { background: rgb(22 135 255 / 1); }
.video-preview__skybox-hint { margin: 9px 0 0; color: #7d8ca3; font-size: 11px; line-height: 1.5; }

.video-preview__shortcuts {
  min-height: 25px;
  display: flex;
  justify-content: center;
  align-items: center;
  gap: 13px;
  color: #7d8ca3;
  font-size: 11px;
}
.video-preview__shortcuts i { opacity: 0.56; }

@media (max-width: 1100px) {
  .video-preview__controls { grid-template-columns: 46px auto minmax(130px, 1fr) 42px 84px 96px 42px 62px 28px 42px; }
}

@media (max-width: 768px) {
  .video-preview__bottom { min-height: 295px; padding: 68px 9px 8px; }
  .video-preview__bottom.is-compact { min-height: 115px; }
  .video-preview__notice { bottom: 310px; max-width: 86vw; overflow: hidden; text-overflow: ellipsis; }
  .video-preview__queue { grid-template-columns: 28px minmax(0, 1fr) 28px; gap: 3px; margin-bottom: 8px; }
  .episode-card { flex-basis: 158px; min-height: 70px; }
  .video-preview__controls { grid-template-columns: 42px minmax(62px, auto) minmax(70px, 1fr) 40px 40px 40px; gap: 1px; }
  .video-preview__controls media-volume-range,
  .video-preview__controls media-playback-rate-button,
  .video-preview__player-trigger { display: none; }
  .video-preview__subtitle-button { grid-template-columns: 1fr; padding: 0; }
  .video-preview__subtitle-button > span,
  .video-preview__subtitle-chevron { display: none; }
  .video-preview__subtitle-popover { right: -46px; }
  .video-preview__shortcuts { display: none; }
}

@media (max-height: 700px) and (min-width: 761px) {
  .video-preview__bottom { min-height: 285px; padding-top: 58px; }
  .episode-card { min-height: 68px; }
  .video-preview__shortcuts { display: none; }
}
</style>
